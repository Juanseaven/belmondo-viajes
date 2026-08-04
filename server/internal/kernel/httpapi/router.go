// Package httpapi arma el router raíz y monta los módulos.
//
// Deliberadamente sin framework: net/http del stdlib con el enrutado por método
// que trae desde Go 1.22 cubre esta superficie y evita una dependencia que
// envejece.
package httpapi

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/auth"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/httpx"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/ws"
	"github.com/juanseaven/viajes-crm/server/internal/modules"
)

type Deps struct {
	Auth           *auth.Service
	Modules        []modules.Module
	Hub            *ws.Hub
	AllowedOrigins []string
	Log            *slog.Logger
}

// Router es el enrutado del sistema. Implementa auth.ModuleLister para que
// /api/me pueda enumerar los módulos sin que el kernel importe a ninguno.
type Router struct {
	mux     *http.ServeMux
	handler http.Handler
	names   []string
}

func NewRouter(d Deps) *Router {
	mux := http.NewServeMux()
	r := &Router{mux: mux}

	for _, m := range d.Modules {
		r.names = append(r.names, m.Name())
	}

	handlers := auth.NewHandlers(d.Auth, r, d.Log)
	handlers.MountPublic(mux)

	// Todo lo que exige sesión se registra en un mux aparte y se envuelve entero,
	// en vez de envolver handler por handler: así no se puede olvidar el
	// middleware al añadir una ruta.
	authed := http.NewServeMux()
	handlers.MountAuthed(authed)
	authed.Handle("GET /api/ws", wsHandler(d))
	mux.Handle("/api/", d.Auth.Require(authed))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"wsClients": d.Hub.Connected(),
			"modules":   r.names,
		})
	})

	for _, m := range d.Modules {
		m.Mount(&moduleRouter{root: mux, authed: authed, prefix: "/api/" + m.Name()})
		d.Log.Info("módulo montado", "module", m.Name())
	}

	r.handler = withCORS(d.AllowedOrigins, withLogging(d.Log, mux))
	return r
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.handler.ServeHTTP(w, req)
}

// ModuleNames alimenta la respuesta de /api/me, de la que el frontend saca el
// menú.
func (r *Router) ModuleNames() []string { return r.names }

// moduleRouter es la vista estrecha del enrutado que recibe cada módulo: solo
// puede colgar handlers, y los autenticados quedan encerrados bajo su prefijo.
type moduleRouter struct {
	root   *http.ServeMux // rutas públicas, sin middleware de sesión
	authed *http.ServeMux // ya envuelto por auth.Require
	prefix string         // /api/mirror
}

// Authed traduce "GET /conversations" a "GET /api/mirror/conversations".
//
// El patrón de net/http lleva el método por delante, así que hay que partirlo
// para insertar el prefijo en la ruta y no delante del método.
func (m *moduleRouter) Authed(pattern string, h http.Handler) {
	method, path := splitPattern(pattern)
	m.authed.Handle(strings.TrimSpace(method+" "+m.prefix+path), h)
}

func (m *moduleRouter) Public(pattern string, h http.Handler) {
	m.root.Handle(pattern, h)
}

func splitPattern(pattern string) (method, path string) {
	pattern = strings.TrimSpace(pattern)
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		return pattern[:i], strings.TrimSpace(pattern[i+1:])
	}
	return "", pattern
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
