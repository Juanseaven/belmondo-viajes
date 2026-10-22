package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// Con quién viaja un cliente, y traer a esas personas a un viaje nuevo.
//
// Los pasajeros siguen siendo de cada viaje: traer a alguien es copiarlo con sus
// documentos y permisos. Las copias son independientes, y editar una no toca el
// viaje de donde salió.

// personKey reconoce a la misma persona entre viajes: nombre y apellido sin
// tildes, mayúsculas ni espacios de más. No se usa el documento porque puede
// faltar o cambiar entre un viaje y otro.
func personKey(firstName, lastName string) string {
	return strings.Join(strings.Fields(foldReplacer.Replace(strings.ToLower(firstName+" "+lastName))), " ")
}

// companions lista las personas que viajaron en los viajes del cliente, una vez
// cada una, en la versión del viaje creado más tarde. excludeTripID deja afuera
// los pasajeros de ese viaje.
func companions(ctx context.Context, q querier, customerID, excludeTripID string) ([]store.Companion, error) {
	rows, err := q.Query(ctx, sqlPassengers+`
		 WHERE t.customer_id = $1 AND ($2 = '' OR t.id::text <> $2)
		 ORDER BY t.created_at DESC, t.number DESC, p.is_holder DESC, p.created_at, p.id`,
		customerID, excludeTripID)
	if err != nil {
		return nil, fmt.Errorf("leyendo con quién viaja: %w", err)
	}
	passengers, err := pgx.CollectRows(rows, scanPassenger)
	if err != nil {
		return nil, fmt.Errorf("armando con quién viaja: %w", err)
	}

	trips := map[string]struct {
		number      int64
		destination string
	}{}
	if len(passengers) > 0 {
		rows, err := q.Query(ctx,
			`SELECT id::text, number, coalesce(destination, '') FROM trips WHERE customer_id = $1`, customerID)
		if err != nil {
			return nil, fmt.Errorf("leyendo los viajes del cliente: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id, destination string
			var number int64
			if err := rows.Scan(&id, &number, &destination); err != nil {
				return nil, fmt.Errorf("armando los viajes del cliente: %w", err)
			}
			trips[id] = struct {
				number      int64
				destination string
			}{number, destination}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	var latest []store.Passenger
	index := map[string]int{}
	tripsOf := map[string]map[string]bool{}
	for _, p := range passengers {
		key := personKey(p.FirstName, p.LastName)
		if tripsOf[key] == nil {
			tripsOf[key] = map[string]bool{}
		}
		tripsOf[key][p.TripID] = true
		if _, seen := index[key]; !seen {
			index[key] = len(latest)
			latest = append(latest, p)
		}
	}
	if err := attachDocuments(ctx, q, latest); err != nil {
		return nil, err
	}

	out := make([]store.Companion, 0, len(latest))
	for _, p := range latest {
		trip := trips[p.TripID]
		c := store.Companion{
			PassengerID: p.ID, FirstName: p.FirstName, LastName: p.LastName, Kind: p.Kind,
			BirthDate: p.BirthDate, TripCount: len(tripsOf[personKey(p.FirstName, p.LastName)]),
			TripID: p.TripID, TripNumber: trip.number, Destination: trip.destination,
			PermitCount: len(p.Permits),
		}
		for i := range p.Documents {
			if p.Documents[i].ID == p.TravelDocumentID {
				c.Document = &p.Documents[i]
			}
		}
		if c.Document == nil && len(p.Documents) > 0 {
			c.Document = &p.Documents[len(p.Documents)-1]
		}
		out = append(out, c)
	}
	return out, nil
}

// TripCompanions son las personas que se pueden traer a un viaje: las de los
// otros viajes del mismo cliente. Vienen marcadas las del viaje anterior más
// reciente que tenga pasajeros, y deshabilitadas las que ya están en este.
func (s *Store) TripCompanions(ctx context.Context, tripID string) ([]store.Companion, error) {
	var customerID string
	if err := s.pool.QueryRow(ctx, `SELECT customer_id::text FROM trips WHERE id = $1`, tripID).
		Scan(&customerID); err != nil {
		return nil, writeError("leyendo el viaje", err)
	}
	list, err := companions(ctx, s.pool, customerID, tripID)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return list, nil
	}

	current, err := readPassengers(ctx, s.pool, tripID)
	if err != nil {
		return nil, err
	}
	inTrip := map[string]bool{}
	for _, p := range current {
		inTrip[personKey(p.FirstName, p.LastName)] = true
	}

	// El viaje anterior más reciente con pasajeros es el del primero de la
	// lista: companions ordena por viaje, del más nuevo al más viejo.
	latestTrip := list[0].TripID
	for i := range list {
		list[i].InTrip = inTrip[personKey(list[i].FirstName, list[i].LastName)]
		list[i].Selected = !list[i].InTrip && list[i].TripID == latestTrip
	}
	return list, nil
}

// CopyPassengers trae a un viaje copias de pasajeros de otros viajes del mismo
// cliente, con sus documentos, permisos y el documento con que viajaban. No
// copia las confirmaciones de requisitos: son de un viaje y un itinerario. El
// titular se copia solo si el viaje no tiene uno; si vienen dos, queda el
// primero. Quien ya está en el viaje no se duplica.
func (s *Store) CopyPassengers(ctx context.Context, tripID string, passengerIDs []string, userID string) ([]store.Passenger, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var customerID string
	if err := tx.QueryRow(ctx, `SELECT customer_id::text FROM trips WHERE id = $1 FOR UPDATE`, tripID).
		Scan(&customerID); err != nil {
		return nil, writeError("leyendo el viaje", err)
	}

	current, err := readPassengers(ctx, tx, tripID)
	if err != nil {
		return nil, err
	}
	present := map[string]bool{}
	holderTaken := false
	for _, p := range current {
		present[personKey(p.FirstName, p.LastName)] = true
		holderTaken = holderTaken || p.IsHolder
	}

	created := make([]string, 0, len(passengerIDs))
	for _, sourceID := range passengerIDs {
		var source struct {
			first, last, kind, residence, travelDoc string
			holder, manual                          bool
			sourceTrip, owner                       string
		}
		err := tx.QueryRow(ctx,
			`SELECT p.first_name, p.last_name, p.kind, p.residence_country,
			        coalesce(p.travel_document_id::text, ''), p.is_holder, p.travel_document_manual,
			        p.trip_id::text, t.customer_id::text
			   FROM passengers p JOIN trips t ON t.id = p.trip_id
			  WHERE p.id = $1`, sourceID).Scan(&source.first, &source.last, &source.kind,
			&source.residence, &source.travelDoc, &source.holder, &source.manual, &source.sourceTrip, &source.owner)
		if err != nil {
			return nil, writeError("leyendo el pasajero a traer", err)
		}
		// Solo se trae gente de otro viaje del mismo cliente.
		if source.owner != customerID || source.sourceTrip == tripID {
			return nil, fmt.Errorf("%w: el pasajero no es de otro viaje de este cliente", store.ErrInvalid)
		}
		key := personKey(source.first, source.last)
		if present[key] {
			continue
		}
		present[key] = true
		holder := source.holder && !holderTaken
		holderTaken = holderTaken || holder

		var newID string
		if err := tx.QueryRow(ctx,
			`INSERT INTO passengers (trip_id, first_name, last_name, kind, is_holder, birth_date,
			                         residence_country, created_by)
			 SELECT $1, first_name, last_name, kind, $3, birth_date, residence_country, $4
			   FROM passengers WHERE id = $2
			 RETURNING id`, tripID, sourceID, holder, userID).Scan(&newID); err != nil {
			return nil, writeError("copiando el pasajero", err)
		}

		rows, err := tx.Query(ctx,
			`SELECT id::text FROM passenger_documents WHERE passenger_id = $1 ORDER BY created_at, id`, sourceID)
		if err != nil {
			return nil, fmt.Errorf("leyendo los documentos a copiar: %w", err)
		}
		docIDs, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return nil, fmt.Errorf("armando los documentos a copiar: %w", err)
		}
		travelDoc := ""
		for _, docID := range docIDs {
			var copyID string
			if err := tx.QueryRow(ctx,
				`INSERT INTO passenger_documents (passenger_id, type, country, number, issued_at, expires_at)
				 SELECT $2, type, country, number, issued_at, expires_at
				   FROM passenger_documents WHERE id = $1
				 RETURNING id::text`, docID, newID).Scan(&copyID); err != nil {
				return nil, writeError("copiando un documento", err)
			}
			if docID == source.travelDoc {
				travelDoc = copyID
			}
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO passenger_permits (passenger_id, kind, country, label, valid_from, expires_at)
			 SELECT $2, kind, country, label, valid_from, expires_at
			   FROM passenger_permits WHERE passenger_id = $1 ORDER BY created_at, id`,
			sourceID, newID); err != nil {
			return nil, writeError("copiando los permisos", err)
		}

		if travelDoc != "" {
			if _, err := tx.Exec(ctx,
				`UPDATE passengers SET travel_document_id = $2, travel_document_manual = $3 WHERE id = $1`,
				newID, travelDoc, source.manual); err != nil {
				return nil, writeError("eligiendo el documento de viaje", err)
			}
		}
		created = append(created, newID)
	}

	if err := refreshDerived(ctx, tx, tripID); err != nil {
		return nil, err
	}
	out := make([]store.Passenger, 0, len(created))
	for _, id := range created {
		p, err := readPassenger(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}
