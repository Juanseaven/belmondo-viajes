package mirror

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/blobs"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/evolution"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

var (
	// ErrLineOffline: la línea no está vinculada. Reintentar no sirve hasta que
	// alguien vuelva a escanear el QR, así que se distingue de un fallo genérico.
	ErrLineOffline = errors.New("la línea no está conectada")

	// ErrSendRejected: Evolution aceptó la petición pero WhatsApp la rechazó.
	ErrSendRejected = errors.New("el envío fue rechazado")
)

// panelSendTTL es cuánto vive la marca que atribuye un envío a su autor.
//
// Cubre de sobra la latencia del webhook, que vuelve en uno o dos segundos. Si
// aun así se perdiera, la consulta a outbound_messages recupera el autor.
const panelSendTTL = 5 * time.Minute

// Sender es el cliente de Evolution visto por este archivo.
type Sender interface {
	SendText(ctx context.Context, instance, number, text string, quote *evolution.Quote) (*evolution.SendTextResult, error)
	SendMedia(ctx context.Context, instance, number, mediaURL, mediatype, caption, fileName string, quote *evolution.Quote) (*evolution.SendTextResult, error)
}

// Attachment es un archivo que acompaña al mensaje.
type Attachment struct {
	FileName string
	MimeType string
	Data     []byte
}

// SendRequest es lo que pide el panel.
type SendRequest struct {
	ConversationID string
	UserID         string
	// IdempotencyKey la genera el cliente. Es lo que hace inofensivo un doble
	// clic o un reintento del navegador.
	IdempotencyKey   string
	Body             string
	Attachment       *Attachment
	ReplyToMessageID string
}

// SendService envía mensajes por una línea y deja constancia de cada intento.
type SendService struct {
	outbound store.OutboundStore
	panel    store.PanelSendTracker
	evo      Sender
	blobs    blobs.Store
	mediaTTL time.Duration
	log      *slog.Logger
}

func NewSendService(
	outbound store.OutboundStore,
	panel store.PanelSendTracker,
	evo Sender,
	store blobs.Store,
	mediaTTL time.Duration,
	log *slog.Logger,
) *SendService {
	return &SendService{
		outbound: outbound, panel: panel, evo: evo,
		blobs: store, mediaTTL: mediaTTL, log: log,
	}
}

// Send manda el mensaje y devuelve el registro del envío.
//
// El orden importa y no es negociable:
//
//  1. Se registra la INTENCIÓN antes de llamar a Evolution. Si el proceso muere
//     a mitad, queda constancia de que se intentó mandar algo.
//  2. Se marca el key.id en Redis ANTES de devolver, para que el webhook que
//     vuelve en segundos encuentre la marca y atribuya el mensaje a su autor.
//
// Un envío ya registrado con la misma clave de idempotencia se devuelve tal cual,
// sin volver a mandar nada.
func (s *SendService) Send(ctx context.Context, req SendRequest) (*store.Outbound, error) {
	target, err := s.outbound.Target(ctx, req.ConversationID)
	if err != nil {
		return nil, err
	}
	if !target.LineActive || target.LineStatus != "open" {
		return nil, ErrLineOffline
	}

	var quote *evolution.Quote
	if req.ReplyToMessageID != "" {
		resolved, err := s.outbound.QuoteTarget(ctx, req.ConversationID, req.ReplyToMessageID)
		if err != nil {
			return nil, err
		}
		quote = &evolution.Quote{
			MessageID:   resolved.WAMessageID,
			Participant: resolved.Participant,
		}
	}

	msgType := domain.TypeText
	if req.Attachment != nil {
		msgType = mediaType(req.Attachment.MimeType)
	}

	// El adjunto se sube ANTES de registrar el envío: si la subida falla, no se
	// deja una fila 'pending' huérfana que nadie va a resolver.
	var storageKey string
	if req.Attachment != nil {
		storageKey = outgoingKey(target.InstanceName, req.IdempotencyKey, req.Attachment)
		if err := s.blobs.Put(ctx, storageKey, req.Attachment.MimeType, req.Attachment.Data); err != nil {
			return nil, fmt.Errorf("subiendo el adjunto: %w", err)
		}
	}

	record, created, err := s.outbound.Create(ctx, &store.Outbound{
		ConversationID: target.ConversationID,
		InstanceID:     target.InstanceID,
		UserID:         req.UserID,
		IdempotencyKey: req.IdempotencyKey,
		Type:           string(msgType),
		Body:           req.Body,
		StorageKey:     storageKey,
	})
	if err != nil {
		return nil, err
	}
	if !created {
		// Doble clic o reintento: ya se mandó (o se está mandando). Devolver el
		// original es justamente lo que evita el mensaje duplicado.
		return record, nil
	}

	result, err := s.dispatch(ctx, target, req, storageKey, msgType, quote)
	if err != nil {
		if markErr := s.outbound.MarkFailed(ctx, record.ID, err.Error()); markErr != nil {
			s.log.Error("no se pudo registrar el fallo de envío",
				"outbound_id", record.ID, "error", markErr)
		}
		record.Status = "failed"
		record.Error = err.Error()
		return record, fmt.Errorf("%w: %v", ErrSendRejected, err)
	}

	// La marca va antes que el UPDATE: el webhook puede llegar en cualquier
	// momento a partir de ahora, y lo que necesita encontrar es esto.
	if err := s.panel.Mark(ctx, result.Key.ID, req.UserID, panelSendTTL); err != nil {
		// No es fatal: outbound_messages permite recuperar el autor después.
		s.log.Warn("no se pudo marcar el envío del panel",
			"wa_message_id", result.Key.ID, "error", err)
	}
	if err := s.outbound.MarkSent(ctx, record.ID, result.Key.ID); err != nil {
		s.log.Error("no se pudo marcar el envío como enviado",
			"outbound_id", record.ID, "error", err)
	}

	record.WAMessageID = result.Key.ID
	record.Status = "sent"
	return record, nil
}

// dispatch elige el endpoint de Evolution según haya adjunto o no.
func (s *SendService) dispatch(
	ctx context.Context, target store.SendTarget, req SendRequest,
	storageKey string, msgType domain.MessageType, quote *evolution.Quote,
) (*evolution.SendTextResult, error) {
	if storageKey == "" {
		return s.evo.SendText(ctx, target.InstanceName, target.RemoteJID, req.Body, quote)
	}

	// Evolution descarga el archivo por su cuenta, así que necesita una URL que
	// alcance desde su contenedor. Se le pasa una firmada de vida corta en vez de
	// hacer público el bucket: por ahí viajan pasaportes.
	url, err := s.blobs.SignedURL(ctx, storageKey, s.mediaTTL)
	if err != nil {
		return nil, fmt.Errorf("firmando el adjunto: %w", err)
	}
	return s.evo.SendMedia(ctx, target.InstanceName, target.RemoteJID,
		url, evolutionMediaType(msgType), req.Body, req.Attachment.FileName, quote)
}

// outgoingKey construye la ruta del adjunto saliente.
//
// El prefijo `outgoing/` lo separa de lo que entra por WhatsApp: son ciclos de
// vida distintos y conviene poder aplicarles retenciones distintas.
func outgoingKey(instance, idempotencyKey string, att *Attachment) string {
	ext := filepath.Ext(att.FileName)
	if ext == "" {
		if exts, err := mime.ExtensionsByType(att.MimeType); err == nil && len(exts) > 0 {
			ext = exts[0]
		}
	}
	if ext == "" {
		ext = ".bin"
	}
	return fmt.Sprintf("outgoing/%s/%s/%s%s",
		sanitizeKey(instance), time.Now().UTC().Format("2006-01"),
		sanitizeKey(idempotencyKey), ext)
}

func sanitizeKey(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, s)
}

// mediaType clasifica el adjunto para nuestro enum message_type.
func mediaType(mimeType string) domain.MessageType {
	switch strings.Split(mimeType, "/")[0] {
	case "image":
		return domain.TypeImage
	case "video":
		return domain.TypeVideo
	case "audio":
		return domain.TypeAudio
	default:
		return domain.TypeDocument
	}
}

// evolutionMediaType traduce al vocabulario de Evolution, que no es el nuestro.
func evolutionMediaType(t domain.MessageType) string {
	switch t {
	case domain.TypeImage:
		return "image"
	case domain.TypeVideo:
		return "video"
	case domain.TypeAudio:
		return "audio"
	default:
		return "document"
	}
}
