package postgres

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
)

func TestMediaProcessingRecoversOnlyStaleRows(t *testing.T) {
	ctx, p := testDB(t)
	lineID := seedLine(t, ctx, p, "media-recovery-line", "Media recovery")
	contactID := seedContact(t, ctx, p, "5491100000001@s.whatsapp.net", "Media", "5491100000001")
	conversationID := seedConversation(t, ctx, p, lineID, contactID, time.Now(), 0)
	oldMessageID := seedMessage(t, ctx, p, conversationID, lineID, contactID, "wa-media-old", "", time.Now())
	recentMessageID := seedMessage(t, ctx, p, conversationID, lineID, contactID, "wa-media-recent", "", time.Now())

	var staleID, recentID string
	if err := p.QueryRow(ctx,
		`INSERT INTO media_assets (message_id, storage_status, attempts, updated_at, source_data)
		 VALUES ($1, 'processing', 1, now() - interval '11 minutes', 'old') RETURNING id`,
		oldMessageID).Scan(&staleID); err != nil {
		t.Fatalf("insert stale media: %v", err)
	}
	if err := p.QueryRow(ctx,
		`INSERT INTO media_assets (message_id, storage_status, attempts, updated_at, source_data)
		 VALUES ($1, 'processing', 1, now() - interval '9 minutes', 'recent') RETURNING id`,
		recentMessageID).Scan(&recentID); err != nil {
		t.Fatalf("insert recent media: %v", err)
	}

	pending, err := NewMedia(p).NextPending(ctx, 10)
	if err != nil {
		t.Fatalf("NextPending: %v", err)
	}
	if !containsPendingMedia(pending, staleID) {
		t.Fatalf("stale media %s was not recovered: %+v", staleID, pending)
	}
	if containsPendingMedia(pending, recentID) {
		t.Fatalf("recent processing media %s was recovered", recentID)
	}
	if len(pending) != 1 || !pending[0].Claimed || pending[0].Attempts != 2 {
		t.Fatalf("claimed media = %+v", pending)
	}
	again, err := NewMedia(p).NextPending(ctx, 10)
	if err != nil {
		t.Fatalf("second NextPending: %v", err)
	}
	if containsPendingMedia(again, staleID) {
		t.Fatalf("stale media was claimed twice: %+v", again)
	}
}

func containsPendingMedia(items []store.PendingMedia, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func TestMarkProcessedRedactsOnlyWebhookBinarySources(t *testing.T) {
	ctx, p := testDB(t)
	payload := []byte(`{
		"event":"messages.upsert",
		"data":{"Message":{"base64":"secret-binary","mediaUrl":"https://private/media","mimetype":"image/jpeg","caption":"pasaporte"}},
		"metadata":{"trace":"keep-me"}
	}`)
	events := NewEvents(p)
	id, err := events.Insert(ctx, "line-redaction", "messages.upsert", "redaction:1", payload)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := events.MarkProcessed(ctx, id); err != nil {
		t.Fatalf("MarkProcessed: %v", err)
	}

	var raw []byte
	var status string
	if err := p.QueryRow(ctx,
		`SELECT payload, status::text FROM webhook_events WHERE id = $1`, id,
	).Scan(&raw, &status); err != nil {
		t.Fatalf("read event: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	data := got["data"].(map[string]any)
	message := data["Message"].(map[string]any)
	if _, ok := message["base64"]; ok {
		t.Fatal("base64 remained in processed webhook")
	}
	if _, ok := message["mediaUrl"]; ok {
		t.Fatal("mediaUrl remained in processed webhook")
	}
	if message["mimetype"] != "image/jpeg" || message["caption"] != "pasaporte" {
		t.Fatalf("message metadata was changed: %#v", message)
	}
	if got["metadata"].(map[string]any)["trace"] != "keep-me" || status != "processed" {
		t.Fatalf("payload/status = %#v / %q", got, status)
	}
}
