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

// isUUID valida la forma 8-4-4-4-12.
//
// Los ids llegan de la URL. Sin esta comprobación, un id con basura hace que
// Postgres devuelva un error de sintaxis y el handler responda 500 a lo que en
// realidad es una petición mal formada de un cliente.
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
