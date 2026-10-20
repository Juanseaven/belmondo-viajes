// Package ingest es la lógica de negocio de la ingesta de webhooks.
//
// Vive separado de worker a propósito: worker es un pool particionado genérico
// que no sabe nada de WhatsApp, e ingest es lo que se ejecuta dentro. Mezclarlos
// creaba un ciclo de imports (webhook necesita worker.Job para encolar; el
// processor necesita webhook para parsear), y ese ciclo era la señal de que eran
// dos responsabilidades distintas.
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/ws"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/webhook"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/worker"
)

const offlineReplayGrace = 5 * time.Minute

// MediaEnqueuer recibe los adjuntos pendientes de bajar de Evolution y subir a R2.
type MediaEnqueuer interface {
	Enqueue(job store.PendingMedia)
}

// Broadcaster es el hub de websockets visto por el processor.
type Broadcaster interface {
	Broadcast(ev ws.Event)
}

// QRStore guarda el último código QR de una línea en vinculación.
type QRStore interface {
	Set(ctx context.Context, instanceName, code string) error
	Clear(ctx context.Context, instanceName string)
}

// OutboundLookup es la red de seguridad de la atribución: recupera de la base el
// autor de un envío cuya marca en Redis se perdió.
type OutboundLookup interface {
	UserByWAMessageID(ctx context.Context, instanceID, waMessageID string) (string, bool)
}

// DirectoryProvider exposes the address book synchronized by one Evolution
// instance. These names are line-specific and differ from public push names.
type DirectoryProvider interface {
	Contacts(ctx context.Context, instance string) ([]domain.ContactIdentity, error)
}

// Processor implementa worker.Handler. Traduce el payload, resuelve el origen
// real del mensaje, persiste y notifica.
type Processor struct {
	events    store.EventStore
	messages  store.MessageStore
	contacts  store.ContactStore
	directory DirectoryProvider
	instances store.InstanceStore
	panel     store.PanelSendTracker
	outbound  OutboundLookup
	qr        QRStore
	media     MediaEnqueuer
	hub       Broadcaster
	log       *slog.Logger
}

func NewProcessor(
	events store.EventStore,
	messages store.MessageStore,
	contacts store.ContactStore,
	directory DirectoryProvider,
	instances store.InstanceStore,
	panel store.PanelSendTracker,
	outbound OutboundLookup,
	qr QRStore,
	media MediaEnqueuer,
	hub Broadcaster,
	log *slog.Logger,
) *Processor {
	return &Processor{
		events: events, messages: messages, contacts: contacts, directory: directory, instances: instances,
		panel: panel, outbound: outbound, qr: qr,
		media: media, hub: hub, log: log,
	}
}

func (p *Processor) Handle(ctx context.Context, job worker.Job) error {
	var err error
	switch job.Event {
	case webhook.EventMessage, webhook.EventSendMessage:
		err = p.handleMessage(ctx, job)
	case webhook.EventReceipt:
		err = p.handleReceipt(ctx, job)
	case webhook.EventHistorySync:
		err = p.handleHistorySync(ctx, job)
	case webhook.EventContact, webhook.EventPushName:
		err = p.refreshDirectory(ctx, job.Instance)
	case webhook.EventGroupInfo, webhook.EventJoinedGroup:
		err = p.handleGroupUpdate(ctx, job)
	case webhook.EventConnected, webhook.EventPairSuccess,
		webhook.EventDisconnected, webhook.EventLoggedOut, webhook.EventConnectFailure:
		err = p.handleConnection(ctx, job)
	case webhook.EventQRCode:
		err = p.handleQRCode(ctx, job)
	case webhook.EventOfflineSyncCompleted:
		err = p.handleOfflineSyncCompleted(ctx, job)
	case webhook.EventQRTimeout:
		p.qr.Clear(ctx, job.Instance)
		p.broadcast("line.qr_timeout", "", map[string]any{"instance": job.Instance})
	default:
		// Evento suscrito pero sin tratamiento propio. Queda registrado en
		// webhook_events, que es donde vive el valor forense.
		return p.events.MarkProcessed(ctx, job.EventID)
	}

	if err != nil {
		if markErr := p.events.MarkFailed(ctx, job.EventID, err.Error()); markErr != nil {
			p.log.Error("no se pudo marcar el evento como fallido", "error", markErr)
		}
		return err
	}
	return p.events.MarkProcessed(ctx, job.EventID)
}

func (p *Processor) handleMessage(ctx context.Context, job worker.Job) error {
	msg, err := webhook.ParseMessage(job.Instance, job.Payload)
	if err != nil {
		return fmt.Errorf("parseando mensaje: %w", err)
	}
	replay := isOfflineReplay(job.ReceivedAt, msg.Timestamp)
	if err := p.persistMessage(ctx, job, msg, !replay); err != nil {
		return err
	}
	if replay {
		p.broadcast("history.sync", "", map[string]any{"instance": job.Instance})
	}
	return nil
}

func isOfflineReplay(receivedAt, messageAt time.Time) bool {
	return !receivedAt.IsZero() && !messageAt.IsZero() && receivedAt.Sub(messageAt) > offlineReplayGrace
}

func (p *Processor) persistMessage(ctx context.Context, job worker.Job, msg *domain.Message, notify bool) error {
	// --- Resolución del origen real ------------------------------------------
	// webhook.ParseMessage marca todo fromMe como OriginDevice porque desde el
	// payload no hay más información: WhatsApp entrega igual lo que se escribió
	// desde el teléfono y lo que salió de esta app.
	//
	// Dos fuentes, en orden:
	//   1. La marca en Redis, que el envío deja justo tras recibir el key.id.
	//   2. outbound_messages, si esa marca no está.
	//
	// El segundo camino no es redundante: cubre que Redis se haya reiniciado o
	// que el webhook le haya ganado la carrera al Mark. Sin él, esos mensajes se
	// atribuirían al teléfono y la trazabilidad mentiría en silencio.
	var sentBy string
	if msg.Direction == domain.Outbound && !msg.Historical {
		if userID, ok := p.panel.Claim(ctx, msg.WAMessageID); ok {
			msg.Origin = domain.OriginPanel
			sentBy = userID
		} else if userID, ok := p.outboundAuthor(ctx, job.Instance, msg.WAMessageID); ok {
			msg.Origin = domain.OriginPanel
			sentBy = userID
		}
	}

	saved, err := p.messages.Save(ctx, msg, sentBy)
	if err != nil {
		return fmt.Errorf("persistiendo mensaje: %w", err)
	}
	if !saved.IsNew {
		return nil // ya estaba: reintento que burló el dedupe
	}

	// El adjunto se encola FUERA de la transacción y sin esperar. El mensaje ya
	// aparece en el panel; el archivo llegará en segundos. Descargarlo aquí
	// bloquearía toda la conversación durante la subida a R2.
	if saved.MediaAssetID != "" && p.media != nil {
		p.media.Enqueue(store.PendingMedia{
			ID:           saved.MediaAssetID,
			MessageID:    saved.MessageID,
			InstanceName: job.Instance,
			WAMessageID:  msg.WAMessageID,
			MimeType:     mediaMime(msg),
			FileName:     mediaFileName(msg),
			Data:         mediaData(msg),
			SourceURL:    mediaSourceURL(msg),
		})
	}

	if !notify {
		return nil
	}
	p.broadcast("message.new", saved.InstanceID, map[string]any{
		"messageId":      saved.MessageID,
		"conversationId": saved.ConversationID,
		"instanceId":     saved.InstanceID,
		// El id de WhatsApp es la única clave con la que el panel puede emparejar
		// el mensaje provisional que pintó al enviar con este, el confirmado.
		"waMessageId": msg.WAMessageID,
		"direction":   msg.Direction,
		"origin":      msg.Origin,
		"type":        msg.Type,
		"body":        msg.Body,
		"quotedId":    msg.QuotedID,
		"senderJid":   msg.SenderJID,
		"senderName":  firstNonEmpty(saved.SenderName, msg.SenderName),
		"senderPhone": saved.SenderPhone,
		"hasMedia":    saved.MediaAssetID != "",
		"timestamp":   msg.Timestamp,
	})
	return nil
}

func (p *Processor) handleHistorySync(ctx context.Context, job worker.Job) error {
	contacts, err := webhook.ParseHistorySyncContacts(job.Payload)
	if err != nil {
		return err
	}
	if p.contacts != nil && len(contacts) > 0 {
		if err := p.contacts.UpsertDirectory(ctx, job.Instance, contacts); err != nil {
			return fmt.Errorf("guardando directorio de WhatsApp: %w", err)
		}
	}
	messages, progress, err := webhook.ParseHistorySync(job.Instance, job.Payload)
	if err != nil {
		return err
	}
	imported := 0
	for _, msg := range messages {
		msg.Historical = true
		if msg.Type.HasMedia() {
			msg.Media = nil
		}
		if err := p.persistMessage(ctx, job, msg, false); err != nil {
			return fmt.Errorf("importando mensaje histórico %s: %w", msg.WAMessageID, err)
		}
		imported++
	}
	if progress == 100 {
		if err := p.refreshDirectory(ctx, job.Instance); err != nil {
			return err
		}
	}
	p.broadcast("history.sync", "", map[string]any{
		"instance": job.Instance,
		"progress": progress,
		"imported": imported,
	})
	return nil
}

// outboundAuthor consulta la base cuando la marca de Redis no está.
func (p *Processor) outboundAuthor(ctx context.Context, instanceName, waMessageID string) (string, bool) {
	if p.outbound == nil {
		return "", false
	}
	instanceID, err := p.instances.IDByName(ctx, instanceName)
	if err != nil {
		return "", false
	}
	return p.outbound.UserByWAMessageID(ctx, instanceID, waMessageID)
}

// handleQRCode recoge el código de vinculación y lo emite al panel.
//
// El QR rota cada ~30 s, así que esto se dispara varias veces por alta. Se guarda
// el último en Redis con TTL para que abrir el modal muestre algo al instante en
// vez de esperar al siguiente.
func (p *Processor) handleQRCode(ctx context.Context, job worker.Job) error {
	var d struct {
		Base64 string `json:"base64"`
		QRCode string `json:"qrcode"`
		Code   string `json:"code"`
	}
	if err := json.Unmarshal(job.Payload, &d); err != nil {
		return fmt.Errorf("parseando el qr: %w", err)
	}

	// Evolution Go entrega la imagen como data URL en `qrcode`.
	image := firstNonEmpty(d.QRCode, d.Base64)
	if image == "" {
		// Sin imagen no hay nada que mostrar, pero tampoco es un fallo: el evento
		// queda registrado y el siguiente QR llega en segundos.
		p.log.Warn("qrcode.updated sin imagen", "instance", job.Instance)
		return nil
	}

	if err := p.qr.Set(ctx, job.Instance, image); err != nil {
		p.log.Warn("no se pudo guardar el qr", "instance", job.Instance, "error", err)
	}

	p.broadcast("line.qr", "", map[string]any{
		"instance": job.Instance,
		"qr":       image,
	})
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (p *Processor) handleReceipt(ctx context.Context, job worker.Job) error {
	var d struct {
		MessageIDs []string `json:"MessageIDs"`
		Type       string   `json:"Type"`
		Chat       string   `json:"Chat"`
		IsFromMe   bool     `json:"IsFromMe"`
	}
	if err := json.Unmarshal(job.Payload, &d); err != nil {
		return fmt.Errorf("parseando recibo: %w", err)
	}
	receiptType := firstNonEmpty(d.Type, job.State)
	for _, id := range d.MessageIDs {
		if err := p.messages.UpdateStatus(ctx, job.Instance, id, receiptStatus(receiptType)); err != nil {
			return err
		}
	}
	if d.IsFromMe && d.Chat != "" && isReadReceipt(receiptType) {
		conversationIDs, err := p.messages.MarkReadFromDevice(ctx, job.Instance, d.Chat)
		if err != nil {
			return fmt.Errorf("aplicando lectura desde WhatsApp: %w", err)
		}
		for _, conversationID := range conversationIDs {
			p.broadcast("conversation.read", "", map[string]any{"conversationId": conversationID})
		}
	}
	return nil
}

func isReadReceipt(value string) bool {
	switch strings.ToLower(value) {
	case "read", "read-self":
		return true
	default:
		return false
	}
}

func (p *Processor) handleDelete(ctx context.Context, job worker.Job) error {
	var d struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
	}
	if err := json.Unmarshal(job.Payload, &d); err != nil {
		return err
	}
	if d.Key.ID == "" {
		return nil
	}
	// Soft delete: que un vendedor borre un mensaje para el cliente no debe
	// borrarlo del registro de auditoría. Ese borrado es, en sí, información.
	return p.messages.MarkDeleted(ctx, job.Instance, d.Key.ID)
}

func (p *Processor) handleConnection(ctx context.Context, job worker.Job) error {
	var d struct {
		Status string `json:"status"`
		JID    string `json:"jid"`
	}
	if err := json.Unmarshal(job.Payload, &d); err != nil {
		return err
	}
	state := "close"
	if job.Event == webhook.EventConnected || job.Event == webhook.EventPairSuccess {
		state = "open"
	}
	if err := p.instances.UpdateConnection(ctx, job.Instance, state, d.JID); err != nil {
		return err
	}
	_ = p.instances.Touch(ctx, job.Instance, time.Now())

	// Una línea caída es lo más urgente que puede pasar: mientras esté así, la
	// agencia no ve ni un mensaje de ese vendedor.
	if state == "close" {
		p.log.Warn("línea desconectada", "instance", job.Instance)
	}
	if state == "open" {
		// Ya vinculada: el QR guardado dejó de servir y mostrarlo sería ofrecer un
		// código muerto.
		p.qr.Clear(ctx, job.Instance)
		if err := p.refreshDirectory(ctx, job.Instance); err != nil {
			p.log.Warn("no se pudo refrescar la agenda al conectar", "instance", job.Instance, "error", err)
		}
	}
	p.broadcast("instance.status", "", map[string]any{
		"instance": job.Instance,
		"state":    state,
	})
	return nil
}

func (p *Processor) handleOfflineSyncCompleted(ctx context.Context, job worker.Job) error {
	var d struct {
		Count int `json:"Count"`
	}
	_ = json.Unmarshal(job.Payload, &d)
	if p.instances != nil {
		_ = p.instances.Touch(ctx, job.Instance, time.Now())
	}
	if err := p.refreshDirectory(ctx, job.Instance); err != nil {
		return err
	}
	p.broadcast("history.sync", "", map[string]any{
		"instance": job.Instance,
		"progress": 100,
		"complete": true,
		"count":    d.Count,
	})
	return nil
}

func (p *Processor) handleGroupUpdate(ctx context.Context, job worker.Job) error {
	var data struct {
		JID  string `json:"JID"`
		Name struct {
			Name string `json:"Name"`
		} `json:"Name"`
		GroupName struct {
			Name string `json:"Name"`
		} `json:"GroupName"`
	}
	if err := json.Unmarshal(job.Payload, &data); err != nil {
		return fmt.Errorf("parseando actualización de grupo: %w", err)
	}
	name := firstNonEmpty(data.Name.Name, data.GroupName.Name)
	if data.JID == "" || name == "" || p.contacts == nil {
		return nil
	}
	if err := p.contacts.SetGroupName(ctx, job.Instance, data.JID, name); err != nil {
		return fmt.Errorf("guardando nombre del grupo: %w", err)
	}
	p.broadcast("history.sync", "", map[string]any{"instance": job.Instance})
	return nil
}

func (p *Processor) refreshDirectory(ctx context.Context, instance string) error {
	if p.directory == nil || p.contacts == nil {
		return nil
	}
	contacts, err := p.directory.Contacts(ctx, instance)
	if err != nil {
		return fmt.Errorf("leyendo agenda de Evolution: %w", err)
	}
	if len(contacts) == 0 {
		return nil
	}
	if err := p.contacts.UpsertDirectory(ctx, instance, contacts); err != nil {
		return fmt.Errorf("guardando agenda de la línea: %w", err)
	}
	return nil
}

func (p *Processor) broadcast(evType, instanceID string, payload map[string]any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		p.log.Error("no se pudo serializar el evento ws", "error", err)
		return
	}
	p.hub.Broadcast(ws.Event{
		Module:     domain.ModuleName,
		Type:       evType,
		InstanceID: instanceID,
		Payload:    raw,
		At:         time.Now().UTC(),
	})
}

func mediaMime(msg *domain.Message) string {
	if msg.Media == nil {
		return ""
	}
	return msg.Media.MimeType
}

func mediaFileName(msg *domain.Message) string {
	if msg.Media == nil {
		return ""
	}
	return msg.Media.FileName
}

func mediaData(msg *domain.Message) []byte {
	if msg.Media == nil {
		return nil
	}
	return msg.Media.Data
}

func mediaSourceURL(msg *domain.Message) string {
	if msg.Media == nil {
		return ""
	}
	return msg.Media.SourceURL
}

// mapStatus traduce los ACK de Baileys. Ojo: no es el mismo mapeo que el de
// webhook.ParseMessage, que trabaja sobre el campo `status` del mensaje inicial.
func mapStatus(s string) domain.MessageStatus {
	switch s {
	case "PENDING":
		return domain.StatusPending
	case "SERVER_ACK":
		return domain.StatusSent
	case "DELIVERY_ACK":
		return domain.StatusDelivered
	case "READ", "PLAYED":
		return domain.StatusRead
	case "ERROR":
		return domain.StatusFailed
	default:
		return domain.StatusSent
	}
}

func receiptStatus(value string) domain.MessageStatus {
	switch strings.ToLower(value) {
	case "delivered", "delivery_ack":
		return domain.StatusDelivered
	case "read", "readself", "read-self", "played":
		return domain.StatusRead
	case "error", "failed":
		return domain.StatusFailed
	default:
		return domain.StatusSent
	}
}
