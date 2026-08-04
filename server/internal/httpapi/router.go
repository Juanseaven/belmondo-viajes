// Package httpapi arma el router. Deliberadamente sin framework: net/http del
// stdlib con el enrutado por método que trae desde Go 1.22 cubre esta superficie
// y evita una dependencia que envejece.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juanseaven/whatsapp-mirror/server/internal/media"
	"github.com/juanseaven/whatsapp-mirror/server/internal/store"
	"github.com/juanseaven/whatsapp-mirror/server/internal/ws"
)

type Deps struct {
	Webhook        http.Handler
	Hub            *ws.Hub
	Media          store.MediaStore
	Signer         media.Uploader
	SignedURLTTL   time.Duration
	AllowedOrigins []string
	Log            *slog.Logger
}

func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()

	// Sin auth: lo protege la apikey del payload y la red interna de Docker.
	// Evolution no puede presentar un JWT.
	mux.Handle("POST /webhooks/evolution", d.Webhook)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"wsClients": d.Hub.Connected(),
		})
	})

	mux.Handle("GET /ws", requireAuth(wsHandler(d)))
	mux.Handle("GET /api/media/{id}", requireAuth(mediaHandler(d)))

	return withCORS(d.AllowedOrigins, withLogging(d.Log, mux))
}

// mediaHandler emite una URL firmada de vida corta en vez de servir el binario.
//
// Dos razones: el backend no gasta ancho de banda proxyando pasaportes de 4 MB, y
// la URL caduca, así que reenviarla por error no expone el archivo de forma
// permanente. El bucket de R2 es privado.
func mediaHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		key, mimeType, err := d.Media.StorageKey(r.Context(), id)
		if err != nil {
			// 404 tanto si no existe como si sigue subiéndose: no se filtra qué
			// ids son válidos.
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no disponible"})
			return
		}

		url, err := d.Signer.SignedURL(r.Context(), key, d.SignedURLTTL)
		if err != nil {
			d.Log.Error("no se pudo firmar la URL", "media_id", id, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "error interno"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"url":       url,
			"mimeType":  mimeType,
			"expiresIn": int(d.SignedURLTTL.Seconds()),
		})
	}
}

func wsHandler(d Deps) http.HandlerFunc {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		// El Origin se valida contra la lista blanca: sin esto, cualquier web
		// podría abrir un socket con la cookie del admin.
		CheckOrigin: func(r *http.Request) bool {
			return slices.Contains(d.AllowedOrigins, r.Header.Get("Origin"))
		},
	}
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			d.Log.Warn("fallo en el upgrade a websocket", "error", err)
			return
		}
		d.Hub.Serve(conn)
	}
}

// requireAuth valida el JWT del panel. La verificación real se implementa en
// internal/auth; aquí queda el punto de enganche del middleware.
func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO(auth): verificar el JWT y poner el user_id en el contexto.
		next.ServeHTTP(w, r)
	})
}

func withCORS(origins []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if slices.Contains(origins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withLogging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		// Solo se registran errores y peticiones lentas: con varias líneas activas
		// el webhook se llama constantemente y un log por evento es ruido caro.
		if d := time.Since(start); rec.status >= 400 || d > time.Second {
			log.Info("http",
				"method", r.Method, "path", r.URL.Path,
				"status", rec.status, "duration", d)
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
