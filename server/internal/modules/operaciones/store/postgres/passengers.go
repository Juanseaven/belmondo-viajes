package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// querier es lo que necesitan las lecturas que corren tanto sueltas como dentro
// de una transacción de escritura.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (s *Store) AddPassenger(ctx context.Context, n store.NewPassenger) (store.Passenger, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.Passenger{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	err = tx.QueryRow(ctx,
		`INSERT INTO passengers
		 (trip_id, first_name, last_name, kind, is_holder, birth_date, residence_country, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, coalesce(nullif($7, ''), 'AR'), $8)
		 RETURNING id`,
		n.TripID, n.FirstName, n.LastName, n.Kind, n.IsHolder, n.BirthDate, n.ResidenceCountry, n.CreatedBy,
	).Scan(&id)
	if err != nil {
		return store.Passenger{}, writeError("creando el pasajero", err)
	}

	if n.DocumentType != "" || n.DocumentNumber != "" || n.DocumentExpiresAt != nil {
		docType := n.DocumentType
		if docType == "" {
			docType = "DNI"
		}
		country := n.DocumentCountry
		if country == "" {
			country = "AR"
		}
		if _, err := insertDocument(ctx, tx, store.NewDocument{PassengerID: id, Type: docType,
			Country: country, Number: n.DocumentNumber, ExpiresAt: n.DocumentExpiresAt}); err != nil {
			return store.Passenger{}, err
		}
	}

	if err := refreshDerived(ctx, tx, n.TripID); err != nil {
		return store.Passenger{}, err
	}
	p, err := readPassenger(ctx, tx, id)
	if err != nil {
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

	if patch.IsHolder != nil && *patch.IsHolder {
		if err := releaseHolder(ctx, tx, passengerID); err != nil {
			return store.Passenger{}, err
		}
	}
	var tripID string
	err = tx.QueryRow(ctx,
		`UPDATE passengers SET
		   first_name = coalesce($2, first_name),
		   last_name = coalesce($3, last_name),
		   kind = coalesce($4, kind),
		   is_holder = coalesce($5, is_holder),
		   birth_date = CASE WHEN $6::text IS NULL THEN birth_date ELSE nullif($6, '')::date END,
		   residence_country = coalesce(nullif($7, ''), residence_country),
		   travel_document_id = CASE WHEN $8::text IS NULL THEN travel_document_id ELSE nullif($8, '')::uuid END
		 WHERE id = $1
		 RETURNING trip_id`,
		passengerID, patch.FirstName, patch.LastName, patch.Kind, patch.IsHolder,
		patch.BirthDate, patch.ResidenceCountry, patch.TravelDocumentID,
	).Scan(&tripID)
	if err != nil {
		return store.Passenger{}, writeError("actualizando el pasajero", err)
	}
	if err := refreshDerived(ctx, tx, tripID); err != nil {
		return store.Passenger{}, err
	}
	p, err := readPassenger(ctx, tx, passengerID)
	if err != nil {
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

// releaseHolder le quita la titularidad a quien la tenga en el viaje del
// pasajero que la recibe. Solo la usa la edición: marcar como titular a alguien
// ya cargado es una corrección explícita. El alta, en cambio, sigue chocando con
// el índice: un pasajero nuevo no desplaza en silencio al titular registrado.
func releaseHolder(ctx context.Context, tx pgx.Tx, newHolderID string) error {
	_, err := tx.Exec(ctx,
		`UPDATE passengers SET is_holder = false
		  WHERE is_holder AND id <> $1
		    AND trip_id = (SELECT trip_id FROM passengers WHERE id = $1)`,
		newHolderID)
	if err != nil {
		return fmt.Errorf("liberando la titularidad: %w", err)
	}
	return nil
}

// sqlPassengers lee los pasajeros con el «menor» ya resuelto.
//
// Con fecha de nacimiento, es menor quien no cumplió 18 a la fecha de salida:
// la autorización se pide al salir, así que quien los cumple durante el viaje
// sale siendo menor. Sin salida cargada, se mide a hoy.
const sqlPassengers = `
SELECT p.id, p.trip_id, p.first_name, p.last_name,
       CASE WHEN p.birth_date IS NULL THEN p.kind
            WHEN p.birth_date + interval '18 years' > coalesce(t.depart_at, current_date) THEN 'menor'
            ELSE 'adulto' END,
       p.is_holder, p.birth_date, p.residence_country, coalesce(p.travel_document_id::text, '')
  FROM passengers p
  JOIN trips t ON t.id = p.trip_id`

// readPassengers devuelve los pasajeros de un viaje con sus documentos y
// permisos, en el orden de la ficha: el titular primero.
func readPassengers(ctx context.Context, q querier, tripID string) ([]store.Passenger, error) {
	rows, err := q.Query(ctx, sqlPassengers+`
		 WHERE p.trip_id = $1 ORDER BY p.is_holder DESC, p.created_at, p.id`, tripID)
	if err != nil {
		return nil, fmt.Errorf("leyendo pasajeros: %w", err)
	}
	passengers, err := pgx.CollectRows(rows, scanPassenger)
	if err != nil {
		return nil, fmt.Errorf("armando pasajeros: %w", err)
	}
	if err := attachDocuments(ctx, q, passengers); err != nil {
		return nil, err
	}
	return passengers, nil
}

func readPassenger(ctx context.Context, q querier, passengerID string) (store.Passenger, error) {
	rows, err := q.Query(ctx, sqlPassengers+` WHERE p.id = $1`, passengerID)
	if err != nil {
		return store.Passenger{}, fmt.Errorf("leyendo el pasajero: %w", err)
	}
	passengers, err := pgx.CollectRows(rows, scanPassenger)
	if err != nil {
		return store.Passenger{}, fmt.Errorf("armando el pasajero: %w", err)
	}
	if len(passengers) == 0 {
		return store.Passenger{}, store.ErrNotFound
	}
	if err := attachDocuments(ctx, q, passengers); err != nil {
		return store.Passenger{}, err
	}
	return passengers[0], nil
}

func scanPassenger(row pgx.CollectableRow) (store.Passenger, error) {
	p := store.Passenger{Documents: []store.Document{}, Permits: []store.Permit{}}
	err := row.Scan(&p.ID, &p.TripID, &p.FirstName, &p.LastName, &p.Kind, &p.IsHolder,
		&p.BirthDate, &p.ResidenceCountry, &p.TravelDocumentID)
	return p, err
}

// attachDocuments completa documentos y permisos en dos consultas, no en una
// por pasajero.
func attachDocuments(ctx context.Context, q querier, passengers []store.Passenger) error {
	if len(passengers) == 0 {
		return nil
	}
	ids := make([]string, len(passengers))
	byID := make(map[string]*store.Passenger, len(passengers))
	for i := range passengers {
		ids[i] = passengers[i].ID
		byID[passengers[i].ID] = &passengers[i]
	}

	rows, err := q.Query(ctx,
		`SELECT d.id, d.passenger_id, p.trip_id, d.type, d.country, coalesce(d.number, ''), d.issued_at, d.expires_at
		   FROM passenger_documents d JOIN passengers p ON p.id = d.passenger_id
		  WHERE d.passenger_id = ANY($1::uuid[]) ORDER BY d.created_at, d.id`, ids)
	if err != nil {
		return fmt.Errorf("leyendo documentos: %w", err)
	}
	docs, err := pgx.CollectRows(rows, scanDocument)
	if err != nil {
		return fmt.Errorf("armando documentos: %w", err)
	}
	for _, d := range docs {
		byID[d.PassengerID].Documents = append(byID[d.PassengerID].Documents, d)
	}

	rows, err = q.Query(ctx,
		`SELECT pm.id, pm.passenger_id, p.trip_id, pm.kind, pm.country, coalesce(pm.label, ''), pm.valid_from, pm.expires_at
		   FROM passenger_permits pm JOIN passengers p ON p.id = pm.passenger_id
		  WHERE pm.passenger_id = ANY($1::uuid[]) ORDER BY pm.created_at, pm.id`, ids)
	if err != nil {
		return fmt.Errorf("leyendo permisos: %w", err)
	}
	permits, err := pgx.CollectRows(rows, scanPermit)
	if err != nil {
		return fmt.Errorf("armando permisos: %w", err)
	}
	for _, pm := range permits {
		byID[pm.PassengerID].Permits = append(byID[pm.PassengerID].Permits, pm)
	}
	return nil
}

func writeError(action string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "23503":
			// Duplicado, o una referencia que no corresponde (el documento de
			// viaje de otro pasajero).
			return store.ErrConflict
		case "23514", "22P02", "22007", "22008":
			// Un CHECK del esquema, un uuid o una fecha mal escritos.
			return fmt.Errorf("%w: %s", store.ErrInvalid, pgErr.Message)
		}
	}
	return fmt.Errorf("%s: %w", action, err)
}
