// Package mirror es el espejo de WhatsApp: refleja en el panel todas las líneas
// de la agencia y permite escribir por cualquiera de ellas.
//
// Este archivo es la única superficie que el arranque conoce. Todo lo demás
// (ingesta, workers, media, persistencia) es interno del módulo y puede cambiar
// sin tocar el kernel ni ningún otro módulo.
package mirror

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/auth"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/httpx"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/ws"
	"github.com/juanseaven/viajes-crm/server/internal/modules"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/evolution"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/ingest"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/media"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store/postgres"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store/rediscache"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/webhook"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/worker"
)

// Name identifica al módulo en las rutas (/api/mirror/...), en schema_migrations
// y en el sobre de los eventos de websocket.
const Name = domain.ModuleName

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Module struct {
	k *kernel.Kernel

	pool      *worker.Pool
	events    pendingHistorySource
	mediaSvc  *media.Service
	webhook   http.Handler
	mediaDB   store.MediaStore
	read      store.ReadStore
	upkeep    store.Maintenance
	contacts  store.ContactStore
	directory lineDirectory
	avatars   *AvatarService
	log       *slog.Logger

	// Escritura.
	send        *SendService
	lines       store.InstanceStore
	provisioner Provisioner
	qr          QRCache

	// Cancela las tareas de fondo propias del módulo (hoy, la purga). Se guarda
	// para que Shutdown pueda pararlas sin depender del contexto de Start.
	stopUpkeep context.CancelFunc
	upkeepDone chan struct{}

	stopRecovery context.CancelFunc
	recoveryDone chan struct{}

	stopMediaRecovery context.CancelFunc
	mediaRecoveryDone chan struct{}
}

// New cablea el módulo. Construye sus propias dependencias (el cliente de
// Evolution es solo suyo) y toma del kernel lo que es de todos.
func New(k *kernel.Kernel) *Module {
	log := k.Log.With("module", Name)
	cfg := k.Cfg

	events := postgres.NewEvents(k.DB)
	messages := postgres.NewMessages(k.DB)
	contacts := postgres.NewContacts(k.DB)
	instances := postgres.NewInstances(k.DB)
	mediaDB := postgres.NewMedia(k.DB)
	panelSends := rediscache.NewPanelSends(k.Redis)

	outbound := postgres.NewOutbound(k.DB)
	qr := rediscache.NewQRCodes(k.Redis)

	evo := evolution.New(cfg.EvolutionBaseURL, cfg.EvolutionAPIKey, cfg.EvolutionWebhookURL)

	mediaSvc := media.NewService(evo, k.Blobs, mediaDB, cfg.WorkerQueueSize, log)
	processor := ingest.NewProcessor(
		events, messages, contacts, evo, instances, panelSends, outbound, qr, mediaSvc, k.Hub, log)
	pool := worker.New(cfg.WorkerShards, cfg.WorkerQueueSize, processor, log)

	m := &Module{
		k:         k,
		pool:      pool,
		events:    events,
		mediaSvc:  mediaSvc,
		webhook:   webhook.NewHandler(events, pool, instances, cfg.EvolutionAPIKey, log),
		mediaDB:   mediaDB,
		read:      postgres.NewRead(k.DB),
		upkeep:    postgres.NewMaintenance(k.DB),
		contacts:  contacts,
		directory: evo,
		avatars:   NewAvatarService(outbound, evo, nil),
		log:       log,

		send:        NewSendService(outbound, panelSends, evo, k.Blobs, cfg.R2SignedURLTTL, log),
		lines:       instances,
		provisioner: evo,
		qr:          qr,
	}
	mediaSvc.OnStored(m.announceStored)
	return m
}

// announceStored avisa que el archivo de un mensaje quedó guardado: el chat
// reemplaza «Descargando adjunto…» por el archivo, y Archivos lo muestra.
func (m *Module) announceStored(ctx context.Context, job store.PendingMedia) {
	var conversationID, instanceID string
	if err := m.k.DB.QueryRow(ctx,
		`SELECT conversation_id::text, instance_id::text FROM messages WHERE id = $1`, job.MessageID,
	).Scan(&conversationID, &instanceID); err != nil {
		m.log.Warn("no se pudo avisar el archivo guardado", "message_id", job.MessageID, "error", err)
		return
	}
	raw, err := json.Marshal(map[string]any{
		"messageId": job.MessageID, "mediaId": job.ID, "conversationId": conversationID,
	})
	if err != nil {
		return
	}
	m.k.Hub.Broadcast(ws.Event{Module: domain.ModuleName, Type: "media.stored", InstanceID: instanceID,
		Payload: raw, At: time.Now().UTC()})
}

// Verifica en tiempo de compilación que el módulo cumple el contrato.
var _ modules.Module = (*Module)(nil)

func (m *Module) Name() string { return Name }

func (m *Module) Migrations() fs.FS {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		// Imposible salvo que se rompa la directiva go:embed de arriba, y eso es
		// un error de compilación disfrazado.
		panic(fmt.Sprintf("mirror: migraciones embebidas ilegibles: %v", err))
	}
	return sub
}

func (m *Module) Mount(r modules.Router) {
	// Público: Evolution no puede presentar un JWT. La petición se autentica con
	// la apikey del payload, que valida el propio handler.
	r.Public("POST /webhooks/evolution", m.webhook)

	// Lectura y envío: todo el equipo.
	r.Authed("GET /lines", http.HandlerFunc(m.listLines))
	r.Authed("GET /conversations", http.HandlerFunc(m.listConversations))
	r.Authed("GET /conversations/{id}", http.HandlerFunc(m.getConversation))
	r.Authed("GET /conversations/{id}/avatar", http.HandlerFunc(m.serveAvatar))
	r.Authed("GET /conversations/{id}/messages", http.HandlerFunc(m.listMessages))
	r.Authed("POST /conversations/{id}/messages", http.HandlerFunc(m.sendMessage))
	r.Authed("POST /conversations/{id}/read", http.HandlerFunc(m.markRead))
	r.Authed("GET /search", http.HandlerFunc(m.searchMessages))
	r.Authed("GET /media/{id}", http.HandlerFunc(m.serveMedia))

	// Gestión de líneas: solo admin. Conectar un número expone todas sus
	// conversaciones al equipo entero, así que no es lo mismo que mandar un
	// mensaje.
	r.Authed("POST /lines", auth.RequireAdmin(http.HandlerFunc(m.createLineHandler)))
	r.Authed("DELETE /lines/{id}", auth.RequireAdmin(http.HandlerFunc(m.deleteLineHandler)))
	r.Authed("GET /lines/{id}/qr", auth.RequireAdmin(http.HandlerFunc(m.lineQR)))
}

func (m *Module) serveAvatar(w http.ResponseWriter, r *http.Request) {
	image, err := m.avatars.Load(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, ErrAvatarUnavailable) {
			w.Header().Set("Cache-Control", "private, max-age=900")
			httpx.NotFound(w, "avatar no disponible")
			return
		}
		httpx.Internal(w, m.log, "no se pudo cargar el avatar", err)
		return
	}
	w.Header().Set("Content-Type", image.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=21600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image.Data)
}

func (m *Module) Start(ctx context.Context) error {
	m.mediaSvc.Start(ctx, m.k.Cfg.MediaWorkers)
	m.pool.Start(ctx)
	if count, err := requeuePendingHistorySync(ctx, m.events, m.pool, 200); err != nil {
		m.log.Warn("no se pudo reencolar el historial pendiente", "requeued", count, "error", err)
	} else if count > 0 {
		m.log.Info("historial pendiente reencolado", "events", count)
	}
	m.refreshLineDirectories(ctx)

	// Recupera los adjuntos que quedaron a medias en el reinicio anterior.
	if err := m.mediaSvc.Requeue(ctx, 200); err != nil {
		m.log.Warn("no se pudo reencolar el media pendiente", "error", err)
	}
	mediaRecoveryCtx, stopMediaRecovery := context.WithCancel(context.WithoutCancel(ctx))
	m.stopMediaRecovery = stopMediaRecovery
	m.mediaRecoveryDone = make(chan struct{})
	go func() {
		defer close(m.mediaRecoveryDone)
		runMediaRecoveryLoop(mediaRecoveryCtx, mediaRecoveryInterval, func(loopCtx context.Context) error {
			if err := m.mediaSvc.Requeue(loopCtx, 200); err != nil && loopCtx.Err() == nil {
				m.log.Warn("no se pudo reencolar el media pendiente", "error", err)
			}
			return nil
		})
	}()

	upkeepCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	m.stopUpkeep = cancel
	m.upkeepDone = make(chan struct{})
	go m.runPurge(upkeepCtx)

	recoveryCtx, stopRecovery := context.WithCancel(context.WithoutCancel(ctx))
	m.stopRecovery = stopRecovery
	m.recoveryDone = make(chan struct{})
	go m.runLineRecovery(recoveryCtx)

	return nil
}

func (m *Module) refreshLineDirectories(ctx context.Context) {
	lines, err := m.read.Lines(ctx)
	if err != nil {
		m.log.Warn("no se pudieron listar líneas para sincronizar agendas", "error", err)
		return
	}
	for _, line := range lines {
		if !shouldRecoverLine(line) {
			continue
		}
		if err := m.reconcileLine(ctx, line, true); err != nil {
			m.log.Warn("no se pudo recuperar la línea", "instance", line.Name, "error", err)
		}
	}
}

// Shutdown drena en el orden que evita perder trabajo: primero el pool de eventos
// (que produce trabajo de media) y después el pipeline de media.
func (m *Module) Shutdown(ctx context.Context) error {
	if m.stopMediaRecovery != nil {
		m.stopMediaRecovery()
		select {
		case <-m.mediaRecoveryDone:
		case <-ctx.Done():
			m.log.Warn("el barrido de media no terminó a tiempo; se abandona")
		}
	}

	if m.stopRecovery != nil {
		m.stopRecovery()
		select {
		case <-m.recoveryDone:
		case <-ctx.Done():
			m.log.Warn("la recuperación de líneas no terminó a tiempo; se abandona")
		}
	}

	// La purga es trabajo de fondo prescindible: se corta primero para que no
	// compita por conexiones con lo que sí hay que drenar.
	if m.stopUpkeep != nil {
		m.stopUpkeep()
		select {
		case <-m.upkeepDone:
		case <-ctx.Done():
			m.log.Warn("la purga no terminó a tiempo; se abandona")
		}
	}

	if err := m.pool.Shutdown(ctx); err != nil {
		// Los eventos siguen en webhook_events con status='pending': el
		// reprocesador los recuperará al arrancar. No se pierde nada.
		m.log.Error("pool no drenado a tiempo", "error", err)
	}
	if err := m.mediaSvc.Shutdown(ctx); err != nil {
		m.log.Error("pipeline de media no drenado a tiempo", "error", err)
	}
	return nil
}

// serveMedia emite una URL firmada de vida corta en vez de servir el binario.
//
// Dos razones: el backend no gasta ancho de banda proxyando archivos de varios
// MB, y la URL caduca, así que reenviarla por error no expone el archivo de forma
// permanente. El bucket es privado.
func (m *Module) serveMedia(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	key, mimeType, err := m.mediaDB.StorageKey(r.Context(), id)
	if err != nil {
		// 404 tanto si no existe como si sigue subiéndose: no se filtra qué ids
		// son válidos.
		httpx.NotFound(w, "no disponible")
		return
	}

	ttl := m.k.Cfg.R2SignedURLTTL
	url, err := m.k.Blobs.SignedURL(r.Context(), key, ttl)
	if err != nil {
		httpx.Internal(w, m.log, "no se pudo firmar la URL", err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"url":       url,
		"mimeType":  mimeType,
		"expiresIn": int(ttl.Seconds()),
	})
}
