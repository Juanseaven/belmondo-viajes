package webhook

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/juanseaven/whatsapp-mirror/server/internal/store"
	"github.com/juanseaven/whatsapp-mirror/server/internal/worker"
)

// maxBodyBytes acota el payload. Los webhooks de Evolution son JSON de metadatos
// (el binario del media NO viaja aquí), así que 2 MB sobra e impide que un body
// gigante agote la memoria del proceso.
const maxBodyBytes = 2 << 20

// Submitter es el pool de workers visto por el handler.
type Submitter interface {
	Submit(job worker.Job) error
}

// Handler recibe los webhooks de Evolution API.
//
// Contrato de diseño: hace lo mínimo y responde. Validar, persistir el crudo y
// encolar. Nada de resolver contactos, descargar media ni hacer fan-out — eso
// ocurre en el pool. Evolution corta a los 30 s y reintenta hasta 10 veces con
// backoff; un handler lento provoca una tormenta de reintentos.
type Handler struct {
	events store.EventStore
	pool   Submitter
	apiKey string
	log    *slog.Logger
}

func NewHandler(events store.EventStore, pool Submitter, apiKey string, log *slog.Logger) *Handler {
	return &Handler{events: events, pool: pool, apiKey: apiKey, log: log}
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
		// 400 y no 5xx: el payload es inválido, reintentarlo no arregla nada.
		h.log.Warn("payload de webhook ilegible", "error", err)
		http.Error(w, "json inválido", http.StatusBadRequest)
		return
	}

	// La apikey puede venir en el cuerpo (webhook global) o en la cabecera.
	// Comparación en tiempo constante: esta clave controla todas las líneas.
	presented := env.APIKey
	if presented == "" {
		presented = r.Header.Get("apikey")
	}
	if subtle.ConstantTimeCompare([]byte(presented), []byte(h.apiKey)) != 1 {
		h.log.Warn("webhook con apikey inválida",
			"instance", env.Instance, "remote", r.RemoteAddr)
		http.Error(w, "no autorizado", http.StatusUnauthorized)
		return
	}

	if env.Instance == "" || env.Event == "" {
		http.Error(w, "faltan instance o event", http.StatusBadRequest)
		return
	}

	dedupe, partition := h.keysFor(env)

	// El INSERT es quien decide si el evento es nuevo. Delegarlo en el índice
	// único de Postgres, en vez de en un SELECT previo, elimina la condición de
	// carrera entre dos reintentos concurrentes.
	eventID, err := h.events.Insert(r.Context(), env.Instance, env.Event, dedupe, body)
	switch {
	case errors.Is(err, store.ErrDuplicate):
		// Reintento de Evolution sobre algo ya visto. 200 para que deje de
		// reintentar.
		w.WriteHeader(http.StatusOK)
		return
	case err != nil:
		// 500 para que Evolution reintente: el evento aún no está a salvo.
		h.log.Error("no se pudo registrar el evento", "error", err, "event", env.Event)
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	err = h.pool.Submit(worker.Job{
		EventID:      eventID,
		Event:        env.Event,
		Instance:     env.Instance,
		PartitionKey: partition,
		Payload:      env.Data,
		ReceivedAt:   time.Now(),
	})
	if errors.Is(err, worker.ErrQueueFull) {
		// 503 + Retry-After. El evento ya está persistido, así que aunque
		// Evolution se rindiera, el reprocesador de webhook_events lo recogerá.
		w.Header().Set("Retry-After", "5")
		http.Error(w, "saturado", http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"received"}`))
}

// keysFor extrae la clave de deduplicación y la de partición.
//
// Se decodifica solo `key` del payload, no el mensaje entero: en la ruta caliente
// del handler cada asignación cuenta, y el parseo completo ya lo hace el worker.
func (h *Handler) keysFor(env Envelope) (dedupe, partition string) {
	var probe struct {
		Key MessageKey `json:"key"`
	}
	if len(env.Data) > 0 {
		_ = json.Unmarshal(env.Data, &probe)
	}

	if probe.Key.ID != "" {
		return DedupeKey(env.Instance, env.Event, probe.Key.ID),
			PartitionKey(env.Instance, probe.Key.RemoteJID)
	}

	// Eventos sin mensaje (connection.update, qrcode.updated): no hay id estable,
	// así que se deduplica por instante de llegada y se particiona por instancia
	// para que el orden de los cambios de estado de una línea se respete.
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	return DedupeKey(env.Instance, env.Event, ts), PartitionKey(env.Instance, "")
}
