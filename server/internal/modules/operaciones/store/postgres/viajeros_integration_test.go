package postgres

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/db"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/db/dbtest"
	kernelmigrations "github.com/juanseaven/viajes-crm/server/internal/kernel/migrations"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// Los datos que necesita el motor de requisitos: con qué documento viaja cada
// pasajero, qué visas tiene, cuándo nació y por qué países pasa el viaje.

// hasta deja ver solo las migraciones anteriores a `limite`, para sembrar datos
// con el esquema viejo y comprobar qué hace la migración nueva con ellos.
type hasta struct {
	fs.FS
	limite string
}

func (h hasta) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(h.FS, name)
	if err != nil {
		return nil, err
	}
	out := entries[:0]
	for _, e := range entries {
		if e.Name() < h.limite {
			out = append(out, e)
		}
	}
	return out, nil
}

// Los documentos que la agencia ya cargó no se pierden: pasan a la tabla nueva
// como argentinos —es lo único que podían ser— y quedan como el documento con
// el que viaja su pasajero.
func TestLaMigracionPasaLosDocumentosCargadosComoArgentinos(t *testing.T) {
	ctx, pool := dbtest.Pool(t)
	if err := db.Migrate(ctx, pool, "kernel", kernelmigrations.FS(), quietLogger()); err != nil {
		t.Fatalf("migraciones del kernel: %v", err)
	}
	if err := db.Migrate(ctx, pool, "mirror", os.DirFS("../../../mirror/migrations"), quietLogger()); err != nil {
		t.Fatalf("migraciones del espejo: %v", err)
	}
	propias := os.DirFS("../../migrations")
	if err := db.Migrate(ctx, pool, domain.ModuleName, hasta{propias, "0005"}, quietLogger()); err != nil {
		t.Fatalf("migraciones viejas de operaciones: %v", err)
	}

	// Con el esquema viejo se siembra a mano: el código ya habla el nuevo.
	var tripID, conDoc, sinDoc string
	if err := pool.QueryRow(ctx,
		`WITH c AS (INSERT INTO customers (name) VALUES ('Ana Pérez') RETURNING id)
		 INSERT INTO trips (customer_id, destination) SELECT id, 'Río' FROM c RETURNING id`).Scan(&tripID); err != nil {
		t.Fatalf("viaje: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO passengers (trip_id, first_name, last_name, document_type, document_number, document_expires_at)
		 VALUES ($1, 'Ana', 'Pérez', 'Pasaporte', 'AAB123456', '2030-01-15') RETURNING id`, tripID).Scan(&conDoc); err != nil {
		t.Fatalf("pasajero con documento: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO passengers (trip_id, first_name, last_name) VALUES ($1, 'Tomás', 'Pérez') RETURNING id`,
		tripID).Scan(&sinDoc); err != nil {
		t.Fatalf("pasajero sin documento: %v", err)
	}

	if err := db.Migrate(ctx, pool, domain.ModuleName, propias, quietLogger()); err != nil {
		t.Fatalf("migración nueva: %v", err)
	}

	detail, err := New(pool).TripDetail(ctx, tripID)
	if err != nil {
		t.Fatalf("leyendo el viaje: %v", err)
	}
	for _, p := range detail.Passengers {
		switch p.ID {
		case conDoc:
			if len(p.Documents) != 1 {
				t.Fatalf("documentos de Ana = %d, se esperaba 1", len(p.Documents))
			}
			d := p.Documents[0]
			if d.Type != "Pasaporte" || d.Country != "AR" || d.Number != "AAB123456" ||
				d.ExpiresAt == nil || !d.ExpiresAt.Equal(fecha("2030-01-15")) {
				t.Errorf("documento migrado = %+v", d)
			}
			if p.TravelDocumentID != d.ID {
				t.Errorf("documento de viaje = %q, se esperaba el migrado %q", p.TravelDocumentID, d.ID)
			}
		case sinDoc:
			if len(p.Documents) != 0 || p.TravelDocumentID != "" {
				t.Errorf("Tomás no tenía documento y quedó con %+v", p)
			}
		}
	}

	var columnas int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.columns
		  WHERE table_name = 'passengers' AND column_name LIKE 'document\_%'`).Scan(&columnas); err != nil {
		t.Fatalf("leyendo columnas: %v", err)
	}
	if columnas != 0 {
		t.Errorf("quedan %d columnas viejas de documento en passengers", columnas)
	}
}

// Un pasajero con doble nacionalidad tiene dos pasaportes y viaja con uno. El
// primero que se carga es el de viaje; cargar otro no lo desplaza, elegirlo sí.
func TestElPasajeroTieneVariosDocumentosYViajaConUno(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	p, err := s.AddPassenger(ctx, store.NewPassenger{TripID: f.trip, FirstName: "Lucía", LastName: "Rossi",
		Kind: "adulto", CreatedBy: f.user})
	if err != nil {
		t.Fatalf("pasajero: %v", err)
	}
	argentino, err := s.AddDocument(ctx, store.NewDocument{PassengerID: p.ID, Type: "Pasaporte",
		Country: "AR", Number: "AAA111", ExpiresAt: ptrTime(fecha("2031-03-01"))})
	if err != nil {
		t.Fatalf("pasaporte argentino: %v", err)
	}
	italiano, err := s.AddDocument(ctx, store.NewDocument{PassengerID: p.ID, Type: "Pasaporte",
		Country: "IT", Number: "YA222", IssuedAt: ptrTime(fecha("2020-05-01")), ExpiresAt: ptrTime(fecha("2030-05-01"))})
	if err != nil {
		t.Fatalf("pasaporte italiano: %v", err)
	}

	got := pasajero(t, s, f.trip, p.ID)
	if len(got.Documents) != 2 {
		t.Fatalf("documentos = %d, se esperaban 2", len(got.Documents))
	}
	if got.TravelDocumentID != argentino.ID {
		t.Fatalf("documento de viaje = %q, se esperaba el primero cargado", got.TravelDocumentID)
	}

	if _, err := s.UpdatePassenger(ctx, p.ID, store.PassengerPatch{TravelDocumentID: &italiano.ID}); err != nil {
		t.Fatalf("eligiendo el italiano: %v", err)
	}
	if got := pasajero(t, s, f.trip, p.ID); got.TravelDocumentID != italiano.ID {
		t.Fatalf("documento de viaje = %q, se esperaba el italiano", got.TravelDocumentID)
	}

	// Borrar el documento de viaje no deja al pasajero sin documento si tiene
	// otro: pasa a viajar con ese.
	if _, err := s.DeleteDocument(ctx, italiano.ID); err != nil {
		t.Fatalf("borrando el italiano: %v", err)
	}
	if got := pasajero(t, s, f.trip, p.ID); got.TravelDocumentID != argentino.ID {
		t.Fatalf("documento de viaje después de borrar = %q, se esperaba el argentino", got.TravelDocumentID)
	}
}

// El documento de viaje tiene que ser del pasajero. Viajar con el pasaporte de
// otro no es un dato: es un error de carga.
func TestElDocumentoDeViajeEsDelPasajero(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	ana, err := s.AddPassenger(ctx, store.NewPassenger{TripID: f.trip, FirstName: "Ana", LastName: "Pérez",
		Kind: "adulto", DocumentType: "DNI", DocumentNumber: "30111222", CreatedBy: f.user})
	if err != nil {
		t.Fatalf("Ana: %v", err)
	}
	tomas, err := s.AddPassenger(ctx, store.NewPassenger{TripID: f.trip, FirstName: "Tomás", LastName: "Pérez",
		Kind: "menor", CreatedBy: f.user})
	if err != nil {
		t.Fatalf("Tomás: %v", err)
	}
	ajeno := ana.TravelDocumentID
	if ajeno == "" {
		t.Fatal("el alta con DNI debería dejar ese DNI como documento de viaje")
	}
	_, err = s.UpdatePassenger(ctx, tomas.ID, store.PassengerPatch{TravelDocumentID: &ajeno})
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("error = %v, se esperaba ErrConflict", err)
	}
}

// El alta con documento lo carga como argentino salvo que diga otra cosa, y lo
// deja como documento de viaje: es lo que hacía antes el formulario.
func TestElAltaConDocumentoLoDejaComoDocumentoDeViaje(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	p, err := s.AddPassenger(ctx, store.NewPassenger{TripID: f.trip, FirstName: "Ana", LastName: "Pérez",
		Kind: "adulto", DocumentType: "Pasaporte", DocumentNumber: "AAB1",
		DocumentExpiresAt: ptrTime(fecha("2030-01-01")), CreatedBy: f.user})
	if err != nil {
		t.Fatalf("alta: %v", err)
	}
	if len(p.Documents) != 1 || p.Documents[0].Country != "AR" || p.TravelDocumentID != p.Documents[0].ID {
		t.Fatalf("pasajero = %+v", p)
	}
}

// Las visas y autorizaciones que ya tiene son las que resuelven exenciones como
// «con visa de EE.UU. vigente alcanza la eTA».
func TestElPasajeroGuardaSusVisasYAutorizaciones(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	p, err := s.AddPassenger(ctx, store.NewPassenger{TripID: f.trip, FirstName: "Ana", LastName: "Pérez",
		Kind: "adulto", CreatedBy: f.user})
	if err != nil {
		t.Fatalf("pasajero: %v", err)
	}
	visa, err := s.AddPermit(ctx, store.NewPermit{PassengerID: p.ID, Kind: "visa", Country: "US",
		Label: "B1/B2", ValidFrom: ptrTime(fecha("2022-01-10")), ExpiresAt: ptrTime(fecha("2032-01-10"))})
	if err != nil {
		t.Fatalf("visa: %v", err)
	}
	if _, err := s.AddPermit(ctx, store.NewPermit{PassengerID: p.ID, Kind: "pase-libre", Country: "US"}); err == nil {
		t.Error("un tipo de permiso desconocido debería rechazarse")
	}

	nuevo := "2033-01-10"
	if _, err := s.UpdatePermit(ctx, visa.ID, store.PermitPatch{ExpiresAt: &nuevo}); err != nil {
		t.Fatalf("editando la visa: %v", err)
	}
	got := pasajero(t, s, f.trip, p.ID)
	if len(got.Permits) != 1 || got.Permits[0].Label != "B1/B2" ||
		got.Permits[0].ExpiresAt == nil || !got.Permits[0].ExpiresAt.Equal(fecha(nuevo)) {
		t.Fatalf("permisos = %+v", got.Permits)
	}

	if _, err := s.DeletePermit(ctx, visa.ID); err != nil {
		t.Fatalf("borrando la visa: %v", err)
	}
	if got := pasajero(t, s, f.trip, p.ID); len(got.Permits) != 0 {
		t.Fatalf("permisos después de borrar = %+v", got.Permits)
	}
}

// Con fecha de nacimiento, si es menor lo dice la fecha y no una elección: se
// mide a la fecha de salida, porque es en la salida donde se pide la
// autorización. Quien cumple 18 durante el viaje sale siendo menor.
func TestConFechaDeNacimientoSiEsMenorLoDiceLaFecha(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s) // sale el 2026-12-10

	casos := []struct {
		nombre, nacio, kindCargado, want string
	}{
		{"Cumple 18 en el viaje", "2008-12-15", "adulto", "menor"},
		{"Cumplió 18 antes", "2008-12-01", "menor", "adulto"},
		{"Cumple 18 el día de salida", "2008-12-10", "menor", "adulto"},
	}
	for _, c := range casos {
		birth := c.nacio
		p, err := s.AddPassenger(ctx, store.NewPassenger{TripID: f.trip, FirstName: c.nombre, LastName: "X",
			Kind: c.kindCargado, CreatedBy: f.user})
		if err != nil {
			t.Fatalf("%s: %v", c.nombre, err)
		}
		updated, err := s.UpdatePassenger(ctx, p.ID, store.PassengerPatch{BirthDate: &birth})
		if err != nil {
			t.Fatalf("%s: %v", c.nombre, err)
		}
		if updated.Kind != c.want {
			t.Errorf("%s: kind = %q, se esperaba %q", c.nombre, updated.Kind, c.want)
		}
		if updated.BirthDate == nil || !updated.BirthDate.Equal(fecha(c.nacio)) {
			t.Errorf("%s: fecha de nacimiento = %v", c.nombre, updated.BirthDate)
		}
	}
}

// La residencia es la Argentina salvo que se diga otra cosa.
func TestLaResidenciaEsArgentinaPorDefecto(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	p, err := s.AddPassenger(ctx, store.NewPassenger{TripID: f.trip, FirstName: "Ana", LastName: "Pérez",
		Kind: "adulto", CreatedBy: f.user})
	if err != nil {
		t.Fatalf("pasajero: %v", err)
	}
	if p.ResidenceCountry != "AR" {
		t.Fatalf("residencia = %q, se esperaba AR", p.ResidenceCountry)
	}
	es := "ES"
	if got, err := s.UpdatePassenger(ctx, p.ID, store.PassengerPatch{ResidenceCountry: &es}); err != nil || got.ResidenceCountry != "ES" {
		t.Fatalf("residencia = %q, %v", got.ResidenceCountry, err)
	}
}

// El itinerario es la lista de países, cada uno destino o escala. Se reemplaza
// entero: es una lista corta que se edita como una sola cosa.
func TestElItinerarioSeGuardaEnOrdenYSeReemplazaEntero(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	err := s.PutItinerary(ctx, f.trip, store.Itinerary{Stops: []store.Stop{
		{Country: "ES", Role: "destino", ArriveAt: ptrTime(fecha("2026-12-11")), LeaveAt: ptrTime(fecha("2026-12-16"))},
		{Country: "FR", Role: "destino"},
		{Country: "PA", Role: "escala"},
	}})
	if err != nil {
		t.Fatalf("itinerario: %v", err)
	}
	detail, err := s.TripDetail(ctx, f.trip)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	var paises []string
	for _, stop := range detail.Itinerary {
		paises = append(paises, stop.Country+":"+stop.Role)
	}
	if got := strings.Join(paises, ","); got != "ES:destino,FR:destino,PA:escala" {
		t.Fatalf("itinerario = %s", got)
	}
	if detail.Itinerary[0].ArriveAt == nil || !detail.Itinerary[0].ArriveAt.Equal(fecha("2026-12-11")) {
		t.Errorf("llegada a España = %v", detail.Itinerary[0].ArriveAt)
	}

	if detail.Destination != "España y Francia" {
		t.Errorf("nombre = %q, se esperaba que salga de los destinos y no de la escala", detail.Destination)
	}

	if err := s.PutItinerary(ctx, f.trip, store.Itinerary{Stops: []store.Stop{{Country: "BR", Role: "destino"}}}); err != nil {
		t.Fatalf("reemplazando: %v", err)
	}
	if detail, _ := s.TripDetail(ctx, f.trip); len(detail.Itinerary) != 1 || detail.Itinerary[0].Country != "BR" {
		t.Fatalf("itinerario reemplazado = %+v", detail.Itinerary)
	}
}

func TestElItinerarioRechazaDatosInvalidos(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	casos := map[string][]store.Stop{
		"país que no es un código": {{Country: "España", Role: "destino"}},
		"rol desconocido":          {{Country: "ES", Role: "crucero"}},
		"sale antes de llegar":     {{Country: "ES", Role: "destino", ArriveAt: ptrTime(fecha("2026-12-15")), LeaveAt: ptrTime(fecha("2026-12-11"))}},
		"el mismo país dos veces":  {{Country: "ES", Role: "destino"}, {Country: "ES", Role: "escala"}},
	}
	for nombre, stops := range casos {
		if err := s.PutItinerary(ctx, f.trip, store.Itinerary{Stops: stops}); !errors.Is(err, store.ErrInvalid) {
			t.Errorf("%s: error = %v, se esperaba ErrInvalid", nombre, err)
		}
	}
}

// El origen no se evalúa como entrada, así que no puede ser también un país del
// itinerario: ni cargando el país, ni cambiando el origen.
func TestElOrigenNoPuedeEstarEnElItinerario(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	err := s.PutItinerary(ctx, f.trip, store.Itinerary{Stops: []store.Stop{{Country: "AR", Role: "destino"}}})
	if !errors.Is(err, store.ErrInvalid) || !strings.Contains(err.Error(), "sale de Argentina") {
		t.Fatalf("destino igual al origen: error = %v", err)
	}
	if err := s.PutItinerary(ctx, f.trip, store.Itinerary{Stops: []store.Stop{{Country: "CL", Role: "destino"}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutItinerary(ctx, f.trip, store.Itinerary{Origin: "CL", Stops: []store.Stop{{Country: "CL", Role: "destino"}}}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("origen igual a un destino: error = %v", err)
	}

	// Cambiar los dos a la vez no choca con el estado intermedio.
	if err := s.PutItinerary(ctx, f.trip, store.Itinerary{Origin: "CL", Stops: []store.Stop{{Country: "AR", Role: "destino"}}}); err != nil {
		t.Fatalf("intercambio: %v", err)
	}
	detail, err := s.TripDetail(ctx, f.trip)
	if err != nil {
		t.Fatal(err)
	}
	if detail.OriginCountry != "CL" || detail.Itinerary[0].Country != "AR" || detail.Destination != "Argentina" {
		t.Fatalf("origen = %s, itinerario = %+v, nombre = %q", detail.OriginCountry, detail.Itinerary, detail.Destination)
	}
}

func TestUnViajeSeCreaConOrigenItinerarioYFechas(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)

	trip, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID: f.contact, Origin: "UY", CreatedBy: f.user,
		Itinerary: []store.Stop{{Country: "PA", Role: "escala"}, {Country: "MX", Role: "destino"}},
		DepartAt:  fecha("2026-12-10"), ReturnAt: fecha("2026-12-20"),
	})
	if err != nil {
		t.Fatalf("creando: %v", err)
	}
	if trip.Destination != "México" {
		t.Errorf("nombre devuelto = %q", trip.Destination)
	}
	detail, err := s.TripDetail(ctx, trip.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.OriginCountry != "UY" || len(detail.Itinerary) != 2 || detail.Destination != "México" || detail.DepartAt == nil {
		t.Fatalf("viaje = origen %s, itinerario %+v, nombre %q, salida %v", detail.OriginCountry, detail.Itinerary,
			detail.Destination, detail.DepartAt)
	}

	// Sin origen, sale de Argentina.
	plain, err := s.CreateTrip(ctx, store.NewTrip{ContactID: f.contact, Destination: "Cancún", CreatedBy: f.user})
	if err != nil {
		t.Fatal(err)
	}
	if detail, _ := s.TripDetail(ctx, plain.ID); detail.OriginCountry != "AR" || detail.Destination != "Cancún" {
		t.Fatalf("sin itinerario: origen %s, nombre %q", detail.OriginCountry, detail.Destination)
	}

	if _, err := s.CreateTrip(ctx, store.NewTrip{ContactID: f.contact, CreatedBy: f.user,
		Itinerary: []store.Stop{{Country: "AR", Role: "destino"}}}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("destino igual al origen por defecto: error = %v", err)
	}
}

// Elegir «viaja con este» lo fija; pedir que elija el sistema lo libera. Borrar
// el documento elegido también lo libera.
func TestElDocumentoElegidoAManoSeFijaYSeLibera(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)
	p, err := s.AddPassenger(ctx, store.NewPassenger{TripID: f.trip, FirstName: "Ana", LastName: "Pérez", Kind: "adulto",
		DocumentType: "DNI", DocumentCountry: "AR", CreatedBy: f.user})
	if err != nil {
		t.Fatal(err)
	}
	pas, err := s.AddDocument(ctx, store.NewDocument{PassengerID: p.ID, Type: "Pasaporte", Country: "AR"})
	if err != nil {
		t.Fatal(err)
	}
	if got := pasajero(t, s, f.trip, p.ID); got.TravelDocumentManual {
		t.Fatalf("recién cargado no es una elección a mano: %+v", got)
	}

	pasID := pas.ID
	if _, err := s.UpdatePassenger(ctx, p.ID, store.PassengerPatch{TravelDocumentID: &pasID}); err != nil {
		t.Fatal(err)
	}
	if got := pasajero(t, s, f.trip, p.ID); !got.TravelDocumentManual || got.TravelDocumentID != pas.ID {
		t.Fatalf("elegido a mano: %+v", got)
	}

	// La evaluación no toca un documento elegido a mano.
	dniID := got0(t, s, f.trip, p.ID)
	if err := s.SaveRequirements(ctx, f.trip, nil, map[string]string{p.ID: dniID}); err != nil {
		t.Fatal(err)
	}
	if got := pasajero(t, s, f.trip, p.ID); got.TravelDocumentID != pas.ID {
		t.Fatalf("la evaluación cambió el elegido a mano: %+v", got)
	}

	sistema := ""
	if _, err := s.UpdatePassenger(ctx, p.ID, store.PassengerPatch{TravelDocumentID: &sistema}); err != nil {
		t.Fatal(err)
	}
	if got := pasajero(t, s, f.trip, p.ID); got.TravelDocumentManual || got.TravelDocumentID != pas.ID {
		t.Fatalf("devuelto al sistema conserva el documento hasta la próxima evaluación: %+v", got)
	}
	if err := s.SaveRequirements(ctx, f.trip, nil, map[string]string{p.ID: dniID}); err != nil {
		t.Fatal(err)
	}
	if got := pasajero(t, s, f.trip, p.ID); got.TravelDocumentID != dniID {
		t.Fatalf("la evaluación no eligió: %+v", got)
	}

	if _, err := s.UpdatePassenger(ctx, p.ID, store.PassengerPatch{TravelDocumentID: &dniID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteDocument(ctx, dniID); err != nil {
		t.Fatal(err)
	}
	if got := pasajero(t, s, f.trip, p.ID); got.TravelDocumentManual || got.TravelDocumentID != pas.ID {
		t.Fatalf("borrado el elegido a mano: %+v", got)
	}
}

// got0 es el primer documento cargado del pasajero.
func got0(t *testing.T, s *Store, tripID, passengerID string) string {
	t.Helper()
	return pasajero(t, s, tripID, passengerID).Documents[0].ID
}

func pasajero(t *testing.T, s *Store, tripID, passengerID string) store.Passenger {
	t.Helper()
	detail, err := s.TripDetail(t.Context(), tripID)
	if err != nil {
		t.Fatalf("leyendo el viaje: %v", err)
	}
	for _, p := range detail.Passengers {
		if p.ID == passengerID {
			return p
		}
	}
	t.Fatalf("el pasajero %s no está en el viaje", passengerID)
	return store.Passenger{}
}
