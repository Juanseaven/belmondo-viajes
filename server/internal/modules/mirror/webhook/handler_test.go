package webhook

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/evolution"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/worker"
)

func TestHandlerAcceptsMatchingEvolutionGoInstanceCredential(t *testing.T) {
	t.Parallel()

	const (
		name = "linea-abc"
		id   = "249aad2e-68f9-464f-bc84-aca560c38f0e"
		key  = "global-secret"
	)
	events := &recordingEvents{}
	pool := &recordingSubmitter{}
	handler := NewHandler(events, pool, fixedInstanceLookup{id: id}, key,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	body := `{"event":"Message","instanceId":"` + id + `","instanceName":"` + name +
		`","instanceToken":"` + evolution.InstanceToken(key, name) +
		`","data":{"Info":{"ID":"MSG-1","Chat":"5491112345678@s.whatsapp.net"},"Message":{"conversation":"hola"}}}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if events.instance != name || events.event != EventMessage {
		t.Fatalf("stored = %q/%q", events.instance, events.event)
	}
	if pool.job.Instance != name || pool.job.PartitionKey != name+"|5491112345678@s.whatsapp.net" {
		t.Fatalf("job = %+v", pool.job)
	}
}

func TestHandlerProcessesTheSameConnectionStateAgainOnALaterReconnect(t *testing.T) {
	t.Parallel()

	const (
		name = "linea-abc"
		id   = "249aad2e-68f9-464f-bc84-aca560c38f0e"
		key  = "global-secret"
	)
	events := &recordingEvents{seen: make(map[string]bool)}
	pool := &recordingSubmitter{}
	handler := NewHandler(events, pool, fixedInstanceLookup{id: id}, key,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Date(2026, 8, 18, 13, 0, 0, 0, time.UTC)
	handler.now = func() time.Time { return now }

	body := `{"event":"Connected","instanceId":"` + id + `","instanceName":"` + name +
		`","instanceToken":"` + evolution.InstanceToken(key, name) +
		`","data":{"status":"open","jid":"5491112345678:1@s.whatsapp.net"}}`
	for attempt := 0; attempt < 2; attempt++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/webhooks/evolution", bytes.NewBufferString(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, body = %s", attempt+1, rec.Code, rec.Body.String())
		}
		now = now.Add(time.Minute)
	}

	if pool.calls != 2 {
		t.Fatalf("submitted connection events = %d, want 2", pool.calls)
	}
}

func TestHandlerRejectsTokenOrInstanceIDThatDoesNotMatchLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		instanceID string
		token      string
	}{
		{name: "wrong token", instanceID: "expected-id", token: "forged"},
		{name: "wrong instance id", instanceID: "different-id", token: evolution.InstanceToken("global-secret", "linea-abc")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := &recordingEvents{}
			pool := &recordingSubmitter{}
			handler := NewHandler(events, pool, fixedInstanceLookup{id: "expected-id"}, "global-secret",
				slog.New(slog.NewTextHandler(io.Discard, nil)))
			body := `{"event":"QRCode","instanceId":"` + tt.instanceID + `","instanceName":"linea-abc","instanceToken":"` + tt.token + `","data":{"qrcode":"data:image/png;base64,eA=="}}`
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/webhooks/evolution", bytes.NewBufferString(body)))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if events.inserted || pool.submitted {
				t.Fatalf("unauthorized webhook was persisted or submitted")
			}
		})
	}
}

type fixedInstanceLookup struct{ id string }

func (f fixedInstanceLookup) IDByName(context.Context, string) (string, error) { return f.id, nil }

type recordingEvents struct {
	inserted bool
	instance string
	event    string
	seen     map[string]bool
}

func (r *recordingEvents) Insert(_ context.Context, instance, event, dedupe string, _ []byte) (int64, error) {
	if r.seen != nil && r.seen[dedupe] {
		return 0, store.ErrDuplicate
	}
	if r.seen != nil {
		r.seen[dedupe] = true
	}
	r.inserted = true
	r.instance = instance
	r.event = event
	return 42, nil
}
func (*recordingEvents) MarkProcessed(context.Context, int64) error      { return nil }
func (*recordingEvents) MarkFailed(context.Context, int64, string) error { return nil }

type recordingSubmitter struct {
	submitted bool
	calls     int
	job       worker.Job
}

func (r *recordingSubmitter) Submit(job worker.Job) error {
	r.submitted = true
	r.calls++
	r.job = job
	return nil
}
