package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implementa Store.
//
// Vive en el mismo paquete que la interfaz, a diferencia del espejo, que separa
// puertos y adaptadores: allí hay cinco repositorios y la separación paga; aquí
// hay uno solo. La interfaz Store existe igual, para que los tests del middleware
// y del servicio no necesiten una base.
type PostgresStore struct{ db *pgxpool.Pool }

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore { return &PostgresStore{db: db} }

// userColumns es el proyectado común. `password_hash` NO está aquí: se pide
// aparte y solo en ByEmailWithHash.
const userColumns = `id, email, full_name, role, is_active, last_login_at, created_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.FullName, &u.Role, &u.IsActive, &u.LastLoginAt, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *PostgresStore) ByEmailWithHash(ctx context.Context, email string) (User, string, error) {
	var u User
	var hash string
	err := s.db.QueryRow(ctx,
		`SELECT `+userColumns+`, password_hash FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.FullName, &u.Role, &u.IsActive, &u.LastLoginAt, &u.CreatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrNotFound
	}
	return u, hash, err
}

func (s *PostgresStore) ByID(ctx context.Context, id string) (User, error) {
	// El id llega de la URL o de un token: si no es un uuid, Postgres devolvería
	// un error de sintaxis en vez de "no encontrado".
	if !isUUID(id) {
		return User{}, ErrNotFound
	}
	return scanUser(s.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

func (s *PostgresStore) List(ctx context.Context) ([]User, error) {
	rows, err := s.db.Query(ctx, `SELECT `+userColumns+` FROM users ORDER BY full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *PostgresStore) Create(ctx context.Context, email, hash, fullName string, role Role) (User, error) {
	const q = `INSERT INTO users (email, password_hash, full_name, role)
	           VALUES ($1, $2, $3, $4::user_role)
	           RETURNING ` + userColumns
	u, err := scanUser(s.db.QueryRow(ctx, q, email, hash, fullName, string(role)))
	if isUniqueViolation(err) {
		return User{}, ErrEmailTaken
	}
	return u, err
}

// Update aplica solo los campos no nulos. COALESCE sobre el parámetro deja el
// valor actual cuando llega NULL, así que un PATCH parcial no pisa nada.
func (s *PostgresStore) Update(ctx context.Context, id string, fullName *string, role *Role, isActive *bool, hash *string) (User, error) {
	if !isUUID(id) {
		return User{}, ErrNotFound
	}
	var roleStr *string
	if role != nil {
		v := string(*role)
		roleStr = &v
	}

	const q = `
		UPDATE users
		   SET full_name     = COALESCE($2, full_name),
		       role          = COALESCE($3::user_role, role),
		       is_active     = COALESCE($4, is_active),
		       password_hash = COALESCE($5, password_hash)
		 WHERE id = $1
		 RETURNING ` + userColumns
	return scanUser(s.db.QueryRow(ctx, q, id, fullName, roleStr, isActive, hash))
}

func (s *PostgresStore) Delete(ctx context.Context, id string) error {
	if !isUUID(id) {
		return ErrNotFound
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) TouchLogin(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE users SET last_login_at = $2 WHERE id = $1`, id, at)
	return err
}

func (s *PostgresStore) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx,
		`SELECT count(*) FROM users WHERE role = 'admin' AND is_active`).Scan(&n)
	return n, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// isUUID valida la forma 8-4-4-4-12 sin traer una dependencia solo para esto.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}
