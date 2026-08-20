package webhook

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/evolution"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/worker"
)

// HistorySync can contain many conversations. Media bytes are kept in MinIO,
// so 32 MiB bounds metadata bursts without rejecting normal initial syncs.
const maxBodyBytes = 64 << 20

type Submitter interface {
	Submit(job worker.Job) error
}

type InstanceLookup interface {
	IDByName(ctx context.Context, evolutionName string) (string, error)
}

type Handler struct {
	events    store.EventStore
	pool      Submitter
	instances InstanceLookup
	globalKey string
	log       *slog.Logger
	now       func() time.Time
}

func NewHandler(events store.EventStore, pool Submitter, instances InstanceLookup, globalKey string, log *slog.Logger) *Handler {
	return &Handler{events: events, pool: pool, instances: instances, globalKey: globalKey, log: log, now: time.Now}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "payload demasiado grande", http.StatusRequestEntityTooLarge)
		return
	}

	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		h.log.Warn("payload de webhook ilegible", "error", err)
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}
	if env.InstanceName == "" || env.InstanceID == "" || env.Event == "" {
		http.Error(w, "faltan instanceName, instanceId o event", http.StatusBadRequest)
		return
	}
	if !h.authorized(r.Context(), env) {
		h.log.Warn("webhook sin credencial válida", "instance", env.InstanceName, "remote", r.RemoteAddr)
		http.Error(w, "no autorizado", http.StatusUnauthorized)
		return
	}

	dedupe, partition := h.keysFor(env)
	eventID, err := h.events.Insert(r.Context(), env.InstanceName, env.Event, dedupe, body)
	switch {
	case errors.Is(err, store.ErrDuplicate):
		w.WriteHeader(http.StatusOK)
		return
	case err != nil:
		h.log.Error("no se pudo registrar el evento", "error", err, "event", env.Event)
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	err = h.pool.Submit(worker.Job{
		EventID:      eventID,
		Event:        env.Event,
		State:        env.State,
		Instance:     env.InstanceName,
		PartitionKey: partition,
		Payload:      env.Data,
		ReceivedAt:   time.Now(),
	})
	if errors.Is(err, worker.ErrQueueFull) {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "saturado", http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"received"}`))
}

func (h *Handler) authorized(ctx context.Context, env Envelope) bool {
	localID, err := h.instances.IDByName(ctx, env.InstanceName)
	if err != nil || localID != env.InstanceID {
		return false
	}
	expected := evolution.InstanceToken(h.globalKey, env.InstanceName)
	return subtle.ConstantTimeCompare([]byte(env.InstanceToken), []byte(expected)) == 1
}

func (h *Handler) keysFor(env Envelope) (dedupe, partition string) {
	var probe struct {
		Info struct {
			ID   string `json:"ID"`
			Chat string `json:"Chat"`
		} `json:"Info"`
	}
	if len(env.Data) > 0 {
		_ = json.Unmarshal(env.Data, &probe)
	}
	if probe.Info.ID != "" {
		return DedupeKey(env.InstanceName, env.Event, probe.Info.ID),
			PartitionKey(env.InstanceName, probe.Info.Chat)
	}

	// QR and sync retries carry identical JSON but no message ID. Hashing the
	// provider payload makes retries idempotent while allowing the next QR/chunk.
	sum := sha256.Sum256(env.Data)
	fingerprint := hex.EncodeToString(sum[:])
	if repeatableLifecycleEvent(env.Event) {
		// Evolution no incluye un identificador ni timestamp en los eventos de
		// conexiÃ³n. Dos conexiones reales pueden traer exactamente el mismo JSON,
		// por lo que deduplicarlas por payload deja la lÃ­nea en un estado viejo.
		// Estos handlers son idempotentes; cada entrega debe representar una nueva
		// transiciÃ³n aunque el contenido coincida con una anterior.
		fingerprint = fmt.Sprintf("%s:%d", fingerprint, h.now().UnixNano())
	}
	return DedupeKey(env.InstanceName, env.Event, fingerprint), PartitionKey(env.InstanceName, "")
}

func repeatableLifecycleEvent(event string) bool {
	switch event {
	case EventConnected, EventPairSuccess, EventOfflineSyncCompleted,
		EventDisconnected, EventLoggedOut, EventConnectFailure,
		EventQRTimeout, EventQRSuccess:
		return true
	default:
		return false
	}
}
