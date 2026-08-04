package auth

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/httpx"
)

// ModuleLister lo implementa el registro de módulos. auth no importa el paquete
// modules: solo necesita los nombres, y esa dependencia invertida mantiene al
// kernel sin saber qué módulos existen.
type ModuleLister interface {
	ModuleNames() []string
}

// Handlers monta los endpoints de sesión y de gestión de usuarios.
//
// El reparto: Mount registra lo público (login) y lo autenticado (/api/me,
// /api/users). Quién puede administrar usuarios lo decide RequireAdmin.
type Handlers struct {
	svc     *Service
	modules ModuleLister
	log     *slog.Logger
}

func NewHandlers(svc *Service, modules ModuleLister, log *slog.Logger) *Handlers {
	return &Handlers{svc: svc, modules: modules, log: log}
}

// MountPublic registra lo que se alcanza sin sesión.
func (h *Handlers) MountPublic(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", h.login)
	// Sin estado de sesión en el servidor: el logout existe para que el frontend
	// tenga un endpoint canónico al que llamar, y para poder añadir una lista de
	// revocación más adelante sin cambiar el contrato.
	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
}

// MountAuthed registra lo que exige sesión. El middleware Require ya lo aplicó
// quien llama.
func (h *Handlers) MountAuthed(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/me", h.me)

	admin := func(fn http.HandlerFunc) http.Handler { return RequireAdmin(fn) }
	mux.Handle("GET /api/users", admin(h.listUsers))
	mux.Handle("POST /api/users", admin(h.createUser))
	mux.Handle("PATCH /api/users/{id}", admin(h.updateUser))
	mux.Handle("DELETE /api/users/{id}", admin(h.deleteUser))
}

func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.BadRequest(w, "cuerpo inválido")
		return
	}

	token, user, err := h.svc.Login(r.Context(), in.Email, in.Password)
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		httpx.Unauthorized(w, "email o contraseña incorrectos")
		return
	case errors.Is(err, ErrUserInactive):
		httpx.Forbidden(w, "la cuenta está desactivada")
		return
	case err != nil:
		httpx.Internal(w, h.log, "fallo en el login", err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"token":     token,
		"user":      user,
		"expiresIn": int(h.svc.tokens.TTL().Seconds()),
	})
}

// me devuelve el usuario y los módulos que puede ver.
//
// HOY devuelve todos los módulos registrados, para todos los usuarios. El valor
// no está en lo que hace sino en dónde está: el día que un módulo deba
// restringirse, el cambio es aquí y en ningún otro sitio del sistema.
func (h *Handlers) me(w http.ResponseWriter, r *http.Request) {
	user, ok := FromContext(r.Context())
	if !ok {
		httpx.Unauthorized(w, "falta el token de sesión")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"user":    user,
		"modules": h.modules.ModuleNames(),
	})
}

func (h *Handlers) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.List(r.Context())
	if err != nil {
		httpx.Internal(w, h.log, "listando usuarios", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"users": users})
}

func (h *Handlers) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		FullName string `json:"fullName"`
		Role     Role   `json:"role"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.BadRequest(w, "cuerpo inválido")
		return
	}
	if in.Role == "" {
		in.Role = RoleMember
	}

	user, err := h.svc.Create(r.Context(), in.Email, in.Password, in.FullName, in.Role)
	switch {
	case errors.Is(err, ErrEmailTaken):
		httpx.Error(w, http.StatusConflict, httpx.CodeConflict, "ya existe un usuario con ese email")
		return
	case err != nil:
		// Los fallos de validación de Create son mensajes pensados para el
		// usuario final (email inválido, contraseña corta), no fallos internos.
		httpx.BadRequest(w, err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, user)
}

func (h *Handlers) updateUser(w http.ResponseWriter, r *http.Request) {
	// Punteros para distinguir "no enviado" de "enviado vacío": sin eso, un PATCH
	// que solo cambia el rol borraría el nombre.
	var in struct {
		FullName *string `json:"fullName"`
		Role     *Role   `json:"role"`
		IsActive *bool   `json:"isActive"`
		Password *string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.BadRequest(w, "cuerpo inválido")
		return
	}

	user, err := h.svc.Update(r.Context(), r.PathValue("id"), in.FullName, in.Role, in.IsActive, in.Password)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, "usuario no encontrado")
		return
	case errors.Is(err, ErrLastAdmin):
		httpx.Error(w, http.StatusConflict, httpx.CodeConflict, err.Error())
		return
	case err != nil:
		httpx.BadRequest(w, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, user)
}

func (h *Handlers) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if actor, ok := FromContext(r.Context()); ok && actor.ID == id {
		httpx.BadRequest(w, "no podés borrar tu propio usuario")
		return
	}

	err := h.svc.Delete(r.Context(), id)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, "usuario no encontrado")
		return
	case errors.Is(err, ErrLastAdmin):
		httpx.Error(w, http.StatusConflict, httpx.CodeConflict, err.Error())
		return
	case err != nil:
		httpx.Internal(w, h.log, "borrando usuario", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
