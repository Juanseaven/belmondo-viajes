package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

func (s *Store) AddPassenger(ctx context.Context, n store.NewPassenger) (store.Passenger, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.Passenger{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var p store.Passenger
	err = tx.QueryRow(ctx,
		`INSERT INTO passengers
		 (trip_id, first_name, last_name, kind, is_holder, document_type,
		  document_number, document_expires_at, created_by)
		 VALUES ($1, $2, $3, $4, $5, nullif($6, ''), nullif($7, ''), $8, $9)
		 RETURNING id, trip_id, first_name, last_name, kind, is_holder,
		           coalesce(document_type, ''), coalesce(document_number, ''), document_expires_at`,
		n.TripID, n.FirstName, n.LastName, n.Kind, n.IsHolder, n.DocumentType,
		n.DocumentNumber, n.DocumentExpiresAt, n.CreatedBy,
	).Scan(&p.ID, &p.TripID, &p.FirstName, &p.LastName, &p.Kind, &p.IsHolder,
		&p.DocumentType, &p.DocumentNumber, &p.DocumentExpiresAt)
	if err != nil {
		return store.Passenger{}, writeError("creando el pasajero", err)
	}
	if err := refreshDerived(ctx, tx, n.TripID); err != nil {
		return store.Passenger{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.Passenger{}, err
	}
	return p, nil
}

func (s *Store) UpdatePassenger(ctx context.Context, passengerID string, patch store.PassengerPatch) (store.Passenger, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.Passenger{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var p store.Passenger
	err = tx.QueryRow(ctx,
		`UPDATE passengers SET
		   first_name = coalesce($2, first_name),
		   last_name = coalesce($3, last_name),
		   kind = coalesce($4, kind),
		   is_holder = coalesce($5, is_holder),
		   document_type = CASE WHEN $6::text IS NULL THEN document_type ELSE nullif($6, '') END,
		   document_number = CASE WHEN $7::text IS NULL THEN document_number ELSE nullif($7, '') END,
		   document_expires_at = CASE WHEN $8::text IS NULL THEN document_expires_at ELSE nullif($8, '')::date END
		 WHERE id = $1
		 RETURNING id, trip_id, first_name, last_name, kind, is_holder,
		           coalesce(document_type, ''), coalesce(document_number, ''), document_expires_at`,
		passengerID, patch.FirstName, patch.LastName, patch.Kind, patch.IsHolder,
		patch.DocumentType, patch.DocumentNumber, patch.DocumentExpiresAt,
	).Scan(&p.ID, &p.TripID, &p.FirstName, &p.LastName, &p.Kind, &p.IsHolder,
		&p.DocumentType, &p.DocumentNumber, &p.DocumentExpiresAt)
	if err != nil {
		return store.Passenger{}, writeError("actualizando el pasajero", err)
	}
	if err := refreshDerived(ctx, tx, p.TripID); err != nil {
		return store.Passenger{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.Passenger{}, err
	}
	return p, nil
}

func (s *Store) DeletePassenger(ctx context.Context, passengerID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var tripID string
	err = tx.QueryRow(ctx,
		`DELETE FROM passengers WHERE id = $1 RETURNING trip_id`, passengerID).Scan(&tripID)
	if err != nil {
		return writeError("borrando el pasajero", err)
	}
	if err := refreshDerived(ctx, tx, tripID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func writeError(action string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return store.ErrConflict
	}
	return fmt.Errorf("%s: %w", action, err)
}
