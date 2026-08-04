// Package store define los puertos de persistencia. Las implementaciones viven
// en los subpaquetes (postgres, rediscache); la lógica de negocio depende solo de
// estas interfaces, lo que permite testear worker y httpapi con dobles.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/juanseaven/whatsapp-mirror/server/internal/domain"
)

// ErrDuplicate indica que el evento ya se había registrado: es un reintento de
// Evolution y debe descartarse sin procesar.
var ErrDuplicate = errors.New("evento duplicado")

// ErrNotFound se devuelve cuando una fila esperada no existe.
var ErrNotFound = errors.New("no encontrado")

// EventStore es la barrera de idempotencia y la bitácora forense.
type EventStore interface {
	// Insert registra el webhook crudo. Devuelve ErrDuplicate si dedupeKey ya
	// existía — así el índice único de la base, y no la aplicación, es la
	// autoridad sobre qué es un duplicado.
	Insert(ctx context.Context, instance, event, dedupeKey string, payload []byte) (int64, error)
	MarkProcessed(ctx context.Context, id int64) error
	MarkFailed(ctx context.Context, id int64, cause string) error
}

// PersistedMessage es el resultado de guardar un mensaje: los ids que necesita el
// resto del pipeline (media y websockets).
type PersistedMessage struct {
	MessageID      string
	ConversationID string
	InstanceID     string
	ContactID      string
	MediaAssetID   string // vacío si el mensaje no lleva adjunto
	IsNew          bool   // false si ya existía (reintento que burló el dedupe)
}

// MessageStore persiste mensajes normalizados.
type MessageStore interface {
	// Save resuelve instancia y contacto, materializa la conversación, inserta el
	// mensaje y actualiza los contadores de la bandeja. Todo en UNA transacción:
	// una conversación con `last_message_at` que no corresponde a ningún mensaje
	// deja la bandeja mintiendo.
	Save(ctx context.Context, msg *domain.Message, sentByUserID string) (*PersistedMessage, error)
	UpdateStatus(ctx context.Context, instanceName, waMessageID string, status domain.MessageStatus) error
	MarkDeleted(ctx context.Context, instanceName, waMessageID string) error
}

// InstanceStore gestiona las líneas conectadas.
type InstanceStore interface {
	IDByName(ctx context.Context, evolutionName string) (string, error)
	UpdateConnection(ctx context.Context, evolutionName, status, phoneJID string) error
	Touch(ctx context.Context, evolutionName string, at time.Time) error
}

// MediaStore lleva el estado del pipeline de subida a R2.
type MediaStore interface {
	NextPending(ctx context.Context, limit int) ([]PendingMedia, error)
	MarkProcessing(ctx context.Context, id string) error
	MarkStored(ctx context.Context, id, storageKey, sha256 string, size int64) error
	MarkFailed(ctx context.Context, id, cause string) error
	StorageKey(ctx context.Context, id string) (string, string, error) // key, mime
}

// PendingMedia es un adjunto pendiente de descargar de Evolution y subir a R2.
type PendingMedia struct {
	ID           string
	MessageID    string
	InstanceName string
	WAMessageID  string
	MimeType     string
	FileName     string
	Attempts     int
}

// PanelSendTracker distingue los salientes del panel de los del teléfono del
// vendedor. Ambos llegan por webhook con fromMe:true y son indistinguibles en el
// payload; sin esto la auditoría no puede decir quién habló.
//
// Se implementa sobre Redis con TTL: es un rendezvous de vida corta entre el
// POST a Evolution y el webhook que vuelve segundos después.
type PanelSendTracker interface {
	// Mark se llama JUSTO tras recibir el key.id de Evolution, antes de que el
	// webhook pueda llegar.
	Mark(ctx context.Context, waMessageID, userID string, ttl time.Duration) error
	// Claim consume la marca. Devuelve ("", false) si el mensaje no salió del
	// panel, es decir: lo escribió el vendedor desde su móvil.
	Claim(ctx context.Context, waMessageID string) (userID string, ok bool)
}
