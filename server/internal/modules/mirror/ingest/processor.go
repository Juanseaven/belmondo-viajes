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
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/ws"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/webhook"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/worker"
)

// MediaEnqueuer recibe los adjuntos pendientes de bajar de Evolution y subir a R2.
type MediaEnqueuer interface {
	Enqueue(job store.PendingMedia)
}

// Broadcaster es el hub de websockets visto por el processor.
type Broadcaster interface {
	Broadcast(ev ws.Event)
}

// Processor implementa worker.Handler. Traduce el payload, resuelve el origen
// real del mensaje, persiste y notifica.
type Processor struct {
	events    store.EventStore
	messages  store.MessageStore
	instances store.InstanceStore
	panel     store.PanelSendTracker
	media     MediaEnqueuer
	hub       Broadcaster
	log       *slog.Logger
}

func NewProcessor(
	events store.EventStore,
	messages store.MessageStore,
	instances store.InstanceStore,
	panel store.PanelSendTracker,
	media MediaEnqueuer,
	hub Broadcaster,
	log *slog.Logger,
) *Processor {
	return &Processor{
		events: events, messages: messages, instances: instances,
		panel: panel, media: media, hub: hub, log: log,
	}
}

func (p *Processor) Handle(ctx context.Context, job worker.Job) error {
	var err error
	switch job.Event {
	case webhook.EventMessagesUpsert, webhook.EventSendMessage:
		err = p.handleMessage(ctx, job)
	case webhook.EventMessagesUpdate:
		err = p.handleStatusUpdate(ctx, job)
	case webhook.EventMessagesDelete:
		err = p.handleDelete(ctx, job)
	case webhook.EventConnectionUpdate:
		err = p.handleConnection(ctx, job)
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

	// --- Resolución del origen real ------------------------------------------
	// webhook.ParseMessage marca todo fromMe como OriginDevice porque desde el
	// payload no hay más información. Aquí se corrige: si el id está marcado en
	// Redis, el mensaje salió de este panel.
	//
	// Si Redis se hubiera perdido, el peor caso es clasificar un envío del panel
	// como 'device'. Degradación silenciosa, nunca corrupción.
	var sentBy string
	if msg.Direction == domain.Outbound {
		if userID, ok := p.panel.Claim(ctx, msg.WAMessageID); ok {
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
	if saved.MediaAssetID != "" {
		p.media.Enqueue(store.PendingMedia{
			ID:           saved.MediaAssetID,
			MessageID:    saved.MessageID,
			InstanceName: job.Instance,
			WAMessageID:  msg.WAMessageID,
			MimeType:     mediaMime(msg),
			FileName:     mediaFileName(msg),
		})
	}

	p.broadcast("message.new", saved.InstanceID, map[string]any{
		"messageId":      saved.MessageID,
		"conversationId": saved.ConversationID,
		"instanceId":     saved.InstanceID,
		"direction":      msg.Direction,
		"origin":         msg.Origin,
		"type":           msg.Type,
		"body":           msg.Body,
		"hasMedia":       saved.MediaAssetID != "",
		"timestamp":      msg.Timestamp,
	})
	return nil
}

func (p *Processor) handleStatusUpdate(ctx context.Context, job worker.Job) error {
	var d struct {
		KeyID string `json:"keyId"`
		Key   struct {
			ID string `json:"id"`
		} `json:"key"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(job.Payload, &d); err != nil {
		return fmt.Errorf("parseando update de estado: %w", err)
	}
	id := d.KeyID
	if id == "" {
		id = d.Key.ID
	}
	if id == "" {
		return nil
	}
	return p.messages.UpdateStatus(ctx, job.Instance, id, mapStatus(d.Status))
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
		State string `json:"state"`
		WUID  string `json:"wuid"`
	}
	if err := json.Unmarshal(job.Payload, &d); err != nil {
		return err
	}
	if err := p.instances.UpdateConnection(ctx, job.Instance, d.State, d.WUID); err != nil {
		return err
	}
	_ = p.instances.Touch(ctx, job.Instance, time.Now())

	// Una línea caída es lo más urgente que puede pasar: mientras esté así, la
	// agencia no ve ni un mensaje de ese vendedor.
	if d.State == "close" {
		p.log.Warn("línea desconectada", "instance", job.Instance)
	}
	p.broadcast("instance.status", "", map[string]any{
		"instance": job.Instance,
		"state":    d.State,
	})
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
