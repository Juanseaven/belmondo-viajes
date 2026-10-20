package postgres

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
	reglas "github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

// AddDocument carga un documento del pasajero. Si todavía no viajaba con
// ninguno, pasa a viajar con este: el primero que se carga es el de viaje.
func (s *Store) AddDocument(ctx context.Context, n store.NewDocument) (store.Document, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.Document{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	d, err := insertDocument(ctx, tx, n)
	if err != nil {
		return store.Document{}, err
	}
	if err := refreshDerived(ctx, tx, d.TripID); err != nil {
		return store.Document{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.Document{}, err
	}
	return d, nil
}

func insertDocument(ctx context.Context, tx pgx.Tx, n store.NewDocument) (store.Document, error) {
	row := tx.QueryRow(ctx,
		`WITH nuevo AS (
		   INSERT INTO passenger_documents (passenger_id, type, country, number, issued_at, expires_at)
		   VALUES ($1, $2, $3, nullif($4, ''), $5, $6)
		   RETURNING id, passenger_id, type, country, coalesce(number, '') AS number, issued_at, expires_at
		 )
		 SELECT n.id, n.passenger_id, p.trip_id, n.type, n.country, n.number, n.issued_at, n.expires_at
		   FROM nuevo n JOIN passengers p ON p.id = n.passenger_id`,
		n.PassengerID, n.Type, n.Country, n.Number, n.IssuedAt, n.ExpiresAt)
	var d store.Document
	if err := row.Scan(&d.ID, &d.PassengerID, &d.TripID, &d.Type, &d.Country, &d.Number,
		&d.IssuedAt, &d.ExpiresAt); err != nil {
		return store.Document{}, documentError("creando el documento", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE passengers SET travel_document_id = $2
		  WHERE id = $1 AND travel_document_id IS NULL`, d.PassengerID, d.ID); err != nil {
		return store.Document{}, fmt.Errorf("eligiendo el documento de viaje: %w", err)
	}
	return d, nil
}

func (s *Store) UpdateDocument(ctx context.Context, documentID string, patch store.DocumentPatch) (store.Document, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.Document{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var d store.Document
	err = tx.QueryRow(ctx,
		`WITH editado AS (
		   UPDATE passenger_documents SET
		     type = coalesce($2, type),
		     country = coalesce($3, country),
		     number = CASE WHEN $4::text IS NULL THEN number ELSE nullif($4, '') END,
		     issued_at = CASE WHEN $5::text IS NULL THEN issued_at ELSE nullif($5, '')::date END,
		     expires_at = CASE WHEN $6::text IS NULL THEN expires_at ELSE nullif($6, '')::date END
		   WHERE id = $1
		   RETURNING id, passenger_id, type, country, coalesce(number, '') AS number, issued_at, expires_at
		 )
		 SELECT e.id, e.passenger_id, p.trip_id, e.type, e.country, e.number, e.issued_at, e.expires_at
		   FROM editado e JOIN passengers p ON p.id = e.passenger_id`,
		documentID, patch.Type, patch.Country, patch.Number, patch.IssuedAt, patch.ExpiresAt,
	).Scan(&d.ID, &d.PassengerID, &d.TripID, &d.Type, &d.Country, &d.Number, &d.IssuedAt, &d.ExpiresAt)
	if err != nil {
		return store.Document{}, documentError("actualizando el documento", err)
	}
	if err := refreshDerived(ctx, tx, d.TripID); err != nil {
		return store.Document{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.Document{}, err
	}
	return d, nil
}

// DeleteDocument borra un documento y devuelve su viaje. Si era el de viaje y
// el pasajero tiene otro, pasa a viajar con el más antiguo de los que quedan.
func (s *Store) DeleteDocument(ctx context.Context, documentID string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var passengerID, tripID string
	err = tx.QueryRow(ctx,
		`WITH borrado AS (DELETE FROM passenger_documents WHERE id = $1 RETURNING passenger_id)
		 SELECT b.passenger_id, p.trip_id FROM borrado b JOIN passengers p ON p.id = b.passenger_id`,
		documentID).Scan(&passengerID, &tripID)
	if err != nil {
		return "", writeError("borrando el documento", err)
	}
	// Si se borró el documento de viaje, queda otro hasta la próxima evaluación,
	// y si era el que alguien eligió a mano, la elección vuelve al sistema.
	if _, err := tx.Exec(ctx,
		`UPDATE passengers p SET travel_document_manual = false, travel_document_id = (
		     SELECT d.id FROM passenger_documents d
		      WHERE d.passenger_id = p.id ORDER BY d.created_at, d.id LIMIT 1)
		  WHERE p.id = $1 AND p.travel_document_id IS NULL`, passengerID); err != nil {
		return "", fmt.Errorf("eligiendo otro documento de viaje: %w", err)
	}
	if err := refreshDerived(ctx, tx, tripID); err != nil {
		return "", err
	}
	return tripID, tx.Commit(ctx)
}

func (s *Store) AddPermit(ctx context.Context, n store.NewPermit) (store.Permit, error) {
	var pm store.Permit
	err := s.pool.QueryRow(ctx,
		`WITH nuevo AS (
		   INSERT INTO passenger_permits (passenger_id, kind, country, label, valid_from, expires_at)
		   VALUES ($1, $2, $3, nullif($4, ''), $5, $6)
		   RETURNING id, passenger_id, kind, country, coalesce(label, '') AS label, valid_from, expires_at
		 )
		 SELECT n.id, n.passenger_id, p.trip_id, n.kind, n.country, n.label, n.valid_from, n.expires_at
		   FROM nuevo n JOIN passengers p ON p.id = n.passenger_id`,
		n.PassengerID, n.Kind, n.Country, n.Label, n.ValidFrom, n.ExpiresAt,
	).Scan(&pm.ID, &pm.PassengerID, &pm.TripID, &pm.Kind, &pm.Country, &pm.Label, &pm.ValidFrom, &pm.ExpiresAt)
	if err != nil {
		return store.Permit{}, documentError("creando el permiso", err)
	}
	return pm, nil
}

func (s *Store) UpdatePermit(ctx context.Context, permitID string, patch store.PermitPatch) (store.Permit, error) {
	var pm store.Permit
	err := s.pool.QueryRow(ctx,
		`WITH editado AS (
		   UPDATE passenger_permits SET
		     kind = coalesce($2, kind),
		     country = coalesce($3, country),
		     label = CASE WHEN $4::text IS NULL THEN label ELSE nullif($4, '') END,
		     valid_from = CASE WHEN $5::text IS NULL THEN valid_from ELSE nullif($5, '')::date END,
		     expires_at = CASE WHEN $6::text IS NULL THEN expires_at ELSE nullif($6, '')::date END
		   WHERE id = $1
		   RETURNING id, passenger_id, kind, country, coalesce(label, '') AS label, valid_from, expires_at
		 )
		 SELECT e.id, e.passenger_id, p.trip_id, e.kind, e.country, e.label, e.valid_from, e.expires_at
		   FROM editado e JOIN passengers p ON p.id = e.passenger_id`,
		permitID, patch.Kind, patch.Country, patch.Label, patch.ValidFrom, patch.ExpiresAt,
	).Scan(&pm.ID, &pm.PassengerID, &pm.TripID, &pm.Kind, &pm.Country, &pm.Label, &pm.ValidFrom, &pm.ExpiresAt)
	if err != nil {
		return store.Permit{}, documentError("actualizando el permiso", err)
	}
	return pm, nil
}

// DeletePermit devuelve el viaje del permiso borrado, para avisar el cambio.
func (s *Store) DeletePermit(ctx context.Context, permitID string) (string, error) {
	var tripID string
	err := s.pool.QueryRow(ctx,
		`WITH borrado AS (DELETE FROM passenger_permits WHERE id = $1 RETURNING passenger_id)
		 SELECT p.trip_id FROM borrado b JOIN passengers p ON p.id = b.passenger_id`,
		permitID).Scan(&tripID)
	if err != nil {
		return "", writeError("borrando el permiso", err)
	}
	return tripID, nil
}

// documentError distingue el pasajero inexistente (una clave foránea rota en el
// alta) del dato inválido.
func documentError(action string, err error) error {
	mapped := writeError(action, err)
	if mapped == store.ErrConflict {
		return store.ErrNotFound
	}
	return mapped
}

var isoCountry = regexp.MustCompile(`^[A-Z]{2}$`)

// PutItinerary reemplaza el itinerario entero y, si viene, el origen. Es una
// lista corta que se edita como una sola cosa: guardarla de a filas solo
// agregaría estados intermedios.
func (s *Store) PutItinerary(ctx context.Context, tripID string, it store.Itinerary) error {
	if err := validItinerary(it.Origin, it.Stops); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// El bloqueo serializa dos ediciones simultáneas del mismo itinerario.
	var origin string
	if err := tx.QueryRow(ctx, `SELECT origin_country FROM trips WHERE id = $1 FOR UPDATE`, tripID).Scan(&origin); err != nil {
		return writeError("leyendo el viaje", err)
	}
	if it.Origin != "" {
		origin = it.Origin
	}
	if err := writeItinerary(ctx, tx, tripID, origin, it.Stops); err != nil {
		return err
	}
	if err := refreshDerived(ctx, tx, tripID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// validItinerary revisa lo que no necesita la base. Que el origen no esté entre
// los países lo revisa writeItinerary, que es quien sabe el origen guardado.
func validItinerary(origin string, stops []store.Stop) error {
	if origin != "" && !isoCountry.MatchString(origin) {
		return fmt.Errorf("%w: %q no es un código de país", store.ErrInvalid, origin)
	}
	seen := make(map[string]bool, len(stops))
	for _, stop := range stops {
		switch {
		case !isoCountry.MatchString(stop.Country):
			return fmt.Errorf("%w: %q no es un código de país", store.ErrInvalid, stop.Country)
		case stop.Role != "destino" && stop.Role != "escala":
			return fmt.Errorf("%w: rol %q", store.ErrInvalid, stop.Role)
		case stop.ArriveAt != nil && stop.LeaveAt != nil && stop.LeaveAt.Before(*stop.ArriveAt):
			return fmt.Errorf("%w: en %s la salida es anterior a la llegada", store.ErrInvalid, stop.Country)
		case seen[stop.Country]:
			return fmt.Errorf("%w: %s aparece dos veces", store.ErrInvalid, stop.Country)
		}
		seen[stop.Country] = true
	}
	return nil
}

// writeItinerary guarda el origen, los países y el nombre del viaje, que se
// arma con los destinos.
func writeItinerary(ctx context.Context, tx pgx.Tx, tripID, origin string, stops []store.Stop) error {
	for _, stop := range stops {
		if stop.Country == origin {
			name := countryName(origin)
			return fmt.Errorf("%w: el viaje sale de %s, no puede ser también %s", store.ErrInvalid, name, stop.Role)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM trip_countries WHERE trip_id = $1`, tripID); err != nil {
		return fmt.Errorf("vaciando el itinerario: %w", err)
	}
	for i, stop := range stops {
		if _, err := tx.Exec(ctx,
			`INSERT INTO trip_countries (trip_id, position, country, role, arrive_at, leave_at)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			tripID, i, stop.Country, stop.Role, stop.ArriveAt, stop.LeaveAt); err != nil {
			return writeError("guardando el itinerario", err)
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE trips SET origin_country = $2, destination = nullif($3, '') WHERE id = $1`,
		tripID, origin, TripName(stops)); err != nil {
		return writeError("guardando el origen", err)
	}
	return nil
}

// TripName es el nombre del viaje: sus destinos en orden, sin las escalas.
// «México», «Italia y Francia», «Francia e Italia».
func TripName(stops []store.Stop) string {
	var names []string
	for _, stop := range stops {
		if stop.Role == "destino" {
			names = append(names, countryName(stop.Country))
		}
	}
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	last := names[len(names)-1]
	// «y» pasa a «e» antes del sonido i: «Francia e Italia».
	and := " y "
	if lower := strings.ToLower(last); (strings.HasPrefix(lower, "i") || strings.HasPrefix(lower, "í") ||
		strings.HasPrefix(lower, "hi")) && !strings.HasPrefix(lower, "hie") && !strings.HasPrefix(lower, "hia") {
		and = " e "
	}
	return strings.Join(names[:len(names)-1], ", ") + and + last
}

func countryName(code string) string {
	if c, ok := reglas.CountryByCode(code); ok {
		return c.Name
	}
	return code
}

func readItinerary(ctx context.Context, q querier, tripID string) ([]store.Stop, error) {
	rows, err := q.Query(ctx,
		`SELECT country, role, arrive_at, leave_at FROM trip_countries
		  WHERE trip_id = $1 ORDER BY position`, tripID)
	if err != nil {
		return nil, fmt.Errorf("leyendo el itinerario: %w", err)
	}
	stops, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (store.Stop, error) {
		var s store.Stop
		err := row.Scan(&s.Country, &s.Role, &s.ArriveAt, &s.LeaveAt)
		return s, err
	})
	if err != nil {
		return nil, fmt.Errorf("armando el itinerario: %w", err)
	}
	return stops, nil
}

func scanDocument(row pgx.CollectableRow) (store.Document, error) {
	var d store.Document
	err := row.Scan(&d.ID, &d.PassengerID, &d.TripID, &d.Type, &d.Country, &d.Number, &d.IssuedAt, &d.ExpiresAt)
	return d, err
}

func scanPermit(row pgx.CollectableRow) (store.Permit, error) {
	var pm store.Permit
	err := row.Scan(&pm.ID, &pm.PassengerID, &pm.TripID, &pm.Kind, &pm.Country, &pm.Label, &pm.ValidFrom, &pm.ExpiresAt)
	return pm, err
}
