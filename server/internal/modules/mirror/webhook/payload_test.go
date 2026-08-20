package webhook

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
)

func TestParseMessageReadsEvolutionGoLivePayload(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"Info": {
			"Chat": "5491112345678@s.whatsapp.net",
			"Sender": "5491112345678@s.whatsapp.net",
			"IsFromMe": false,
			"IsGroup": false,
			"ID": "3EB0LIVE",
			"PushName": "María",
			"Timestamp": "2026-08-05T17:00:01Z"
		},
		"Message": {"conversation": "Necesito un viaje"}
	}`)

	msg, err := ParseMessage("linea-abc", raw)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	if msg.InstanceName != "linea-abc" || msg.RemoteJID != "5491112345678@s.whatsapp.net" || msg.WAMessageID != "3EB0LIVE" {
		t.Fatalf("identity = %+v", msg)
	}
	if msg.Direction != domain.Inbound || msg.Origin != domain.OriginContact {
		t.Fatalf("direction/origin = %s/%s", msg.Direction, msg.Origin)
	}
	if msg.Type != domain.TypeText || msg.Body != "Necesito un viaje" || msg.PushName != "María" {
		t.Fatalf("content = %+v", msg)
	}
	wantTime := time.Date(2026, 8, 5, 17, 0, 1, 0, time.UTC)
	if !msg.Timestamp.Equal(wantTime) {
		t.Fatalf("timestamp = %s, want %s", msg.Timestamp, wantTime)
	}
}

func TestParseMessageKeepsTheParticipantWhoSentToAGroup(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"Info": {
			"Chat": "5491123812203-1611959832@g.us",
			"Sender": "5491126374402@s.whatsapp.net",
			"IsFromMe": false,
			"IsGroup": true,
			"ID": "3EB0GROUP",
			"PushName": "Sofia",
			"Timestamp": "2026-08-05T17:00:01Z"
		},
		"Message": {"conversation": "Hola grupo"}
	}`)

	msg, err := ParseMessage("linea-abc", raw)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	if msg.SenderJID != "5491126374402@s.whatsapp.net" || msg.SenderName != "Sofia" {
		t.Fatalf("group sender = %q/%q", msg.SenderJID, msg.SenderName)
	}
	if msg.PushName != "" {
		t.Fatalf("group contact name = %q, want empty; the sender must not rename the group", msg.PushName)
	}
}

func TestParseMessagePrefersThePhoneJIDForAGroupParticipant(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"Info": {
			"Chat": "120363321554617552@g.us",
			"Sender": "94132898906287@lid",
			"SenderAlt": "5491126437432@s.whatsapp.net",
			"IsFromMe": false,
			"IsGroup": true,
			"ID": "3EB0GROUP-LID",
			"PushName": "BETOAVEN",
			"Timestamp": "2026-08-11T12:46:26Z"
		},
		"Message": {"conversation": "Hay excepciones en bebidas"}
	}`)

	msg, err := ParseMessage("linea-abc", raw)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	if msg.SenderJID != "5491126437432@s.whatsapp.net" {
		t.Fatalf("group sender JID = %q, want the phone JID from SenderAlt", msg.SenderJID)
	}
}

func TestParseMessageKeepsInlineMediaForTheAsyncPipeline(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"Info":{"Chat":"5491112345678@s.whatsapp.net","Sender":"5491112345678@s.whatsapp.net",
		"IsFromMe":false,"IsGroup":false,"ID":"MEDIA-1","PushName":"María",
		"Timestamp":"2026-08-11T12:00:00Z","MediaType":"image"},
		"Message":{"imageMessage":{"mimetype":"image/jpeg","fileLength":4,"width":2,"height":2},
		"base64":"dGVzdA=="}
	}`)

	msg, err := ParseMessage("linea-abc", raw)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	if msg.Media == nil || string(msg.Media.Data) != "test" {
		t.Fatalf("inline media = %+v, want decoded test bytes", msg.Media)
	}
}

func TestParseHistorySyncExpandsConversationsIntoMessages(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"Data": {
			"syncType": 2,
			"progress": 100,
			"pushnames": [{"ID":"5491112345678@s.whatsapp.net","pushname":"María"}],
			"conversations": [{
				"ID": "5491112345678@s.whatsapp.net",
				"name": "María",
				"messages": [
					{"message": {
						"key": {"remoteJID":"5491112345678@s.whatsapp.net","fromMe":false,"ID":"HIST-1"},
						"message": {"conversation":"mensaje anterior"},
						"messageTimestamp": 1722877200,
						"pushName": "María"
					}},
					{"message": {
						"key": {"remoteJID":"5491112345678@s.whatsapp.net","fromMe":true,"ID":"HIST-2"},
						"message": {"extendedTextMessage":{"text":"respuesta anterior"}},
						"messageTimestamp": "1722877260"
					}}
				]
			}]
		}
	}`)

	messages, progress, err := ParseHistorySync("linea-abc", raw)
	if err != nil {
		t.Fatalf("ParseHistorySync: %v", err)
	}
	if progress != 100 {
		t.Fatalf("progress = %d", progress)
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(messages))
	}
	if messages[0].WAMessageID != "HIST-1" || messages[0].Body != "mensaje anterior" || messages[0].PushName != "María" {
		t.Fatalf("first = %+v", messages[0])
	}
	if messages[1].WAMessageID != "HIST-2" || messages[1].Direction != domain.Outbound || messages[1].Origin != domain.OriginDevice {
		t.Fatalf("second = %+v", messages[1])
	}
}

func TestParseHistorySyncKeepsLIDParticipantAndDirectoryMapping(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"Data": {
			"progress": 100,
			"pushnames": [
				{"ID":"5491126374402@s.whatsapp.net","pushname":"Sofia"}
			],
			"phoneNumberToLidMappings": [
				{"pnJID":"5491126374402@s.whatsapp.net","lidJID":"139663763349692@lid"}
			],
			"conversations": [{
				"ID": "5491123812203-1611959832@g.us",
				"name": "Merequetengue",
				"messages": [{"message": {
					"key": {"remoteJID":"5491123812203-1611959832@g.us","fromMe":false,"ID":"GROUP-1"},
					"participant": "139663763349692@lid",
					"pushName": "Sofia",
					"message": {"conversation":"mensaje grupal"},
					"messageTimestamp": 1722877200
				}}]
			}]
		}
	}`)

	messages, _, err := ParseHistorySync("linea-abc", raw)
	if err != nil {
		t.Fatalf("ParseHistorySync: %v", err)
	}
	if len(messages) != 1 || messages[0].SenderJID != "139663763349692@lid" {
		t.Fatalf("history group sender = %+v", messages)
	}
	if messages[0].PushName != "Merequetengue" {
		t.Fatalf("history group title = %q, want Merequetengue", messages[0].PushName)
	}

	contacts, err := ParseHistorySyncContacts(raw)
	if err != nil {
		t.Fatalf("ParseHistorySyncContacts: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("contacts = %d, want 1", len(contacts))
	}
	got := contacts[0]
	if got.PhoneJID != "5491126374402@s.whatsapp.net" || got.LIDJID != "139663763349692@lid" || got.Name != "Sofia" {
		t.Fatalf("contact identity = %+v", got)
	}
}
