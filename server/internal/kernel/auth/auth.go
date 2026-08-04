// Package auth es la autenticación compartida del panel: un único login para
// todos los módulos.
//
// Deliberadamente NO decide a qué módulos entra un usuario. Hoy todos entran a
// todos; el rol solo distingue quién administra usuarios. El día que eso cambie,
// el único lugar donde hay que tocarlo es la respuesta de /api/me.
package auth

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrInvalidCredentials es deliberadamente uno solo para email inexistente y
	// contraseña incorrecta: distinguirlos convierte el login en un oráculo que
	// dice qué emails están dados de alta.
	ErrInvalidCredentials = errors.New("credenciales inválidas")

	ErrUserInactive = errors.New("usuario inactivo")
	ErrNotFound     = errors.New("usuario no encontrado")
	ErrEmailTaken   = errors.New("el email ya está registrado")
)

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

func (r Role) Valid() bool { return r == RoleAdmin || r == RoleMember }

// User es un usuario sin el hash de la contraseña: este tipo se serializa a JSON
// y no debe poder llevarse el hash por descuido.
type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	FullName    string     `json:"fullName"`
	Role        Role       `json:"role"`
	IsActive    bool       `json:"isActive"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

// Store es el puerto de persistencia de usuarios.
type Store interface {
	// ByEmailWithHash devuelve el usuario y su hash. Es el único método que
	// expone el hash, y existe solo para que Login pueda compararlo.
	ByEmailWithHash(ctx context.Context, email string) (User, string, error)
	ByID(ctx context.Context, id string) (User, error)
	List(ctx context.Context) ([]User, error)
	Create(ctx context.Context, email, hash, fullName string, role Role) (User, error)
	Update(ctx context.Context, id string, fullName *string, role *Role, isActive *bool, hash *string) (User, error)
	Delete(ctx context.Context, id string) error
	TouchLogin(ctx context.Context, id string, at time.Time) error
	CountAdmins(ctx context.Context) (int, error)
}
