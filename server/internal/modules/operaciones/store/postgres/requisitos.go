package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/evaluacion"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
	reglas "github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

// SaveRequirements reemplaza la evaluación del viaje, guarda el documento que
// eligió para cada pasajero (si nadie lo eligió a mano) y recalcula los
// controles que la siguen, en una transacción. El bloqueo del viaje serializa
// dos evaluaciones simultáneas: la última que termina es la que queda.
func (s *Store) SaveRequirements(ctx context.Context, tripID string, reqs []evaluacion.Requirement, docs map[string]string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var locked string
	if err := tx.QueryRow(ctx, `SELECT id FROM trips WHERE id = $1 FOR UPDATE`, tripID).Scan(&locked); err != nil {
		return writeError("leyendo el viaje", err)
	}
	for passengerID, docID := range docs {
		// El documento tiene que ser del pasajero: lo garantiza la clave
		// compuesta. Uno que alguien eligió a mano no se toca.
		if _, err := tx.Exec(ctx,
			`UPDATE passengers SET travel_document_id = $3
			  WHERE id = $1 AND trip_id = $2 AND NOT travel_document_manual
			    AND travel_document_id IS DISTINCT FROM $3::uuid`, passengerID, tripID, docID); err != nil {
			return writeError("guardando el documento de viaje", err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM trip_requirements WHERE trip_id = $1`, tripID); err != nil {
		return fmt.Errorf("borrando la evaluación anterior: %w", err)
	}
	for i, r := range reqs {
		sources, err := json.Marshal(r.Sources)
		if err != nil {
			return err
		}
		if r.Sources == nil {
			sources = []byte("[]")
		}
		detail, err := json.Marshal(r.Detail)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO trip_requirements (trip_id, position, passenger_id, country, role, key, status, text,
			                                client_text, confirmable, confirmed, missing_data, sources, field_hash,
			                                detail, searching)
			 VALUES ($1, $2, nullif($3, '')::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
			tripID, i, r.PassengerID, r.Country, r.Role, r.Key, r.Status, r.Text, r.ClientText,
			r.Confirmable, r.Confirmed, r.MissingData, sources, r.FieldHash, detail, r.Searching); err != nil {
			return writeError("guardando la evaluación", err)
		}
	}
	if err := refreshDerived(ctx, tx, tripID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func readRequirements(ctx context.Context, q querier, tripID string) ([]store.TripRequirement, error) {
	rows, err := q.Query(ctx,
		`SELECT coalesce(r.passenger_id::text, ''), r.country, r.role, r.key, r.status, r.text, r.client_text,
		        r.confirmable, r.confirmed, r.missing_data, r.sources, r.field_hash, r.detail, r.searching,
		        coalesce(u.full_name, ''), c.confirmed_at
		   FROM trip_requirements r
		   LEFT JOIN LATERAL (
		     SELECT rc.confirmed_by, rc.confirmed_at FROM requirement_confirmations rc
		      WHERE r.confirmed AND rc.trip_id = r.trip_id AND rc.passenger_id = r.passenger_id
		        AND rc.country = r.country AND rc.key = r.key AND rc.field_hash = r.field_hash
		      ORDER BY rc.confirmed_at DESC LIMIT 1
		   ) c ON true
		   LEFT JOIN users u ON u.id = c.confirmed_by
		  WHERE r.trip_id = $1 ORDER BY r.position`, tripID)
	if err != nil {
		return nil, fmt.Errorf("leyendo requisitos: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (store.TripRequirement, error) {
		var r store.TripRequirement
		var sources, detail []byte
		err := row.Scan(&r.PassengerID, &r.Country, &r.Role, &r.Key, &r.Status, &r.Text, &r.ClientText,
			&r.Confirmable, &r.Confirmed, &r.MissingData, &sources, &r.FieldHash, &detail, &r.Searching,
			&r.ConfirmedByName, &r.ConfirmedAt)
		if err == nil {
			err = json.Unmarshal(sources, &r.Sources)
		}
		if err == nil {
			err = json.Unmarshal(detail, &r.Detail)
		}
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("armando requisitos: %w", err)
	}
	return out, nil
}

// Confirm registra que una persona revisó un requisito y cumple. Solo vale
// para los confirmables que no cumplen todavía, y queda atada al valor de la
// regla con que se evaluó.
func (s *Store) Confirm(ctx context.Context, c store.NewConfirmation) error {
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT field_hash FROM trip_requirements
		  WHERE trip_id = $1 AND passenger_id = $2 AND country = $3 AND key = $4
		    AND confirmable AND status <> 'cumple'`,
		c.TripID, c.PassengerID, c.Country, c.Key).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrNoConfirmable
	}
	if err != nil {
		return writeError("leyendo el requisito", err)
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO requirement_confirmations (trip_id, passenger_id, country, key, field_hash, confirmed_by, message_id)
		 VALUES ($1, $2, $3, $4, $5, $6, nullif($7, '')::uuid)`,
		c.TripID, c.PassengerID, c.Country, c.Key, hash, c.UserID, c.MessageID); err != nil {
		return writeError("guardando la confirmación", err)
	}
	return nil
}

// EvaluationInput junta lo que la evaluación necesita del viaje.
func (s *Store) EvaluationInput(ctx context.Context, tripID string) (store.EvaluationInput, error) {
	var in store.EvaluationInput
	err := s.pool.QueryRow(ctx, `SELECT origin_country, depart_at, return_at FROM trips WHERE id = $1`, tripID).
		Scan(&in.Trip.Origin, &in.Trip.DepartAt, &in.Trip.ReturnAt)
	if err != nil {
		return store.EvaluationInput{}, writeError("leyendo el viaje", err)
	}
	stops, err := readItinerary(ctx, s.pool, tripID)
	if err != nil {
		return store.EvaluationInput{}, err
	}
	for _, st := range stops {
		in.Trip.Itinerary = append(in.Trip.Itinerary, evaluacion.Stop{Country: st.Country, Role: st.Role,
			ArriveAt: st.ArriveAt, LeaveAt: st.LeaveAt})
	}
	passengers, err := readPassengers(ctx, s.pool, tripID)
	if err != nil {
		return store.EvaluationInput{}, err
	}
	for _, p := range passengers {
		in.Trip.Passengers = append(in.Trip.Passengers, ToEvaluation(p))
	}

	rows, err := s.pool.Query(ctx,
		`SELECT passenger_id::text, country, key, field_hash FROM requirement_confirmations WHERE trip_id = $1`, tripID)
	if err != nil {
		return store.EvaluationInput{}, fmt.Errorf("leyendo confirmaciones: %w", err)
	}
	in.Confirmations, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (evaluacion.Confirmation, error) {
		var c evaluacion.Confirmation
		err := row.Scan(&c.PassengerID, &c.Country, &c.Key, &c.FieldHash)
		return c, err
	})
	if err != nil {
		return store.EvaluationInput{}, fmt.Errorf("armando confirmaciones: %w", err)
	}
	return in, nil
}

// ToEvaluation traduce un pasajero de la ficha al que lee la evaluación: el
// documento de viaje y sus permisos.
func ToEvaluation(p store.Passenger) evaluacion.Passenger {
	out := evaluacion.Passenger{ID: p.ID, Name: p.FirstName, Minor: p.Kind == "menor", ResidenceCountry: p.ResidenceCountry,
		DocManual: p.TravelDocumentManual}
	for _, d := range p.Documents {
		doc := evaluacion.Document{ID: d.ID, Type: d.Type, Country: d.Country, Number: d.Number,
			IssuedAt: d.IssuedAt, ExpiresAt: d.ExpiresAt}
		out.Docs = append(out.Docs, doc)
		if d.ID == p.TravelDocumentID {
			out.Doc = &doc
		}
	}
	for _, pm := range p.Permits {
		out.Permits = append(out.Permits, evaluacion.Permit{Kind: pm.Kind, Country: pm.Country, Label: pm.Label,
			ValidFrom: pm.ValidFrom, ExpiresAt: pm.ExpiresAt})
	}
	return out
}

// OpenTripsUsing devuelve los viajes no cerrados en los que algún pasajero
// viaja —o podría viajar, si el sistema elige su documento— con un documento de
// esa nacionalidad a ese país, y los que todavía no tienen pasajeros y muestran
// lo que se le pide a un pasaporte argentino.
func (s *Store) OpenTripsUsing(ctx context.Context, pairs []reglas.Pair) ([]string, error) {
	nats := make([]string, len(pairs))
	countries := make([]string, len(pairs))
	for i, p := range pairs {
		nats[i], countries[i] = p.Nationality, p.Country
	}
	return s.tripIDs(ctx,
		`SELECT DISTINCT t.id::text
		   FROM trips t
		   JOIN trip_countries tc ON tc.trip_id = t.id
		   JOIN unnest($1::text[], $2::text[]) AS r(nationality, country) ON r.country = tc.country
		  WHERE t.status <> 'cerrado'
		    AND (EXISTS (SELECT 1 FROM passengers p
		                   JOIN passenger_documents d ON d.passenger_id = p.id
		                  WHERE p.trip_id = t.id AND d.country = r.nationality
		                    AND (d.id = p.travel_document_id OR NOT p.travel_document_manual))
		         OR (r.nationality = '`+evaluacion.PreviewNationality+`'
		             AND NOT EXISTS (SELECT 1 FROM passengers p WHERE p.trip_id = t.id)))`, nats, countries)
}

// OpenTrips devuelve todos los viajes no cerrados, para evaluarlos al arrancar.
func (s *Store) OpenTrips(ctx context.Context) ([]string, error) {
	return s.tripIDs(ctx, `SELECT id::text FROM trips WHERE status <> 'cerrado' ORDER BY created_at`)
}

func (s *Store) tripIDs(ctx context.Context, sql string, args ...any) ([]string, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("listando viajes: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// notifiedRequirements son los requisitos en falta al momento de un aviso de
// requisitos migratorios. Falla si queda algo por verificar: eso es trabajo de
// la agencia y el aviso no lo cubre. Las recomendaciones no son faltantes.
func notifiedRequirements(ctx context.Context, tx pgx.Tx, tripID string) ([]byte, error) {
	var pendingVerify bool
	var notified []string
	err := tx.QueryRow(ctx,
		`SELECT coalesce(bool_or(status = 'verificar'), false),
		        coalesce(array_agg(`+requirementID+`) FILTER (WHERE status = 'falta'), '{}')
		   FROM trip_requirements r
		  WHERE trip_id = $1 AND status NOT IN ('cumple', 'aviso') AND key NOT IN ('vigencia', 'antiguedad')`,
		tripID).Scan(&pendingVerify, &notified)
	if err != nil {
		return nil, fmt.Errorf("leyendo los requisitos avisados: %w", err)
	}
	if pendingVerify {
		return nil, store.ErrQuedaPorVerificar
	}
	return json.Marshal(notified)
}

// requirementID identifica un requisito entre evaluaciones: pasajero, país y
// clave. Lo usan el aviso y el recálculo para saber si un requisito es nuevo.
const requirementID = `(coalesce(r.passenger_id::text, '') || '|' || r.country || '|' || r.key)`
