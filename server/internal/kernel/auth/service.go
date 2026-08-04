package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Service es la lógica de autenticación y gestión de usuarios.
type Service struct {
	store  Store
	tokens *Tokens
	log    *slog.Logger
}

func NewService(store Store, tokens *Tokens, log *slog.Logger) *Service {
	return &Service{store: store, tokens: tokens, log: log}
}

// Login valida credenciales y emite un token.
//
// El coste de bcrypt se paga incluso cuando el email no existe (ver dummyHash):
// sin eso, la diferencia de tiempo entre "email desconocido" y "contraseña mala"
// permite enumerar qué cuentas existen.
func (s *Service) Login(ctx context.Context, email, password string) (string, User, error) {
	email = strings.TrimSpace(strings.ToLower(email))

	user, hash, err := s.store.ByEmailWithHash(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
			return "", User{}, ErrInvalidCredentials
		}
		return "", User{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return "", User{}, ErrInvalidCredentials
	}
	// Se comprueba DESPUÉS de la contraseña: si no, un usuario dado de baja podría
	// confirmar su email probando cualquier contraseña.
	if !user.IsActive {
		return "", User{}, ErrUserInactive
	}

	token, err := s.tokens.Issue(user)
	if err != nil {
		return "", User{}, err
	}

	// El último acceso es informativo: que falle no puede impedir entrar.
	if err := s.store.TouchLogin(ctx, user.ID, time.Now()); err != nil {
		s.log.Warn("no se pudo registrar el último acceso", "user_id", user.ID, "error", err)
	}
	return token, user, nil
}

// Authenticate resuelve el usuario de un token. Lo usa el middleware.
//
// Relee el usuario de la base en vez de confiar en los claims: un token de 12
// horas seguiría siendo válido para alguien dado de baja hace diez.
func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	claims, err := s.tokens.Verify(token)
	if err != nil {
		return User{}, err
	}
	user, err := s.store.ByID(ctx, claims.UserID)
	if err != nil {
		return User{}, ErrInvalidToken
	}
	if !user.IsActive {
		return User{}, ErrUserInactive
	}
	return user, nil
}

func (s *Service) List(ctx context.Context) ([]User, error) { return s.store.List(ctx) }

func (s *Service) ByID(ctx context.Context, id string) (User, error) { return s.store.ByID(ctx, id) }

func (s *Service) Create(ctx context.Context, email, password, fullName string, role Role) (User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || !strings.Contains(email, "@") {
		return User{}, fmt.Errorf("email inválido")
	}
	if strings.TrimSpace(fullName) == "" {
		return User{}, fmt.Errorf("el nombre es obligatorio")
	}
	if !role.Valid() {
		return User{}, fmt.Errorf("rol inválido: %q", role)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	return s.store.Create(ctx, email, hash, strings.TrimSpace(fullName), role)
}

// Update aplica cambios parciales. Los punteros nil significan "no tocar".
func (s *Service) Update(ctx context.Context, id string, fullName *string, role *Role, isActive *bool, password *string) (User, error) {
	if role != nil && !role.Valid() {
		return User{}, fmt.Errorf("rol inválido: %q", *role)
	}

	var hash *string
	if password != nil {
		h, err := HashPassword(*password)
		if err != nil {
			return User{}, err
		}
		hash = &h
	}

	// Quedarse sin ningún admin deja el sistema sin nadie que pueda crear
	// usuarios: hay que recuperarlo a mano contra la base.
	demoting := role != nil && *role != RoleAdmin
	deactivating := isActive != nil && !*isActive
	if demoting || deactivating {
		if err := s.guardLastAdmin(ctx, id); err != nil {
			return User{}, err
		}
	}
	return s.store.Update(ctx, id, fullName, role, isActive, hash)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.guardLastAdmin(ctx, id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// ErrLastAdmin protege contra dejar el panel sin administradores.
var ErrLastAdmin = errors.New("no se puede dejar el sistema sin administradores activos")

func (s *Service) guardLastAdmin(ctx context.Context, id string) error {
	user, err := s.store.ByID(ctx, id)
	if err != nil {
		return err
	}
	if !user.IsAdmin() || !user.IsActive {
		return nil
	}
	n, err := s.store.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastAdmin
	}
	return nil
}

// EnsureSeedAdmin crea el primer administrador si la tabla está vacía.
//
// Sin esto no hay forma de entrar la primera vez: no existe registro público y
// crear usuarios exige ser admin. Solo actúa sobre una base sin usuarios, así que
// no puede pisar nada en un sistema en marcha.
func (s *Service) EnsureSeedAdmin(ctx context.Context, email, password string) error {
	if email == "" || password == "" {
		return nil
	}
	users, err := s.store.List(ctx)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return nil
	}
	user, err := s.Create(ctx, email, password, "Administrador", RoleAdmin)
	if err != nil {
		return fmt.Errorf("creando el admin inicial: %w", err)
	}
	s.log.Info("administrador inicial creado", "email", user.Email)
	return nil
}

const minPasswordLen = 10

func HashPassword(password string) (string, error) {
	if len(password) < minPasswordLen {
		return "", fmt.Errorf("la contraseña debe tener al menos %d caracteres", minPasswordLen)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hasheando la contraseña: %w", err)
	}
	return string(h), nil
}

// dummyHash es un bcrypt válido de una contraseña arbitraria. Solo se compara
// contra él para gastar el mismo tiempo que un login real cuando el email no
// existe.
var dummyHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")
