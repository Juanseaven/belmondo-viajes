package ingest

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/ws"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/webhook"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/worker"
)

func TestProcessorPersistsHistorySyncAndBroadcastsOneRefresh(t *testing.T) {
	t.Parallel()

	events := &processorEvents{}
	messages := &processorMessages{}
	hub := &processorHub{}
	contacts := &processorContacts{}
	processor := NewProcessor(events, messages, contacts, nil, nil, nil, nil, nil, nil, hub,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	payload := json.RawMessage(`{"Data":{"progress":75,"conversations":[{"ID":"5491112345678@s.whatsapp.net","messages":[{"message":{"key":{"remoteJID":"5491112345678@s.whatsapp.net","fromMe":false,"ID":"HIST-1"},"message":{"conversation":"uno"},"messageTimestamp":1722877200}},{"message":{"key":{"remoteJID":"5491112345678@s.whatsapp.net","fromMe":true,"ID":"HIST-2"},"message":{"conversation":"dos"},"messageTimestamp":1722877260}}]}]}}`)

	err := processor.Handle(context.Background(), worker.Job{
		EventID: 9, Event: webhook.EventHistorySync, Instance: "linea-abc", Payload: payload,
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(messages.saved) != 2 {
		t.Fatalf("saved = %d, want 2", len(messages.saved))
	}
	if len(contacts.saved) != 0 {
		t.Fatalf("directory entries = %d, want 0", len(contacts.saved))
	}
	for _, msg := range messages.saved {
		if !msg.Historical {
			t.Fatalf("history message was not marked historical: %+v", msg)
		}
	}
	if events.processed != 9 {
		t.Fatalf("processed event = %d", events.processed)
	}
	if len(hub.events) != 1 || hub.events[0].Type != "history.sync" {
		t.Fatalf("broadcasts = %+v", hub.events)
	}
	var body struct {
		Instance string `json:"instance"`
		Progress int    `json:"progress"`
		Imported int    `json:"imported"`
	}
	if err := json.Unmarshal(hub.events[0].Payload, &body); err != nil {
		t.Fatalf("decode broadcast: %v", err)
	}
	if body.Instance != "linea-abc" || body.Progress != 75 || body.Imported != 2 {
		t.Fatalf("broadcast payload = %+v", body)
	}
}

func TestProcessorRefreshesTheLineAddressBookWhenHistorySyncCompletes(t *testing.T) {
	t.Parallel()

	contacts := &processorContacts{}
	directory := &processorDirectory{contacts: []domain.ContactIdentity{{
		PhoneJID: "5491126437432@s.whatsapp.net", FullName: "Beto", PushName: "BETOAVEN",
	}}}
	processor := NewProcessor(&processorEvents{}, &processorMessages{}, contacts, directory,
		nil, nil, nil, nil, nil, &processorHub{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := processor.Handle(context.Background(), worker.Job{
		EventID: 11, Event: webhook.EventHistorySync, Instance: "juanse-abc",
		Payload: json.RawMessage(`{"Data":{"progress":100,"conversations":[]}}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if contacts.instance != "juanse-abc" || len(contacts.saved) != 1 || contacts.saved[0].FullName != "Beto" {
		t.Fatalf("saved line directory = instance %q contacts %+v", contacts.instance, contacts.saved)
	}
}

func TestProcessorAppliesAGroupNameUpdate(t *testing.T) {
	t.Parallel()

	contacts := &processorContacts{}
	processor := NewProcessor(&processorEvents{}, &processorMessages{}, contacts, nil,
		nil, nil, nil, nil, nil, &processorHub{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := processor.Handle(context.Background(), worker.Job{
		EventID: 12, Event: webhook.EventGroupInfo, Instance: "juanse-abc",
		Payload: json.RawMessage(`{
			"JID":"120363321554617552@g.us",
			"Name":{"Name":"Administrativas aven","NameSetAt":"2026-08-11T12:00:00Z"}
		}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if contacts.groupJID != "120363321554617552@g.us" || contacts.groupName != "Administrativas aven" {
		t.Fatalf("group update = %q/%q", contacts.groupJID, contacts.groupName)
	}
}

func TestProcessorBroadcastsTheGroupMessageAuthor(t *testing.T) {
	t.Parallel()

	hub := &processorHub{}
	processor := NewProcessor(&processorEvents{}, &processorMessages{}, nil, nil,
		nil, nil, nil, nil, nil, hub, slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := processor.Handle(context.Background(), worker.Job{
		EventID: 13, Event: webhook.EventMessage, Instance: "juanse-abc",
		Payload: json.RawMessage(`{
			"Info":{"Chat":"120363321554617552@g.us","Sender":"94132898906287@lid",
			"SenderAlt":"5491126437432@s.whatsapp.net","IsFromMe":false,"IsGroup":true,
			"ID":"GROUP-LIVE","PushName":"BETOAVEN","Timestamp":"2026-08-11T12:46:26Z"},
			"Message":{"conversation":"Hola"}
		}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(hub.events) != 1 {
		t.Fatalf("events = %d, want 1", len(hub.events))
	}
	var payload struct {
		SenderJID  string `json:"senderJid"`
		SenderName string `json:"senderName"`
	}
	if err := json.Unmarshal(hub.events[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SenderJID != "5491126437432@s.whatsapp.net" || payload.SenderName != "BETOAVEN" {
		t.Fatalf("broadcast author = %+v", payload)
	}
}

func TestProcessorBroadcastsTheQuotedMessageID(t *testing.T) {
	t.Parallel()

	hub := &processorHub{}
	processor := NewProcessor(&processorEvents{}, &processorMessages{}, nil, nil,
		nil, nil, nil, nil, nil, hub, slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := processor.Handle(context.Background(), worker.Job{
		EventID: 15, Event: webhook.EventMessage, Instance: "juanse-abc",
		Payload: json.RawMessage(`{
			"Info":{"Chat":"5491111111111@s.whatsapp.net","Sender":"5491111111111@s.whatsapp.net",
			"IsFromMe":false,"IsGroup":false,"ID":"LIVE-REPLY","PushName":"Sofia",
			"Timestamp":"2026-08-11T12:46:26Z"},
			"Message":{"extendedTextMessage":{"text":"Respuesta","contextInfo":{"stanzaID":"WA-ORIGINAL"}}}
		}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	var payload struct {
		QuotedID string `json:"quotedId"`
	}
	if err := json.Unmarshal(hub.events[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.QuotedID != "WA-ORIGINAL" {
		t.Fatalf("quotedId = %q", payload.QuotedID)
	}
}

func TestProcessorUsesTheReceiptRootStateWhenDataTypeIsEmpty(t *testing.T) {
	t.Parallel()

	messages := &processorMessages{}
	processor := NewProcessor(&processorEvents{}, messages, nil, nil,
		nil, nil, nil, nil, nil, &processorHub{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := processor.Handle(context.Background(), worker.Job{
		EventID: 14, Event: webhook.EventReceipt, Instance: "juanse-abc", State: "Delivered",
		Payload: json.RawMessage(`{"MessageIDs":["WA-DELIVERED"],"Type":""}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if messages.statusID != "WA-DELIVERED" || messages.status != domain.StatusDelivered {
		t.Fatalf("receipt update = %q/%q", messages.statusID, messages.status)
	}
}

func TestProcessorClearsUnreadWhenThisWhatsAppAccountReadTheChat(t *testing.T) {
	t.Parallel()

	messages := &processorMessages{readConversationIDs: []string{"conv-1"}}
	hub := &processorHub{}
	processor := NewProcessor(&processorEvents{}, messages, nil, nil,
		nil, nil, nil, nil, nil, hub, slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := processor.Handle(context.Background(), worker.Job{
		EventID: 16, Event: webhook.EventReceipt, Instance: "juanse-abc",
		Payload: json.RawMessage(`{
			"Chat":"5491111111111@s.whatsapp.net","IsFromMe":true,
			"MessageIDs":["WA-INBOUND"],"Type":"read"
		}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if messages.readInstance != "juanse-abc" || messages.readChat != "5491111111111@s.whatsapp.net" {
		t.Fatalf("device read = %q/%q", messages.readInstance, messages.readChat)
	}
	if len(hub.events) != 1 || hub.events[0].Type != "conversation.read" {
		t.Fatalf("broadcasts = %+v", hub.events)
	}
	var payload struct {
		ConversationID string `json:"conversationId"`
	}
	if err := json.Unmarshal(hub.events[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ConversationID != "conv-1" {
		t.Fatalf("conversationId = %q, want conv-1", payload.ConversationID)
	}
}

func TestProcessorDoesNotClearUnreadForAReceiptFromTheCustomer(t *testing.T) {
	t.Parallel()

	messages := &processorMessages{readConversationIDs: []string{"conv-1"}}
	processor := NewProcessor(&processorEvents{}, messages, nil, nil,
		nil, nil, nil, nil, nil, &processorHub{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := processor.Handle(context.Background(), worker.Job{
		EventID: 17, Event: webhook.EventReceipt, Instance: "juanse-abc",
		Payload: json.RawMessage(`{
			"Chat":"5491111111111@s.whatsapp.net","IsFromMe":false,
			"MessageIDs":["WA-OUTBOUND"],"Type":"read"
		}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if messages.readChat != "" {
		t.Fatalf("customer receipt cleared unread for %q", messages.readChat)
	}
}

func TestProcessorPersistsAnOfflineReplayWithoutBroadcastingEveryOldMessage(t *testing.T) {
	t.Parallel()

	messages := &processorMessages{}
	hub := &processorHub{}
	processor := NewProcessor(&processorEvents{}, messages, nil, nil,
		nil, nil, nil, nil, nil, hub, slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := processor.Handle(context.Background(), worker.Job{
		EventID: 18, Event: webhook.EventMessage, Instance: "juanse-abc",
		ReceivedAt: time.Date(2026, 8, 27, 18, 0, 0, 0, time.UTC),
		Payload: json.RawMessage(`{
			"Info":{"Chat":"5491111111111@s.whatsapp.net","Sender":"5491111111111@s.whatsapp.net",
			"IsFromMe":false,"IsGroup":false,"ID":"OFFLINE-1","PushName":"Sofia",
			"Timestamp":"2026-08-21T12:00:00Z"},
			"Message":{"conversation":"Mensaje atrasado"}
		}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(messages.saved) != 1 || messages.saved[0].Historical {
		t.Fatalf("offline replay must be persisted as a normal unread candidate: %+v", messages.saved)
	}
	if len(hub.events) != 1 || hub.events[0].Type != "history.sync" {
		t.Fatalf("old replay broadcasts = %+v, want one batch-refresh signal", hub.events)
	}
}

func TestProcessorStoresEvolutionGoQRCode(t *testing.T) {
	t.Parallel()

	events := &processorEvents{}
	qr := &processorQR{}
	hub := &processorHub{}
	processor := NewProcessor(events, &processorMessages{}, nil, nil, nil, nil, nil, qr, nil, hub,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := processor.Handle(context.Background(), worker.Job{
		EventID: 10, Event: webhook.EventQRCode, Instance: "linea-abc",
		Payload: json.RawMessage(`{"qrcode":"data:image/png;base64,QR-CONTENT","code":"raw-code"}`),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if qr.instance != "linea-abc" || qr.code != "data:image/png;base64,QR-CONTENT" {
		t.Fatalf("stored QR = instance %q code %q", qr.instance, qr.code)
	}
	if len(hub.events) != 1 || hub.events[0].Type != "line.qr" {
		t.Fatalf("broadcasts = %+v", hub.events)
	}
}

type processorEvents struct{ processed int64 }

func (*processorEvents) Insert(context.Context, string, string, string, []byte) (int64, error) {
	return 0, nil
}
func (p *processorEvents) MarkProcessed(_ context.Context, id int64) error {
	p.processed = id
	return nil
}
func (*processorEvents) MarkFailed(context.Context, int64, string) error { return nil }

type processorMessages struct {
	saved               []*domain.Message
	statusID            string
	status              domain.MessageStatus
	readInstance        string
	readChat            string
	readConversationIDs []string
}

func (p *processorMessages) Save(_ context.Context, msg *domain.Message, _ string) (*store.PersistedMessage, error) {
	p.saved = append(p.saved, msg)
	return &store.PersistedMessage{MessageID: msg.WAMessageID, ConversationID: "conv", InstanceID: "instance", IsNew: true}, nil
}
func (p *processorMessages) UpdateStatus(_ context.Context, _, id string, status domain.MessageStatus) error {
	p.statusID, p.status = id, status
	return nil
}
func (p *processorMessages) MarkReadFromDevice(_ context.Context, instance, chat string) ([]string, error) {
	p.readInstance, p.readChat = instance, chat
	return p.readConversationIDs, nil
}
func (*processorMessages) MarkDeleted(context.Context, string, string) error { return nil }

type processorContacts struct {
	instance  string
	saved     []domain.ContactIdentity
	groupJID  string
	groupName string
}

func (p *processorContacts) UpsertDirectory(_ context.Context, instance string, contacts []domain.ContactIdentity) error {
	p.instance = instance
	p.saved = append(p.saved, contacts...)
	return nil
}

func (p *processorContacts) SetGroupName(_ context.Context, _, jid, name string) error {
	p.groupJID, p.groupName = jid, name
	return nil
}

type processorDirectory struct{ contacts []domain.ContactIdentity }

func (p *processorDirectory) Contacts(context.Context, string) ([]domain.ContactIdentity, error) {
	return p.contacts, nil
}

type processorHub struct{ events []ws.Event }

func (p *processorHub) Broadcast(event ws.Event) { p.events = append(p.events, event) }

type processorQR struct {
	instance string
	code     string
}

func (p *processorQR) Set(_ context.Context, instance, code string) error {
	p.instance = instance
	p.code = code
	return nil
}
func (*processorQR) Clear(context.Context, string) {}
