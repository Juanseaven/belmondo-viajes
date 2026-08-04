// Package domain define el vocabulario del sistema. No importa nada del
// proyecto: todos los demás paquetes dependen de este y ninguno al revés.
package domain

import "time"

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
	WAMessageID  string
	PushName     string
	Direction    Direction
	Origin       Origin
	Type         MessageType
	Status       MessageStatus
	Body         string
	QuotedID     string
	Timestamp    time.Time
	IsGroup      bool

	// Presente solo si Type.HasMedia(). El binario todavía no se ha descargado.
	Media *MediaInfo
}

// MediaInfo son los metadatos que viajan en el webhook. El archivo en sí NO
// viene: la URL de mmg.whatsapp.net está cifrada y hay que pedirle el binario
// descifrado a Evolution.
type MediaInfo struct {
	MimeType string
	FileName string
	SizeBytes int64
	Duration int
	Width    int
	Height   int
}

// Conversation es el thread materializado por (línea, contacto).
type Conversation struct {
	ID           string
	InstanceID   string
	ContactID    string
	RemoteJID    string
	IsGroup      bool
	LastMessage  time.Time
	UnreadCount  int
}
