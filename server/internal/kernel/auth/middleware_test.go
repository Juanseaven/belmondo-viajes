package auth

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeStore evita necesitar Postgres para probar el middleware: lo que se prueba
// aquí es la decisión de dejar pasar o no, no el SQL.
type fakeStore struct {
	users map[string]User
}

func (f *fakeStore) ByID(_ context.Context, id string) (User, error) {
	u, ok := f.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (f *fakeStore) ByEmailWithHash(context.Context, string) (User, string, error) {
	return User{}, "", ErrNotFound
}
func (f *fakeStore) List(context.Context) ([]User, error) { return nil, nil }
func (f *fakeStore) Create(context.Context, string, string, string, Role) (User, error) {
	return User{}, nil
}
func (f *fakeStore) Update(context.Context, string, *string, *Role, *bool, *string) (User, error) {
	return User{}, nil
}
func (f *fakeStore) Delete(context.Context, string) error                { return nil }
func (f *fakeStore) TouchLogin(context.Context, string, time.Time) error { return nil }
func (f *fakeStore) CountAdmins(context.Context) (int, error)            { return 1, nil }

var _ Store = (*fakeStore)(nil)

func newTestService(t *testing.T, users ...User) (*Service, *Tokens) {
	t.Helper()
	tokens := newTestTokens(t, time.Hour)
	store := &fakeStore{users: map[string]User{}}
	for _, u := range users {
		store.users[u.ID] = u
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(store, tokens, log), tokens
}

// okHandler es lo que el middleware protege: si responde 200, dejó pasar.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequire(t *testing.T) {
	activo := User{ID: "u-1", Email: "a@b.c", Role: RoleMember, IsActive: true}
	inactivo := User{ID: "u-2", Email: "d@e.f", Role: RoleMember, IsActive: false}

	svc, tokens := newTestService(t, activo, inactivo)

	tokenActivo, err := tokens.Issue(activo)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	tokenInactivo, err := tokens.Issue(inactivo)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	tokenBorrado, err := tokens.Issue(User{ID: "u-fantasma"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	cases := []struct {
		name   string
		header string
		query  string
		want   int
	}{
		{name: "sin token", want: http.StatusUnauthorized},
		{name: "sin prefijo Bearer", header: tokenActivo, want: http.StatusUnauthorized},
		{name: "token basura", header: "Bearer no-es-un-jwt", want: http.StatusUnauthorized},
		// El token sigue siendo válido y bien firmado: lo que corta es releer al
		// usuario de la base en cada petición.
		{name: "usuario dado de baja", header: "Bearer " + tokenInactivo, want: http.StatusUnauthorized},
		{name: "usuario que ya no existe", header: "Bearer " + tokenBorrado, want: http.StatusUnauthorized},
		{name: "token válido", header: "Bearer " + tokenActivo, want: http.StatusOK},
		// Los WebSocket del navegador no pueden mandar cabeceras en el handshake.
		{name: "token por query param", query: tokenActivo, want: http.StatusOK},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			url := "/api/me"
			if c.query != "" {
				url += "?token=" + c.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			rec := httptest.NewRecorder()

			svc.Require(okHandler()).ServeHTTP(rec, req)

			if rec.Code != c.want {
				t.Fatalf("status = %d, se esperaba %d (cuerpo: %s)", rec.Code, c.want, rec.Body)
			}
		})
	}
}

// El 401 no debe revelar si la cuenta existe: un atacante no puede usarlo para
// enumerar usuarios.
func TestRequireNoFiltraSiElUsuarioExiste(t *testing.T) {
	inactivo := User{ID: "u-2", IsActive: false}
	svc, tokens := newTestService(t, inactivo)

	tokenInactivo, _ := tokens.Issue(inactivo)
	tokenFantasma, _ := tokens.Issue(User{ID: "u-fantasma"})

	body := func(token string) string {
		req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		svc.Require(okHandler()).ServeHTTP(rec, req)
		return rec.Body.String()
	}

	if body(tokenInactivo) != body(tokenFantasma) {
		t.Fatalf("las respuestas difieren y permiten distinguir una cuenta existente:\n  baja:     %s\n  inexistente: %s",
			body(tokenInactivo), body(tokenFantasma))
	}
}

func TestRequireAdmin(t *testing.T) {
	cases := []struct {
		name string
		user *User
		want int
	}{
		{"sin usuario en contexto", nil, http.StatusUnauthorized},
		{"member", &User{ID: "u-1", Role: RoleMember, IsActive: true}, http.StatusForbidden},
		{"admin", &User{ID: "u-2", Role: RoleAdmin, IsActive: true}, http.StatusOK},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
			if c.user != nil {
				req = req.WithContext(context.WithValue(req.Context(), ctxKey{}, *c.user))
			}
			rec := httptest.NewRecorder()

			RequireAdmin(okHandler()).ServeHTTP(rec, req)

			if rec.Code != c.want {
				t.Fatalf("status = %d, se esperaba %d", rec.Code, c.want)
			}
		})
	}
}

func TestRequirePoneElUsuarioEnElContexto(t *testing.T) {
	user := User{ID: "u-1", Email: "a@b.c", Role: RoleAdmin, IsActive: true}
	svc, tokens := newTestService(t, user)
	token, _ := tokens.Issue(user)

	var visto User
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		visto, _ = FromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	svc.Require(handler).ServeHTTP(httptest.NewRecorder(), req)

	if visto.ID != user.ID || visto.Role != RoleAdmin {
		t.Fatalf("usuario en contexto = %+v, se esperaba %+v", visto, user)
	}
}

// El hash de la contraseña no puede escaparse por la serialización de User: es la
// única defensa si alguien devuelve el struct entero desde un handler nuevo.
func TestUserNoSerializaElHash(t *testing.T) {
	raw, err := json.Marshal(User{ID: "u-1", Email: "a@b.c"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, prohibido := range []string{"password", "passwordHash", "password_hash", "hash"} {
		if _, ok := fields[prohibido]; ok {
			t.Fatalf("User serializa el campo %q: %s", prohibido, raw)
		}
	}
}
