// Command api es el backend de viajes_crm.
//
// El arranque tiene una forma deliberada: se construye la infraestructura
// compartida (el Kernel), se declara la lista de módulos, y a partir de ahí todo
// —migraciones, rutas, procesos de fondo, apagado— se deriva recorriéndola.
//
// Agregar un módulo al sistema es escribir su paquete y añadir una línea en
// `mods`.
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

	"github.com/juanseaven/viajes-crm/server/internal/kernel"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/auth"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/blobs"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/config"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/db"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/httpapi"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/migrations"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/ws"
	"github.com/juanseaven/viajes-crm/server/internal/modules"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror"
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

	// Holgado porque aquí dentro corren las migraciones, no solo las conexiones.
	startCtx, cancelStart := context.WithTimeout(ctx, 60*time.Second)
	defer cancelStart()

	// --- Kernel ---------------------------------------------------------------
	pool, err := db.Connect(startCtx, cfg.DatabaseURL, cfg.DatabaseMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()
	log.Info("postgres conectado", "max_conns", cfg.DatabaseMaxConns)

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, DB: cfg.RedisDB})
	defer func() { _ = rdb.Close() }()
	if err := rdb.Ping(startCtx).Err(); err != nil {
		return err
	}
	log.Info("redis conectado", "db", cfg.RedisDB)

	store, localBlobs, err := openBlobs(startCtx, cfg, log)
	if err != nil {
		return err
	}

	k := &kernel.Kernel{
		Cfg: cfg, DB: pool, Redis: rdb, Blobs: store,
		Hub: ws.NewHub(log), Log: log,
	}

	// --- Módulos --------------------------------------------------------------
	// Esta lista ES el sistema: migraciones, rutas y ciclo de vida se derivan de
	// recorrerla. Para sumar un módulo, se añade aquí.
	registry := modules.NewRegistry(log,
		mirror.New(k),
	)

	// --- Esquema --------------------------------------------------------------
	// El kernel va primero: los módulos tienen claves foráneas contra `users`.
	if err := db.Migrate(startCtx, pool, "kernel", migrations.FS(), log); err != nil {
		return err
	}
	for _, m := range registry.All() {
		if err := db.Migrate(startCtx, pool, m.Name(), m.Migrations(), log); err != nil {
			return err
		}
	}

	// --- Autenticación --------------------------------------------------------
	tokens, err := auth.NewTokens(cfg.JWTSecret, cfg.JWTTTL)
	if err != nil {
		return err
	}
	authSvc := auth.NewService(auth.NewPostgresStore(pool), tokens, log)
	if err := authSvc.EnsureSeedAdmin(startCtx, cfg.SeedAdminEmail, cfg.SeedAdminPassword); err != nil {
		return err
	}

	// --- Arranque de los módulos ----------------------------------------------
	if err := registry.Start(ctx); err != nil {
		return err
	}

	// --- HTTP -----------------------------------------------------------------
	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(httpapi.Deps{
			Auth:           authSvc,
			Modules:        registry.All(),
			Hub:            k.Hub,
			LocalBlobs:     localBlobs,
			AllowedOrigins: cfg.AllowedOrigins,
			Log:            log,
		}),
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

	return shutdown(srv, registry, log)
}

// shutdown drena en el orden que evita perder eventos:
//  1. el servidor HTTP deja de aceptar peticiones nuevas;
//  2. cada módulo termina lo que ya tiene encolado (en orden inverso al arranque).
//
// Solo después el defer de run cierra la base y Redis.
func shutdown(srv *http.Server, registry *modules.Registry, log *slog.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("apagado del servidor http con errores", "error", err)
	}
	registry.Shutdown(ctx)

	log.Info("apagado limpio")
	return nil
}

// openBlobs elige dónde se guardan los archivos.
//
// Los adjuntos de WhatsApp no viajan en el webhook: hay que pedírselos a
// Evolution y guardarlos en algún sitio para poder mostrarlos después. R2 es ese
// sitio en producción; sin sus credenciales se usa el disco, que alcanza para
// desarrollo y para un despliegue de una sola máquina.
//
// El segundo valor de retorno solo va poblado en el caso local: es lo que hace
// que el router monte la ruta que sirve esos archivos.
func openBlobs(ctx context.Context, cfg *config.Config, log *slog.Logger) (blobs.Store, *blobs.Local, error) {
	if cfg.UseR2() {
		r2, err := blobs.NewR2(ctx, cfg.R2AccountID, cfg.R2AccessKeyID, cfg.R2SecretKey, cfg.R2Bucket)
		if err != nil {
			return nil, nil, err
		}
		log.Info("almacenamiento en R2", "bucket", cfg.R2Bucket)
		return r2, nil, nil
	}

	local, err := blobs.NewLocal(cfg.BlobsDir, cfg.PublicBaseURL, cfg.JWTSecret)
	if err != nil {
		return nil, nil, err
	}
	log.Warn("almacenamiento en disco local: los archivos viven en este contenedor",
		"dir", cfg.BlobsDir, "base_url", cfg.PublicBaseURL)
	return local, local, nil
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
