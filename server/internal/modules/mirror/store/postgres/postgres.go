// Package postgres implementa los puertos de store del espejo sobre PostgreSQL
// con pgx. El pool lo abre el kernel (internal/kernel/db) y llega por constructor.
package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// uniqueViolation es el SQLSTATE que devuelve Postgres al chocar contra un índice
// único. Es la señal que convierte los reintentos de Evolution en no-ops.
const uniqueViolation = "23505"

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}
