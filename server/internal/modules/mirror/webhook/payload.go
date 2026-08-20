// Package webhook translates Evolution Go payloads into the mirror domain.
package webhook

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
)

const (
	EventMessage              = "Message"
	EventSendMessage          = "SendMessage"
	EventReceipt              = "Receipt"
	EventHistorySync          = "HistorySync"
	EventConnected            = "Connected"
	EventPairSuccess          = "PairSuccess"
	EventOfflineSyncCompleted = "OfflineSyncCompleted"
	EventDisconnected         = "Disconnected"
	EventLoggedOut            = "LoggedOut"
	EventConnectFailure       = "ConnectFailure"
	EventQRCode               = "QRCode"
	EventQRTimeout            = "QRTimeout"
	EventQRSuccess            = "QRSuccess"
	EventContact              = "Contact"
	EventPushName             = "PushName"
	EventGroupInfo            = "GroupInfo"
	EventJoinedGroup          = "JoinedGroup"
)

// Envelope is common to every Evolution Go webhook.
type Envelope struct {
	Event         string          `json:"event"`
	State         string          `json:"state"`
	Data          json.RawMessage `json:"data"`
	InstanceID    string          `json:"instanceId"`
	InstanceName  string          `json:"instanceName"`
	InstanceToken string          `json:"instanceToken"`
}

type liveMessageData struct {
	Info struct {
		Chat      string   `json:"Chat"`
		Sender    string   `json:"Sender"`
		SenderAlt string   `json:"SenderAlt"`
		IsFromMe  bool     `json:"IsFromMe"`
		IsGroup   bool     `json:"IsGroup"`
		ID        string   `json:"ID"`
		PushName  string   `json:"PushName"`
		Timestamp flexTime `json:"Timestamp"`
	} `json:"Info"`
	Message json.RawMessage `json:"Message"`
}

type historyKey struct {
	RemoteJID   string `json:"remoteJID"`
	FromMe      bool   `json:"fromMe"`
	ID          string `json:"ID"`
	IDLower     string `json:"id"`
	Participant string `json:"participant"`
}

type historyMessageData struct {
	Key              historyKey      `json:"key"`
	Participant      string          `json:"participant"`
	Message          json.RawMessage `json:"message"`
	MessageTimestamp flexInt64       `json:"messageTimestamp"`
	PushName         string          `json:"pushName"`
	Status           any             `json:"status"`
}

type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("timestamp inválido %q: %w", s, err)
	}
	*f = flexInt64(n)
	return nil
}

type flexTime struct{ time.Time }

func (f *flexTime) UnmarshalJSON(b []byte) error {
	value := strings.Trim(string(b), `"`)
	if value == "" || value == "null" {
		return nil
	}
	if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
		f.Time = time.Unix(unix, 0).UTC()
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fmt.Errorf("timestamp inválido %q: %w", value, err)
	}
	f.Time = parsed.UTC()
	return nil
}

type content struct {
	Conversation string `json:"conversation"`
	Base64       string `json:"base64"`
	MediaURL     string `json:"mediaUrl"`

	ExtendedText *struct {
		Text        string       `json:"text"`
		ContextInfo *contextInfo `json:"contextInfo"`
	} `json:"extendedTextMessage"`

	Image *struct {
		Caption    string    `json:"caption"`
		Mimetype   string    `json:"mimetype"`
		FileLength flexInt64 `json:"fileLength"`
		Width      int       `json:"width"`
		Height     int       `json:"height"`
	} `json:"imageMessage"`

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
		Text string `json:"text"`
		Key  *struct {
			ID      string `json:"ID"`
			IDLower string `json:"id"`
		} `json:"key"`
	} `json:"reactionMessage"`
}

type contextInfo struct {
	StanzaID string `json:"stanzaID"`
	StanzaId string `json:"stanzaId"`
}

// ParseMessage normalizes a live Message or SendMessage event.
func ParseMessage(instance string, raw json.RawMessage) (*domain.Message, error) {
	var data liveMessageData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("decodificando data del mensaje: %w", err)
	}
	if data.Info.ID == "" || data.Info.Chat == "" {
		return nil, fmt.Errorf("evento sin Info.ID o Info.Chat")
	}

	contactName := data.Info.PushName
	if data.Info.IsGroup || strings.HasSuffix(data.Info.Chat, "@g.us") {
		contactName = ""
	}
	msg := baseMessage(instance, data.Info.Chat, data.Info.ID, contactName,
		data.Info.IsFromMe, data.Info.IsGroup, data.Info.Timestamp.Time)
	if msg.IsGroup && !data.Info.IsFromMe {
		msg.SenderJID = preferredPhoneJID(data.Info.Sender, data.Info.SenderAlt)
		msg.SenderName = data.Info.PushName
	}
	if err := decodeContent(msg, data.Message); err != nil {
		return nil, err
	}
	return msg, nil
}

// ParseHistorySync expands every conversation chunk into the same normalized
// messages used by the live ingestion path. WhatsApp may send several chunks.
func ParseHistorySync(instance string, raw json.RawMessage) ([]*domain.Message, int, error) {
	var env struct {
		Data struct {
			Progress  int `json:"progress"`
			PushNames []struct {
				ID   string `json:"ID"`
				Name string `json:"pushname"`
			} `json:"pushnames"`
			Conversations []struct {
				ID       string `json:"ID"`
				Name     string `json:"name"`
				Messages []struct {
					Message historyMessageData `json:"message"`
				} `json:"messages"`
			} `json:"conversations"`
		} `json:"Data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, 0, fmt.Errorf("decodificando history sync: %w", err)
	}

	pushNames := make(map[string]string, len(env.Data.PushNames))
	for _, item := range env.Data.PushNames {
		pushNames[item.ID] = item.Name
	}

	var messages []*domain.Message
	for _, conversation := range env.Data.Conversations {
		for _, item := range conversation.Messages {
			data := item.Message
			id := firstNonEmpty(data.Key.ID, data.Key.IDLower)
			remoteJID := firstNonEmpty(data.Key.RemoteJID, conversation.ID)
			if id == "" || remoteJID == "" {
				continue
			}
			isGroup := strings.HasSuffix(remoteJID, "@g.us")
			contactName := firstNonEmpty(data.PushName, conversation.Name, pushNames[remoteJID])
			if isGroup {
				contactName = conversation.Name
			}
			msg := baseMessage(instance, remoteJID, id, contactName, data.Key.FromMe,
				isGroup, timestampOrNow(int64(data.MessageTimestamp)))
			if msg.IsGroup && !data.Key.FromMe {
				msg.SenderJID = firstNonEmpty(data.Participant, data.Key.Participant)
				msg.SenderName = data.PushName
			}
			if err := decodeContent(msg, data.Message); err != nil {
				return nil, 0, err
			}
			messages = append(messages, msg)
		}
	}
	return messages, env.Data.Progress, nil
}

// ParseHistorySyncContacts extracts directory data independently from messages.
// Evolution sends push names and LID mappings in different history chunks, so
// tying them to the conversations of the same chunk drops valid names.
func ParseHistorySyncContacts(raw json.RawMessage) ([]domain.ContactIdentity, error) {
	var env struct {
		Data struct {
			PushNames []struct {
				ID   string `json:"ID"`
				Name string `json:"pushname"`
			} `json:"pushnames"`
			Mappings []struct {
				PhoneJID string `json:"pnJID"`
				LIDJID   string `json:"lidJID"`
			} `json:"phoneNumberToLidMappings"`
		} `json:"Data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decodificando directorio de history sync: %w", err)
	}

	byPhone := make(map[string]domain.ContactIdentity)
	phoneByLID := make(map[string]string)
	for _, mapping := range env.Data.Mappings {
		if mapping.PhoneJID == "" {
			continue
		}
		entry := byPhone[mapping.PhoneJID]
		entry.PhoneJID = mapping.PhoneJID
		entry.LIDJID = mapping.LIDJID
		byPhone[mapping.PhoneJID] = entry
		if mapping.LIDJID != "" {
			phoneByLID[mapping.LIDJID] = mapping.PhoneJID
		}
	}
	for _, item := range env.Data.PushNames {
		phoneJID := item.ID
		if mapped := phoneByLID[item.ID]; mapped != "" {
			phoneJID = mapped
		}
		if phoneJID == "" {
			continue
		}
		entry := byPhone[phoneJID]
		entry.PhoneJID = phoneJID
		entry.Name = item.Name
		byPhone[phoneJID] = entry
	}

	contacts := make([]domain.ContactIdentity, 0, len(byPhone))
	for _, entry := range byPhone {
		contacts = append(contacts, entry)
	}
	return contacts, nil
}

func baseMessage(instance, remoteJID, id, pushName string, fromMe, isGroup bool, at time.Time) *domain.Message {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	msg := &domain.Message{
		InstanceName: instance,
		RemoteJID:    remoteJID,
		WAMessageID:  id,
		PushName:     pushName,
		Timestamp:    at.UTC(),
		IsGroup:      isGroup || strings.HasSuffix(remoteJID, "@g.us"),
		Status:       domain.StatusSent,
	}
	if fromMe {
		msg.Direction = domain.Outbound
		msg.Origin = domain.OriginDevice
	} else {
		msg.Direction = domain.Inbound
		msg.Origin = domain.OriginContact
	}
	return msg
}

func decodeContent(msg *domain.Message, raw json.RawMessage) error {
	var value content
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &value); err != nil {
			msg.Type = domain.TypeUnsupported
			return nil
		}
	}
	applyContent(msg, &value)
	if msg.Media != nil {
		msg.Media.SourceURL = value.MediaURL
		if value.Base64 != "" {
			data, err := base64.StdEncoding.DecodeString(value.Base64)
			if err != nil {
				return fmt.Errorf("decodificando media base64: %w", err)
			}
			msg.Media.Data = data
		}
	}
	return nil
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
			msg.QuotedID = firstNonEmpty(c.ExtendedText.ContextInfo.StanzaID, c.ExtendedText.ContextInfo.StanzaId)
		}
	case c.Image != nil:
		msg.Type = domain.TypeImage
		msg.Body = c.Image.Caption
		msg.Media = &domain.MediaInfo{MimeType: c.Image.Mimetype, SizeBytes: int64(c.Image.FileLength), Width: c.Image.Width, Height: c.Image.Height}
	case c.Audio != nil:
		msg.Type = domain.TypeAudio
		msg.Media = &domain.MediaInfo{MimeType: c.Audio.Mimetype, SizeBytes: int64(c.Audio.FileLength), Duration: c.Audio.Seconds}
	case c.Video != nil:
		msg.Type = domain.TypeVideo
		msg.Body = c.Video.Caption
		msg.Media = &domain.MediaInfo{MimeType: c.Video.Mimetype, SizeBytes: int64(c.Video.FileLength), Duration: c.Video.Seconds}
	case c.Document != nil:
		msg.Type = domain.TypeDocument
		msg.Body = c.Document.Caption
		msg.Media = &domain.MediaInfo{MimeType: c.Document.Mimetype, FileName: c.Document.FileName, SizeBytes: int64(c.Document.FileLength)}
	case c.Sticker != nil:
		msg.Type = domain.TypeSticker
		msg.Media = &domain.MediaInfo{MimeType: c.Sticker.Mimetype, SizeBytes: int64(c.Sticker.FileLength)}
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
			msg.QuotedID = firstNonEmpty(c.Reaction.Key.ID, c.Reaction.Key.IDLower)
		}
	default:
		msg.Type = domain.TypeUnsupported
	}
}

func timestampOrNow(unix int64) time.Time {
	if unix <= 0 {
		return time.Now().UTC()
	}
	return time.Unix(unix, 0).UTC()
}

func PartitionKey(instance, remoteJID string) string { return instance + "|" + remoteJID }
func DedupeKey(instance, event, messageID string) string {
	return instance + ":" + event + ":" + messageID
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func preferredPhoneJID(values ...string) string {
	for _, value := range values {
		if strings.HasSuffix(value, "@s.whatsapp.net") {
			return value
		}
	}
	return firstNonEmpty(values...)
}
