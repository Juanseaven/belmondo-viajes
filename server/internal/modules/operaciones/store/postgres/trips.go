// Package postgres implementa la persistencia del módulo de operaciones.
//
// Lee las tablas públicas del módulo espejo —contacts, conversations y
// messages— y tiene claves foráneas hacia ellas. No escribe ninguna.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) CreateTrip(ctx context.Context, n store.NewTrip) (store.Trip, error) {
	if n.Origin == "" {
		n.Origin = "AR"
	}
	if err := validItinerary(n.Origin, n.Itinerary); err != nil {
		return store.Trip{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.Trip{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	customerID := n.CustomerID
	if customerID == "" {
		if customerID, err = customerFor(ctx, tx, n.ContactID, n.CreatedBy); err != nil {
			return store.Trip{}, err
		}
	} else {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM customers WHERE id = $1)`,
			customerID).Scan(&exists); err != nil {
			return store.Trip{}, writeError("buscando el cliente", err)
		}
		if !exists {
			return store.Trip{}, store.ErrNotFound
		}
	}

	var t store.Trip
	err = tx.QueryRow(ctx,
		`INSERT INTO trips (customer_id, destination, origin_country, depart_at, return_at,
		                    seller_user_id, created_by)
		 VALUES ($1, nullif(btrim($2), ''), $3, nullif($4, '0001-01-01'::date),
		         nullif($5, '0001-01-01'::date), $6, $6)
		 RETURNING id, number, customer_id, status, coalesce(destination, ''), depart_at,
		           return_at, created_at`,
		customerID, n.Destination, n.Origin, n.DepartAt, n.ReturnAt, n.CreatedBy,
	).Scan(&t.ID, &t.Number, &t.CustomerID, &t.Status, &t.Destination,
		&t.DepartAt, &t.ReturnAt, &t.CreatedAt)
	if err != nil {
		return store.Trip{}, fmt.Errorf("insertando el viaje: %w", err)
	}
	t.SellerUserID = n.CreatedBy

	// Sin países, queda el nombre que vino: el viaje se puede empezar por un
	// pasajero o un pago, y el itinerario llega después.
	if len(n.Itinerary) > 0 {
		if err := writeItinerary(ctx, tx, t.ID, n.Origin, n.Itinerary); err != nil {
			return store.Trip{}, err
		}
		t.Destination = TripName(n.Itinerary)
	}

	if err := materializeChecklist(ctx, tx, t); err != nil {
		return store.Trip{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return store.Trip{}, err
	}
	return t, nil
}

func (s *Store) UpdateTrip(ctx context.Context, tripID string, patch store.TripPatch) (store.Trip, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.Trip{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var t store.Trip
	err = tx.QueryRow(ctx,
		`UPDATE trips SET
		   destination = coalesce($2, destination),
		   depart_at = coalesce($3, depart_at),
		   return_at = coalesce($4, return_at),
		   status = coalesce($5, status),
		   seller_user_id = CASE WHEN $6::text IS NULL THEN seller_user_id ELSE nullif($6, '')::uuid END
		 WHERE id = $1
		 RETURNING id, number, customer_id, status, coalesce(destination, ''), depart_at, return_at,
		           coalesce(seller_user_id::text, ''), created_at`,
		tripID, patch.Destination, patch.DepartAt, patch.ReturnAt, patch.Status,
		patch.SellerUserID,
	).Scan(&t.ID, &t.Number, &t.CustomerID, &t.Status, &t.Destination, &t.DepartAt,
		&t.ReturnAt, &t.SellerUserID, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Trip{}, store.ErrNotFound
	}
	if err != nil {
		return store.Trip{}, fmt.Errorf("actualizando el viaje: %w", err)
	}

	ids := make([]string, 0)
	dues := make([]time.Time, 0)
	for _, item := range domain.Checklist() {
		if item.Stage != domain.StageControl {
			continue
		}
		if due, ok := item.DueAt(t.CreatedAt, valueOrZero(t.DepartAt), valueOrZero(t.ReturnAt)); ok {
			ids = append(ids, item.ID)
			dues = append(dues, due)
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE controls c SET due_at = d.due_at
		   FROM unnest($2::text[], $3::date[]) AS d(item_id, due_at)
		  WHERE c.trip_id = $1 AND c.item_id = d.item_id`, tripID, ids, dues); err != nil {
		return store.Trip{}, fmt.Errorf("recalculando vencimientos: %w", err)
	}
	if err := refreshDerived(ctx, tx, tripID); err != nil {
		return store.Trip{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.Trip{}, err
	}
	return t, nil
}

// customerFor devuelve el cliente del contacto, creándolo si no existe.
//
// El nombre sale del que WhatsApp reporta; si viene vacío, del teléfono. Es un
// punto de partida editable, no un dato definitivo: lo que importa es que abrir
// el viaje no obligue a nadie a llenar una ficha de cliente antes de empezar.
func customerFor(ctx context.Context, tx pgx.Tx, contactID, userID string) (string, error) {
	var customerID string
	err := tx.QueryRow(ctx,
		`SELECT customer_id FROM customer_contacts WHERE contact_id = $1`,
		contactID).Scan(&customerID)
	if err == nil {
		return customerID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("buscando el cliente del contacto: %w", err)
	}

	var name, phone string
	err = tx.QueryRow(ctx,
		`SELECT coalesce(nullif(push_name, ''), phone_number, 'Cliente'),
		        coalesce(phone_number, '')
		   FROM contacts WHERE id = $1`, contactID).Scan(&name, &phone)
	if err != nil {
		return "", fmt.Errorf("leyendo el contacto: %w", err)
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO customers (name, phone_number, created_by)
		 VALUES ($1, nullif($2, ''), $3) RETURNING id`,
		name, phone, userID).Scan(&customerID)
	if err != nil {
		return "", fmt.Errorf("creando el cliente: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO customer_contacts (customer_id, contact_id, linked_by)
		 VALUES ($1, $2, $3)`, customerID, contactID, userID)
	if err != nil {
		return "", fmt.Errorf("vinculando el contacto al cliente: %w", err)
	}
	return customerID, nil
}

// materializeChecklist copia la plantilla al viaje con los vencimientos ya
// calculados.
//
// El plazo es un dato de la plantilla y la fecha se resuelve acá, una sola vez.
// Los derivados quedan sin fecha: no hay a quién apurar.
func materializeChecklist(ctx context.Context, tx pgx.Tx, t store.Trip) error {
	items := domain.Checklist()

	ids := make([]string, 0, len(items))
	stages := make([]string, 0, len(items))
	closings := make([]string, 0, len(items))
	criticals := make([]bool, 0, len(items))
	derives := make([]*string, 0, len(items))
	dues := make([]*time.Time, 0, len(items))

	for _, item := range items {
		ids = append(ids, item.ID)
		stages = append(stages, string(item.Stage))
		closings = append(closings, string(item.Closing))
		criticals = append(criticals, item.Critical)

		if item.Derives != "" {
			d := string(item.Derives)
			derives = append(derives, &d)
		} else {
			derives = append(derives, nil)
		}

		if due, ok := item.DueAt(t.CreatedAt, valueOrZero(t.DepartAt), valueOrZero(t.ReturnAt)); ok {
			d := due
			dues = append(dues, &d)
		} else {
			dues = append(dues, nil)
		}
	}

	_, err := tx.Exec(ctx,
		`INSERT INTO controls (trip_id, item_id, stage, closing, critical, derives, due_at)
		 SELECT $1, item_id, stage, closing, critical, derives, due_at
		   FROM unnest($2::text[], $3::text[], $4::text[], $5::bool[], $6::text[], $7::date[])
		        AS t(item_id, stage, closing, critical, derives, due_at)`,
		t.ID, ids, stages, closings, criticals, derives, dues)
	if err != nil {
		return fmt.Errorf("materializando los controles: %w", err)
	}
	return refreshDerived(ctx, tx, t.ID)
}

func valueOrZero(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}
