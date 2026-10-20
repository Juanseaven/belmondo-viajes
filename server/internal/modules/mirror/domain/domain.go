// Package domain define el vocabulario del módulo espejo. No importa nada del
// proyecto: todos los demás paquetes del módulo dependen de este y ninguno al
// revés.
package domain

import "time"

// ModuleName es la identidad del módulo: prefijo de rutas, clave en
// schema_migrations y remitente en los eventos de websocket.
//
// Vive aquí, en la hoja del árbol de dependencias, para que tanto el paquete
// raíz del módulo como los internos puedan usarla sin ciclos.
const ModuleName = "mirror"

type Direction string

const (
	Inbound  Direction = "inbound"
	Outbound Direction = "outbound"
)

// Origin es el eje de la auditoría. WhatsApp marca como fromMe:true tanto lo que
// el vendedor escribe desde su teléfono como lo que el admin envía desde el
// panel; el payload no los distingue. Ver worker.Processor para cómo se resuelve.
type Origin string

const (
	// OriginContact: lo escribió el cliente final.
	OriginContact Origin = "contact"
	// OriginDevice: lo escribió el vendedor desde su móvil. El caso normal en un
	// espejo pasivo.
	OriginDevice Origin = "device"
	// OriginPanel: lo envió un administrador desde esta aplicación.
	OriginPanel Origin = "panel"
)

type MessageType string

const (
	TypeText        MessageType = "text"
	TypeImage       MessageType = "image"
	TypeAudio       MessageType = "audio"
	TypeVideo       MessageType = "video"
	TypeDocument    MessageType = "document"
	TypeSticker     MessageType = "sticker"
	TypeLocation    MessageType = "location"
	TypeContact     MessageType = "contact"
	TypeReaction    MessageType = "reaction"
	TypeSystem      MessageType = "system"
	TypeUnsupported MessageType = "unsupported"
)

// HasMedia indica si el tipo requiere pasar por el pipeline de descarga y subida
// a R2.
func (t MessageType) HasMedia() bool {
	switch t {
	case TypeImage, TypeAudio, TypeVideo, TypeDocument, TypeSticker:
		return true
	default:
		return false
	}
}

type MessageStatus string

const (
	StatusPending   MessageStatus = "pending"
	StatusSent      MessageStatus = "sent"
	StatusDelivered MessageStatus = "delivered"
	StatusRead      MessageStatus = "read"
	StatusFailed    MessageStatus = "failed"
)

// Message es un mensaje ya normalizado, listo para persistir. Es lo que produce
// el paquete webhook a partir del payload crudo de Evolution.
type Message struct {
	InstanceName string
	RemoteJID    string
	// AlternateJID links the phone JID of a private chat with WhatsApp's opaque
	// LID when a live inbound event carries both identities.
	AlternateJID string
	WAMessageID  string
	PushName     string
	// SenderJID/SenderName identify the participant inside a group. RemoteJID is
	// the group itself, so without these fields every inbound bubble loses its
	// author. They stay empty for private chats.
	SenderJID  string
	SenderName string
	Direction  Direction
	Origin     Origin
	Type       MessageType
	Status     MessageStatus
	Body       string
	QuotedID   string
	Timestamp  time.Time
	IsGroup    bool
	Historical bool

	// Presente solo si Type.HasMedia(). El binario todavía no se ha descargado.
	Media *MediaInfo
}

// ContactIdentity is one entry from WhatsApp's history-sync directory. Modern
// group messages identify participants with an opaque @lid JID; this mapping is
// what lets the read side resolve that LID back to a named phone contact.
type ContactIdentity struct {
	PhoneJID     string
	LIDJID       string
	Name         string // push name from HistorySync; kept for compatibility
	FirstName    string
	FullName     string
	PushName     string
	BusinessName string
}

// MediaInfo son los metadatos que viajan en el webhook. El archivo en sí NO
// viene: la URL de mmg.whatsapp.net está cifrada y hay que pedirle el binario
// descifrado a Evolution.
type MediaInfo struct {
	MimeType  string
	FileName  string
	SizeBytes int64
	Duration  int
	Width     int
	Height    int
	Data      []byte
	SourceURL string
}

// Conversation es el thread materializado por (línea, contacto).
type Conversation struct {
	ID          string
	InstanceID  string
	ContactID   string
	RemoteJID   string
	IsGroup     bool
	LastMessage time.Time
	UnreadCount int
}
