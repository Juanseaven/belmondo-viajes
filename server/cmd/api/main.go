// Command api es el backend del panel espejo.
//
// Orden de arranque y apagado deliberado: los recursos se cierran en orden
// inverso al de apertura, y el pool de workers se drena ANTES de cerrar la base.
// Al revés, un despliegue perdería los eventos que quedasen en las colas.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/juanseaven/whatsapp-mirror/server/internal/config"
	"github.com/juanseaven/whatsapp-mirror/server/internal/evolution"
	"github.com/juanseaven/whatsapp-mirror/server/internal/httpapi"
	"github.com/juanseaven/whatsapp-mirror/server/internal/ingest"
	"github.com/juanseaven/whatsapp-mirror/server/internal/media"
	"github.com/juanseaven/whatsapp-mirror/server/internal/store/postgres"
	"github.com/juanseaven/whatsapp-mirror/server/internal/store/rediscache"
	"github.com/juanseaven/whatsapp-mirror/server/internal/webhook"
	"github.com/juanseaven/whatsapp-mirror/server/internal/worker"
	"github.com/juanseaven/whatsapp-mirror/server/internal/ws"
)

func main() {
	// Lo usa el healthcheck del compose: evita meter curl en una imagen scratch.
	healthcheck := flag.Bool("healthcheck", false, "consulta /healthz y sale")
	flag.Parse()
	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	if err := run(); err != nil {
		slog.Error("arranque fallido", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)

	// Contexto raíz cancelado por SIGINT/SIGTERM. Docker manda SIGTERM y espera
	// 10 s antes del SIGKILL, así que el apagado debe caber en ese margen.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- Infraestructura ------------------------------------------------------
	startCtx, cancelStart := context.WithTimeout(ctx, 30*time.Second)
	defer cancelStart()

	db, err := postgres.Connect(startCtx, cfg.DatabaseURL, cfg.DatabaseMaxConns)
	if err != nil {
		return err
	}
	defer db.Close()
	log.Info("postgres conectado", "max_conns", cfg.DatabaseMaxConns)

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, DB: cfg.RedisDB})
	defer func() { _ = rdb.Close() }()
	if err := rdb.Ping(startCtx).Err(); err != nil {
		return err
	}
	log.Info("redis conectado", "db", cfg.RedisDB)

	r2, err := media.NewR2(startCtx, cfg.R2AccountID, cfg.R2AccessKeyID, cfg.R2SecretKey, cfg.R2Bucket)
	if err != nil {
		return err
	}

	// --- Repositorios ---------------------------------------------------------
	events := postgres.NewEvents(db)
	messages := postgres.NewMessages(db)
	instances := postgres.NewInstances(db)
	mediaStore := postgres.NewMedia(db)
	panelSends := rediscache.NewPanelSends(rdb)

	// --- Servicios ------------------------------------------------------------
	evo := evolution.New(cfg.EvolutionBaseURL, cfg.EvolutionAPIKey)
	hub := ws.NewHub(log)

	mediaSvc := media.NewService(evo, r2, mediaStore, cfg.WorkerQueueSize, log)
	mediaSvc.Start(ctx, cfg.MediaWorkers)

	processor := ingest.NewProcessor(events, messages, instances, panelSends, mediaSvc, hub, log)
	pool := worker.New(cfg.WorkerShards, cfg.WorkerQueueSize, processor, log)
	pool.Start(ctx)

	// Recupera los adjuntos que quedaron a medias en el reinicio anterior.
	if err := mediaSvc.Requeue(startCtx, 200); err != nil {
		log.Warn("no se pudo reencolar el media pendiente", "error", err)
	}

	// --- HTTP -----------------------------------------------------------------
	router := httpapi.NewRouter(httpapi.Deps{
		Webhook:        webhook.NewHandler(events, pool, cfg.WebhookAPIKey, log),
		Hub:            hub,
		Media:          mediaStore,
		Signer:         r2,
		SignedURLTTL:   cfg.R2SignedURLTTL,
		AllowedOrigins: cfg.AllowedOrigins,
		Log:            log,
	})

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: router,
		// Sin WriteTimeout: cortaría los websockets, que son conexiones de larga
		// duración por definición. La protección está en los deadlines por
		// escritura de ws.Client.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("servidor http escuchando", "addr", cfg.HTTPAddr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		log.Info("señal de apagado recibida, drenando")
	}

	return shutdown(srv, pool, mediaSvc, log)
}

// shutdown drena en el orden que evita perder eventos:
//  1. el servidor HTTP deja de aceptar webhooks nuevos;
//  2. el pool termina lo que ya tiene encolado;
//  3. el pipeline de media acaba las subidas en curso.
//
// Solo después el defer de main cierra la base y Redis.
func shutdown(srv *http.Server, pool *worker.Pool, mediaSvc *media.Service, log *slog.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("apagado del servidor http con errores", "error", err)
	}
	if err := pool.Shutdown(ctx); err != nil {
		// Los eventos siguen en webhook_events con status='pending': el
		// reprocesador los recuperará al arrancar. No se pierde nada.
		log.Error("pool no drenado a tiempo", "error", err)
	}
	if err := mediaSvc.Shutdown(ctx); err != nil {
		log.Error("pipeline de media no drenado a tiempo", "error", err)
	}

	log.Info("apagado limpio")
	return nil
}

func runHealthcheck() int {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		port = "8080"
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	// JSON: estos logs los ingiere un agregador, no los lee una persona en una
	// terminal.
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}
