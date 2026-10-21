package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/evaluacion"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
	reglas "github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

// Los requisitos evaluados se guardan para que los controles se cierren con una
// consulta, como el resto de los derivados.

func aviso(t *testing.T, ctx context.Context, p *pgxpool.Pool, f fixture, n string) string {
	t.Helper()
	var id string
	if err := p.QueryRow(ctx,
		`INSERT INTO messages (conversation_id, instance_id, contact_id, wa_message_id,
		                       direction, origin, type, body, wa_timestamp, sent_by_user_id)
		 VALUES ($1, $2, $3, $4, 'outbound', 'panel', 'text', 'aviso', now(), $5) RETURNING id`,
		f.conversation, f.instance, f.contact, "aviso-"+n, f.user).Scan(&id); err != nil {
		t.Fatalf("sembrando el aviso: %v", err)
	}
	return id
}

func pax(t *testing.T, s *Store, f fixture, name string, expires string) store.Passenger {
	t.Helper()
	p, err := s.AddPassenger(t.Context(), store.NewPassenger{TripID: f.trip, FirstName: name, LastName: "Pérez",
		Kind: "adulto", DocumentType: "Pasaporte", DocumentExpiresAt: ptrTime(fecha(expires)), CreatedBy: f.user})
	if err != nil {
		t.Fatalf("pasajero %s: %v", name, err)
	}
	return p
}

func req(paxID, country, key string, status evaluacion.Status) evaluacion.Requirement {
	return evaluacion.Requirement{PassengerID: paxID, Country: country, Key: key, Status: status,
		Text: key + " " + string(status), Confirmable: status != evaluacion.Cumple, FieldHash: "h-" + key}
}

func TestRequisitosMigratoriosSigueALosRequisitos(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)
	ana := pax(t, s, f, "Ana", "2030-01-01")

	if got := estado(t, ctx, pool, f.trip, "control-visa"); got != "abierto" {
		t.Fatalf("sin evaluar: %s", got)
	}

	todoBien := []evaluacion.Requirement{
		req(ana.ID, "US", evaluacion.KeyVisa, evaluacion.Cumple),
		req(ana.ID, "US", evaluacion.KeyDocumento, evaluacion.Cumple),
		// La vigencia la gobierna el otro control: no frena a este.
		req(ana.ID, "US", evaluacion.KeyVigencia, evaluacion.Verificar),
	}
	if err := s.SaveRequirements(ctx, f.trip, todoBien, nil); err != nil {
		t.Fatalf("guardando: %v", err)
	}
	if got := estado(t, ctx, pool, f.trip, "control-visa"); got != "cerrado" {
		t.Fatalf("todo cumple: %s, se esperaba cerrado", got)
	}

	falta := append([]evaluacion.Requirement{}, todoBien...)
	falta[0].Status = evaluacion.Falta
	if err := s.SaveRequirements(ctx, f.trip, falta, nil); err != nil {
		t.Fatalf("guardando: %v", err)
	}
	if got := estado(t, ctx, pool, f.trip, "control-visa"); got != "abierto" {
		t.Fatalf("falta la visa: %s, se esperaba abierto", got)
	}

	detail, err := s.TripDetail(ctx, f.trip)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	if len(detail.Requirements) != 3 || detail.Requirements[0].Key != evaluacion.KeyVisa ||
		detail.Requirements[0].Status != evaluacion.Falta {
		t.Fatalf("requisitos = %+v", detail.Requirements)
	}
}

// El aviso prueba que se le avisó al cliente, no que la agencia verificó: con
// algo en verificar no cierra. Y prueba lo que se avisó: un requisito nuevo lo
// reabre.
func TestElAvisoCierraLoAvisadoYUnRequisitoNuevoLoReabre(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)
	ana := pax(t, s, f, "Ana", "2030-01-01")
	visa := control(t, ctx, pool, f.trip, "control-visa")

	conDudas := []evaluacion.Requirement{
		req(ana.ID, "US", evaluacion.KeyVisa, evaluacion.Falta),
		req(ana.ID, "US", "vacuna:fiebre amarilla", evaluacion.Verificar),
	}
	if err := s.SaveRequirements(ctx, f.trip, conDudas, nil); err != nil {
		t.Fatal(err)
	}
	err := s.CloseControl(ctx, store.CloseControl{ControlID: visa, UserID: f.user, MessageID: aviso(t, ctx, pool, f, "1")})
	if !errors.Is(err, store.ErrQuedaPorVerificar) {
		t.Fatalf("error = %v, se esperaba ErrQuedaPorVerificar", err)
	}

	soloFalta := conDudas[:1]
	if err := s.SaveRequirements(ctx, f.trip, soloFalta, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseControl(ctx, store.CloseControl{ControlID: visa, UserID: f.user,
		MessageID: aviso(t, ctx, pool, f, "2")}); err != nil {
		t.Fatalf("cerrando con el aviso: %v", err)
	}
	if got := estado(t, ctx, pool, f.trip, "control-visa"); got != "cerrado" {
		t.Fatalf("avisado: %s", got)
	}

	// Lo mismo que se avisó no lo reabre.
	if err := s.SaveRequirements(ctx, f.trip, soloFalta, nil); err != nil {
		t.Fatal(err)
	}
	if got := estado(t, ctx, pool, f.trip, "control-visa"); got != "cerrado" {
		t.Fatalf("misma falta que se avisó: %s, debería seguir cerrado", got)
	}

	// Un requisito nuevo, sí.
	nuevo := append(append([]evaluacion.Requirement{}, soloFalta...), req(ana.ID, "MX", evaluacion.KeyVisa, evaluacion.Falta))
	if err := s.SaveRequirements(ctx, f.trip, nuevo, nil); err != nil {
		t.Fatal(err)
	}
	if got := estado(t, ctx, pool, f.trip, "control-visa"); got != "abierto" {
		t.Fatalf("falta nueva después del aviso: %s, debería reabrirse", got)
	}
}

// El control del documento mira la vigencia del destino donde la hay, y la del
// regreso donde no.
func TestElControlDelDocumentoMiraLaVigenciaDelDestino(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s) // vuelve el 2026-12-24
	ana := pax(t, s, f, "Ana", "2027-02-20")

	if got := estado(t, ctx, pool, f.trip, "control-vto-doc"); got != "cerrado" {
		t.Fatalf("vence después del regreso y sin requisitos: %s", got)
	}

	schengen := []evaluacion.Requirement{req(ana.ID, "ES", evaluacion.KeyVigencia, evaluacion.Falta)}
	if err := s.SaveRequirements(ctx, f.trip, schengen, nil); err != nil {
		t.Fatal(err)
	}
	if got := estado(t, ctx, pool, f.trip, "control-vto-doc"); got != "abierto" {
		t.Fatalf("Schengen pide más vigencia: %s, se esperaba abierto", got)
	}

	desconocida := []evaluacion.Requirement{req(ana.ID, "BR", evaluacion.KeyVigencia, evaluacion.Verificar)}
	if err := s.SaveRequirements(ctx, f.trip, desconocida, nil); err != nil {
		t.Fatal(err)
	}
	if got := estado(t, ctx, pool, f.trip, "control-vto-doc"); got != "cerrado" {
		t.Fatalf("vigencia desconocida usa el criterio del regreso: %s, se esperaba cerrado", got)
	}
}

func TestConfirmarUnRequisitoQuedaAtadoAlValorDeLaRegla(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)
	ana := pax(t, s, f, "Ana", "2030-01-01")

	reqs := []evaluacion.Requirement{
		req(ana.ID, "JP", evaluacion.KeyVisa, evaluacion.Verificar),
		{PassengerID: ana.ID, Country: "JP", Key: evaluacion.KeyVigencia, Status: evaluacion.Falta, MissingData: true, Text: "falta"},
	}
	if err := s.SaveRequirements(ctx, f.trip, reqs, nil); err != nil {
		t.Fatal(err)
	}

	err := s.Confirm(ctx, store.NewConfirmation{TripID: f.trip, PassengerID: ana.ID, Country: "JP",
		Key: evaluacion.KeyVigencia, UserID: f.user})
	if !errors.Is(err, store.ErrNoConfirmable) {
		t.Fatalf("un dato que falta no se confirma: %v", err)
	}

	if err := s.Confirm(ctx, store.NewConfirmation{TripID: f.trip, PassengerID: ana.ID, Country: "JP",
		Key: evaluacion.KeyVisa, UserID: f.user}); err != nil {
		t.Fatalf("confirmando: %v", err)
	}
	in, err := s.EvaluationInput(ctx, f.trip)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	if len(in.Confirmations) != 1 || in.Confirmations[0].FieldHash != "h-visa" || in.Confirmations[0].Key != evaluacion.KeyVisa {
		t.Fatalf("confirmaciones = %+v", in.Confirmations)
	}
	if len(in.Trip.Passengers) != 1 || in.Trip.Passengers[0].Doc == nil || in.Trip.Passengers[0].Doc.Type != "Pasaporte" {
		t.Fatalf("pasajeros = %+v", in.Trip.Passengers)
	}

	// Al re-evaluar, la confirmación se ve con quién la hizo.
	reqs[0].Status, reqs[0].Confirmed = evaluacion.Cumple, true
	if err := s.SaveRequirements(ctx, f.trip, reqs, nil); err != nil {
		t.Fatal(err)
	}
	detail, _ := s.TripDetail(ctx, f.trip)
	if detail.Requirements[0].ConfirmedByName != "Vendedora" || detail.Requirements[0].ConfirmedAt == nil {
		t.Fatalf("requisito confirmado = %+v", detail.Requirements[0])
	}
}

func TestLosViajesAbiertosQueUsanUnaRegla(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)
	pax(t, s, f, "Ana", "2030-01-01")
	if err := s.PutItinerary(ctx, f.trip, store.Itinerary{Stops: []store.Stop{{Country: "US", Role: "destino"}}}); err != nil {
		t.Fatal(err)
	}

	ids, err := s.OpenTripsUsing(ctx, []reglas.Pair{{Nationality: "AR", Country: "US"}})
	if err != nil || len(ids) != 1 || ids[0] != f.trip {
		t.Fatalf("viajes = %v, %v", ids, err)
	}
	if ids, _ := s.OpenTripsUsing(ctx, []reglas.Pair{{Nationality: "IT", Country: "US"}}); len(ids) != 0 {
		t.Fatalf("ningún pasajero viaja con pasaporte italiano: %v", ids)
	}

	cerrado := domain.TripCerrado
	if _, err := s.UpdateTrip(ctx, f.trip, store.TripPatch{Status: &cerrado}); err != nil {
		t.Fatal(err)
	}
	if ids, _ := s.OpenTripsUsing(ctx, []reglas.Pair{{Nationality: "AR", Country: "US"}}); len(ids) != 0 {
		t.Fatalf("un viaje cerrado no se re-evalúa: %v", ids)
	}
	if ids, err := s.OpenTrips(ctx); err != nil || len(ids) != 0 {
		t.Fatalf("abiertos = %v, %v", ids, err)
	}
}

// Un viaje sin pasajeros muestra lo que se le pide a un pasaporte argentino:
// cuando esa regla cambia, también hay que avisarle.
func TestUnViajeSinPasajerosUsaLaReglaDelPasaporteArgentino(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)
	if err := s.PutItinerary(ctx, f.trip, store.Itinerary{Stops: []store.Stop{{Country: "JP", Role: "destino"}}}); err != nil {
		t.Fatal(err)
	}
	if ids, err := s.OpenTripsUsing(ctx, []reglas.Pair{{Nationality: "AR", Country: "JP"}}); err != nil || len(ids) != 1 {
		t.Fatalf("viajes = %v, %v", ids, err)
	}
	if ids, _ := s.OpenTripsUsing(ctx, []reglas.Pair{{Nationality: "IT", Country: "JP"}}); len(ids) != 0 {
		t.Fatalf("la vista previa es solo del pasaporte argentino: %v", ids)
	}

	// Con un pasajero italiano, deja de mostrar la vista previa.
	if _, err := s.AddPassenger(ctx, store.NewPassenger{TripID: f.trip, FirstName: "Luca", LastName: "Rossi", Kind: "adulto",
		DocumentType: "Pasaporte", DocumentCountry: "IT", CreatedBy: f.user}); err != nil {
		t.Fatal(err)
	}
	if ids, _ := s.OpenTripsUsing(ctx, []reglas.Pair{{Nationality: "AR", Country: "JP"}}); len(ids) != 0 {
		t.Fatalf("con pasajeros, la regla argentina ya no la usa: %v", ids)
	}
	if ids, _ := s.OpenTripsUsing(ctx, []reglas.Pair{{Nationality: "IT", Country: "JP"}}); len(ids) != 1 {
		t.Fatalf("la regla italiana sí: %v", ids)
	}
}

// Una recomendación no es un faltante: no abre «Requisitos migratorios», y el
// detalle de cada renglón vuelve como se guardó.
func TestUnAvisoNoPesaYElDetalleSeGuarda(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	f := escenario(t, ctx, pool, s)
	ana := pax(t, s, f, "Ana", "2030-01-01")

	visa := req(ana.ID, "US", evaluacion.KeyVisa, evaluacion.Cumple)
	visa.Detail = evaluacion.Detail{
		Pide:  []evaluacion.Line{{Text: "Estados Unidos pide visa consular."}},
		Tiene: []string{"Tiene visa de Estados Unidos."},
		Hacer: []string{"Nada."},
	}
	visa.Searching = true
	renovar := evaluacion.Requirement{PassengerID: ana.ID, Key: evaluacion.KeyRenovar, Status: evaluacion.Aviso,
		Text: "Renovación del pasaporte", ClientText: "renovar el pasaporte de Ana"}
	if err := s.SaveRequirements(ctx, f.trip, []evaluacion.Requirement{renovar, visa}, nil); err != nil {
		t.Fatalf("guardando: %v", err)
	}
	if got := estado(t, ctx, pool, f.trip, "control-visa"); got != "cerrado" {
		t.Fatalf("con una recomendación: %s, se esperaba cerrado", got)
	}

	detail, err := s.TripDetail(ctx, f.trip)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	got := detail.Requirements[1]
	if len(got.Detail.Pide) != 1 || got.Detail.Tiene[0] != "Tiene visa de Estados Unidos." || !got.Searching {
		t.Fatalf("detalle leído = %+v", got)
	}
	if detail.Requirements[0].Status != evaluacion.Aviso {
		t.Fatalf("aviso leído = %+v", detail.Requirements[0])
	}

	// El aviso al cliente cubre lo que faltaba; la recomendación, que no es un
	// faltante, no lo reabre.
	falta := req(ana.ID, "US", evaluacion.KeyVisa, evaluacion.Falta)
	if err := s.SaveRequirements(ctx, f.trip, []evaluacion.Requirement{falta}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseControl(ctx, store.CloseControl{ControlID: control(t, ctx, pool, f.trip, "control-visa"),
		UserID: f.user, MessageID: aviso(t, ctx, pool, f, "1")}); err != nil {
		t.Fatalf("cerrando con el aviso: %v", err)
	}
	if err := s.SaveRequirements(ctx, f.trip, []evaluacion.Requirement{renovar, falta}, nil); err != nil {
		t.Fatal(err)
	}
	if got := estado(t, ctx, pool, f.trip, "control-visa"); got != "cerrado" {
		t.Fatalf("una recomendación nueva después del aviso: %s, debería seguir cerrado", got)
	}
}
