package store

import (
	"context"
	"time"
)

// Outbound es un envío originado en el panel.
//
// Se registra la INTENCIÓN antes de llamar a Evolution, no después. Así, si el
// proceso muere entre el POST y la respuesta, queda constancia de que se intentó
// mandar algo: sin esta fila, un envío perdido no deja rastro en ningún lado,
// porque `messages` solo se puebla cuando WhatsApp devuelve el eco.
type Outbound struct {
	ID             string
	ConversationID string
	InstanceID     string
	UserID         string
	IdempotencyKey string

	Type       string // text | image | video | audio | document
	Body       string
	StorageKey string // adjunto ya subido; vacío si es solo texto

	WAMessageID string
	Status      string // pending | sent | failed
	Error       string
	SentAt      *time.Time
	CreatedAt   time.Time
}

// SendTarget es lo que hace falta para enviar por una línea, resuelto en una sola
// consulta desde la conversación.
type SendTarget struct {
	ConversationID string
	InstanceID     string
	InstanceName   string // evolution_name: la clave hacia Evolution
	RemoteJID      string
	LineStatus     string // si no es 'open', la línea no puede enviar
	LineActive     bool
}

// OutboundStore registra los envíos del panel.
type OutboundStore interface {
	// Target resuelve línea y destinatario de una conversación.
	Target(ctx context.Context, conversationID string) (SendTarget, error)

	// Create registra la intención de envío.
	//
	// Si la idempotencyKey ya existía devuelve la fila previa con created=false, y
	// quien llama NO debe volver a mandar a Evolution. Es lo que hace inofensivo
	// un doble clic o un reintento del navegador: la autoridad es el índice único
	// de la base, no una comprobación previa que tendría carrera.
	Create(ctx context.Context, o *Outbound) (result *Outbound, created bool, err error)

	MarkSent(ctx context.Context, id, waMessageID string) error
	MarkFailed(ctx context.Context, id, cause string) error

	// UserByWAMessageID recupera el autor de un envío del panel.
	//
	// Es la red de seguridad de la atribución: el camino normal es la marca en
	// Redis, pero si esa marca se perdió —Redis reiniciado, o el webhook ganó la
	// carrera— esta consulta la recupera de la base. Devuelve ("", false) si el
	// mensaje no salió del panel.
	UserByWAMessageID(ctx context.Context, instanceID, waMessageID string) (userID string, ok bool)
}
