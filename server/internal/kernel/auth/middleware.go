package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/httpx"
)

type ctxKey struct{}

// FromContext devuelve el usuario autenticado. El segundo valor es false en
// rutas públicas, donde nunca corrió el middleware.
func FromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

// UserID es el atajo para lo que necesita casi todo handler.
func UserID(ctx context.Context) string {
	u, _ := FromContext(ctx)
	return u.ID
}

// Require exige un token válido. Es el middleware que envuelve todo lo que un
// módulo monta con Router.Authed.
//
// Acepta el token por cabecera Authorization y, como alternativa, por query
// param: los WebSocket del navegador no permiten mandar cabeceras en el
// handshake y esa es la única forma de autenticarlos sin cookies.
func (s *Service) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			httpx.Unauthorized(w, "falta el token de sesión")
			return
		}
		user, err := s.Authenticate(r.Context(), token)
		if err != nil {
			httpx.Unauthorized(w, "sesión inválida o expirada")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	})
}

// RequireAdmin se aplica DESPUÉS de Require: da por hecho que hay usuario en el
// contexto.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := FromContext(r.Context())
		if !ok {
			httpx.Unauthorized(w, "falta el token de sesión")
			return
		}
		if !user.IsAdmin() {
			httpx.Forbidden(w, "hace falta rol de administrador")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if after, found := strings.CutPrefix(h, "Bearer "); found {
			return strings.TrimSpace(after)
		}
		return ""
	}
	return r.URL.Query().Get("token")
}
