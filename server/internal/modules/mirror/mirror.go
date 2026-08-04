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
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/juanseaven/viajes-crm/server/internal/kernel"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/httpx"
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

	pool     *worker.Pool
	mediaSvc *media.Service
	webhook  http.Handler
	mediaDB  store.MediaStore
	log      *slog.Logger
}

// New cablea el módulo. Construye sus propias dependencias (el cliente de
// Evolution es solo suyo) y toma del kernel lo que es de todos.
func New(k *kernel.Kernel) *Module {
	log := k.Log.With("module", Name)
	cfg := k.Cfg

	events := postgres.NewEvents(k.DB)
	messages := postgres.NewMessages(k.DB)
	instances := postgres.NewInstances(k.DB)
	mediaDB := postgres.NewMedia(k.DB)
	panelSends := rediscache.NewPanelSends(k.Redis)

	evo := evolution.New(cfg.EvolutionBaseURL, cfg.EvolutionAPIKey)

	mediaSvc := media.NewService(evo, k.Blobs, mediaDB, cfg.WorkerQueueSize, log)
	processor := ingest.NewProcessor(events, messages, instances, panelSends, mediaSvc, k.Hub, log)
	pool := worker.New(cfg.WorkerShards, cfg.WorkerQueueSize, processor, log)

	return &Module{
		k:        k,
		pool:     pool,
		mediaSvc: mediaSvc,
		webhook:  webhook.NewHandler(events, pool, cfg.WebhookAPIKey, log),
		mediaDB:  mediaDB,
		log:      log,
	}
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

	r.Authed("GET /media/{id}", http.HandlerFunc(m.serveMedia))
}

func (m *Module) Start(ctx context.Context) error {
	m.mediaSvc.Start(ctx, m.k.Cfg.MediaWorkers)
	m.pool.Start(ctx)

	// Recupera los adjuntos que quedaron a medias en el reinicio anterior.
	if err := m.mediaSvc.Requeue(ctx, 200); err != nil {
		m.log.Warn("no se pudo reencolar el media pendiente", "error", err)
	}
	return nil
}

// Shutdown drena en el orden que evita perder trabajo: primero el pool de eventos
// (que produce trabajo de media) y después el pipeline de media.
func (m *Module) Shutdown(ctx context.Context) error {
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
