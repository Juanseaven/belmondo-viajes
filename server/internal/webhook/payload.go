// Package webhook traduce los payloads de Evolution API v2 al vocabulario de
// domain. Es la única frontera del sistema que conoce el formato de Evolution:
// si algún día se cambia de proveedor, solo este paquete se reescribe.
package webhook

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juanseaven/whatsapp-mirror/server/internal/domain"
)

// Nombres de evento que emite Evolution v2.
const (
	EventMessagesUpsert   = "messages.upsert"
	EventMessagesUpdate   = "messages.update"
	EventMessagesDelete   = "messages.delete"
	EventSendMessage      = "send.message"
	EventContactsUpsert   = "contacts.upsert"
	EventContactsUpdate   = "contacts.update"
	EventChatsUpsert      = "chats.upsert"
	EventChatsUpdate      = "chats.update"
	EventConnectionUpdate = "connection.update"
	EventQRCodeUpdated    = "qrcode.updated"
	EventCall             = "call"
)

// Envelope es la estructura común a todos los webhooks globales.
type Envelope struct {
	Event    string          `json:"event"`
	Instance string          `json:"instance"`
	Data     json.RawMessage `json:"data"`
	Sender   string          `json:"sender"`
	APIKey   string          `json:"apikey"`
	DateTime string          `json:"date_time"`
}

// MessageKey identifica un mensaje dentro de una instancia.
type MessageKey struct {
	RemoteJID string `json:"remoteJid"`
	FromMe    bool   `json:"fromMe"`
	ID        string `json:"id"`
	// En grupos, el JID del participante que envió realmente.
	Participant string `json:"participant"`
}

// MessageData es el `data` de messages.upsert / send.message.
type MessageData struct {
	Key              MessageKey      `json:"key"`
	PushName         string          `json:"pushName"`
	Message          json.RawMessage `json:"message"`
	MessageType      string          `json:"messageType"`
	MessageTimestamp flexInt64       `json:"messageTimestamp"`
	Status           string          `json:"status"`
}

// flexInt64 existe porque Evolution manda messageTimestamp unas veces como
// número y otras como string, según el tipo de evento. Sin esto el unmarshal
// falla de forma intermitente y solo para ciertos mensajes.
type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("messageTimestamp inválido %q: %w", s, err)
	}
	*f = flexInt64(n)
	return nil
}

// content refleja el objeto `message` de Baileys. Solo se declaran los campos
// que se usan; el resto se ignora deliberadamente.
type content struct {
	Conversation string `json:"conversation"`

	ExtendedText *struct {
		Text        string       `json:"text"`
		ContextInfo *contextInfo `json:"contextInfo"`
	} `json:"extendedTextMessage"`

	Image *struct {
		Caption  string `json:"caption"`
		Mimetype string `json:"mimetype"`
		FileLength flexInt64 `json:"fileLength"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
	} `json:"imageMessage"`

	// Los audios de WhatsApp llegan como audio/ogg; codecs=opus.
	Audio *struct {
		Mimetype   string    `json:"mimetype"`
		FileLength flexInt64 `json:"fileLength"`
		Seconds    int       `json:"seconds"`
		PTT        bool      `json:"ptt"`
	} `json:"audioMessage"`

	Video *struct {
		Caption    string    `json:"caption"`
		Mimetype   string    `json:"mimetype"`
		FileLength flexInt64 `json:"fileLength"`
		Seconds    int       `json:"seconds"`
	} `json:"videoMessage"`

	Document *struct {
		Caption    string    `json:"caption"`
		FileName   string    `json:"fileName"`
		Mimetype   string    `json:"mimetype"`
		FileLength flexInt64 `json:"fileLength"`
	} `json:"documentMessage"`

	Sticker *struct {
		Mimetype   string    `json:"mimetype"`
		FileLength flexInt64 `json:"fileLength"`
	} `json:"stickerMessage"`

	Location *struct {
		Latitude  float64 `json:"degreesLatitude"`
		Longitude float64 `json:"degreesLongitude"`
		Name      string  `json:"name"`
	} `json:"locationMessage"`

	Contact *struct {
		DisplayName string `json:"displayName"`
	} `json:"contactMessage"`

	Reaction *struct {
		Text string      `json:"text"`
		Key  *MessageKey `json:"key"`
	} `json:"reactionMessage"`
}

type contextInfo struct {
	StanzaID string `json:"stanzaId"`
}

// ParseMessage normaliza un evento de mensaje.
//
// El Origin que devuelve nunca es OriginPanel: aquí no hay forma de saberlo.
// Distinguir un saliente del vendedor de uno del admin requiere consultar Redis,
// y eso lo hace worker.Processor. Desde aquí, todo fromMe es OriginDevice.
func ParseMessage(instance string, raw json.RawMessage) (*domain.Message, error) {
	var d MessageData
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("decodificando data del mensaje: %w", err)
	}
	if d.Key.ID == "" || d.Key.RemoteJID == "" {
		return nil, fmt.Errorf("evento sin key.id o key.remoteJid")
	}

	msg := &domain.Message{
		InstanceName: instance,
		RemoteJID:    d.Key.RemoteJID,
		WAMessageID:  d.Key.ID,
		PushName:     d.PushName,
		Timestamp:    timestampOrNow(int64(d.MessageTimestamp)),
		IsGroup:      strings.HasSuffix(d.Key.RemoteJID, "@g.us"),
		Status:       mapStatus(d.Status),
	}

	if d.Key.FromMe {
		msg.Direction = domain.Outbound
		msg.Origin = domain.OriginDevice
	} else {
		msg.Direction = domain.Inbound
		msg.Origin = domain.OriginContact
	}

	var c content
	if len(d.Message) > 0 {
		if err := json.Unmarshal(d.Message, &c); err != nil {
			// Un tipo de mensaje que no sabemos leer no debe tumbar la ingesta:
			// se registra como unsupported y la conversación sigue completa.
			msg.Type = domain.TypeUnsupported
			return msg, nil
		}
	}
	applyContent(msg, &c)
	return msg, nil
}

func applyContent(msg *domain.Message, c *content) {
	switch {
	case c.Conversation != "":
		msg.Type = domain.TypeText
		msg.Body = c.Conversation

	case c.ExtendedText != nil:
		msg.Type = domain.TypeText
		msg.Body = c.ExtendedText.Text
		if c.ExtendedText.ContextInfo != nil {
			msg.QuotedID = c.ExtendedText.ContextInfo.StanzaID
		}

	case c.Image != nil:
		msg.Type = domain.TypeImage
		msg.Body = c.Image.Caption
		msg.Media = &domain.MediaInfo{
			MimeType:  c.Image.Mimetype,
			SizeBytes: int64(c.Image.FileLength),
			Width:     c.Image.Width,
			Height:    c.Image.Height,
		}

	case c.Audio != nil:
		msg.Type = domain.TypeAudio
		msg.Media = &domain.MediaInfo{
			MimeType:  c.Audio.Mimetype,
			SizeBytes: int64(c.Audio.FileLength),
			Duration:  c.Audio.Seconds,
		}

	case c.Video != nil:
		msg.Type = domain.TypeVideo
		msg.Body = c.Video.Caption
		msg.Media = &domain.MediaInfo{
			MimeType:  c.Video.Mimetype,
			SizeBytes: int64(c.Video.FileLength),
			Duration:  c.Video.Seconds,
		}

	case c.Document != nil:
		msg.Type = domain.TypeDocument
		msg.Body = c.Document.Caption
		msg.Media = &domain.MediaInfo{
			MimeType:  c.Document.Mimetype,
			FileName:  c.Document.FileName,
			SizeBytes: int64(c.Document.FileLength),
		}

	case c.Sticker != nil:
		msg.Type = domain.TypeSticker
		msg.Media = &domain.MediaInfo{
			MimeType:  c.Sticker.Mimetype,
			SizeBytes: int64(c.Sticker.FileLength),
		}

	case c.Location != nil:
		msg.Type = domain.TypeLocation
		msg.Body = fmt.Sprintf("%s (%f,%f)", c.Location.Name, c.Location.Latitude, c.Location.Longitude)

	case c.Contact != nil:
		msg.Type = domain.TypeContact
		msg.Body = c.Contact.DisplayName

	case c.Reaction != nil:
		msg.Type = domain.TypeReaction
		msg.Body = c.Reaction.Text
		if c.Reaction.Key != nil {
			msg.QuotedID = c.Reaction.Key.ID
		}

	default:
		msg.Type = domain.TypeUnsupported
	}
}

func mapStatus(s string) domain.MessageStatus {
	switch strings.ToUpper(s) {
	case "PENDING":
		return domain.StatusPending
	case "SERVER_ACK", "SENT":
		return domain.StatusSent
	case "DELIVERY_ACK", "DELIVERED":
		return domain.StatusDelivered
	case "READ", "PLAYED":
		return domain.StatusRead
	case "ERROR", "FAILED":
		return domain.StatusFailed
	default:
		return domain.StatusSent
	}
}

// timestampOrNow protege el orden de la conversación: un mensaje sin timestamp
// válido se ancla al momento de recepción en vez de caer a 1970 y romper el
// scroll de la bandeja.
func timestampOrNow(unix int64) time.Time {
	if unix <= 0 {
		return time.Now().UTC()
	}
	return time.Unix(unix, 0).UTC()
}

// PartitionKey determina el shard del pool de workers. Todos los eventos de una
// misma conversación comparten clave, así que se procesan en orden.
func PartitionKey(instance, remoteJID string) string {
	return instance + "|" + remoteJID
}

// DedupeKey es la barrera de idempotencia contra los reintentos de Evolution
// (hasta 10 por evento, con backoff exponencial).
func DedupeKey(instance, event, messageID string) string {
	return instance + ":" + event + ":" + messageID
}
