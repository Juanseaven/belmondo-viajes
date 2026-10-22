package operaciones

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/evaluacion"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
	reglas "github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

type fakeRepo struct {
	created store.NewTrip
	trip    store.Trip
	inbox   []store.InboxItem
	sellers []store.Seller

	pendingDay time.Time
	filter     store.TripFilter

	err            error
	itinerary      []store.Stop
	origin         string
	newDocument    store.NewDocument
	documentPatch  store.DocumentPatch
	newPermit      store.NewPermit
	passengerPatch store.PassengerPatch

	newCustomer     store.NewCustomer
	customerFilter  store.CustomerFilter
	copied          []string
	contactCustomer string
}

func (f *fakeRepo) CreateTrip(_ context.Context, n store.NewTrip) (store.Trip, error) {
	f.created = n
	return f.trip, nil
}
func (f *fakeRepo) UpdateTrip(context.Context, string, store.TripPatch) (store.Trip, error) {
	return f.trip, nil
}
func (f *fakeRepo) TripsByContact(context.Context, string) ([]store.TripSummary, error) {
	return []store.TripSummary{}, nil
}
func (f *fakeRepo) TripDetail(context.Context, string) (store.TripDetailData, error) {
	return store.TripDetailData{Trip: f.trip}, nil
}
func (f *fakeRepo) AddPassenger(context.Context, store.NewPassenger) (store.Passenger, error) {
	return store.Passenger{}, nil
}
func (f *fakeRepo) UpdatePassenger(_ context.Context, _ string, patch store.PassengerPatch) (store.Passenger, error) {
	f.passengerPatch = patch
	return store.Passenger{}, f.err
}
func (f *fakeRepo) DeletePassenger(context.Context, string) (string, error) { return "trip-1", nil }
func (f *fakeRepo) EvaluationInput(context.Context, string) (store.EvaluationInput, error) {
	return store.EvaluationInput{}, nil
}
func (f *fakeRepo) SaveRequirements(context.Context, string, []evaluacion.Requirement, map[string]string) error {
	return nil
}
func (f *fakeRepo) Confirm(context.Context, store.NewConfirmation) error            { return nil }
func (f *fakeRepo) OpenTripsUsing(context.Context, []reglas.Pair) ([]string, error) { return nil, nil }
func (f *fakeRepo) OpenTrips(context.Context) ([]string, error)                     { return nil, nil }
func (f *fakeRepo) PutPayment(context.Context, store.Payment) (store.Payment, error) {
	return store.Payment{}, nil
}
func (f *fakeRepo) CloseControl(context.Context, store.CloseControl) error { return nil }
func (f *fakeRepo) ReopenControl(context.Context, string) error            { return nil }
func (f *fakeRepo) Customer(context.Context, string) (store.Customer, error) {
	return store.Customer{}, nil
}
func (f *fakeRepo) UpdateCustomer(context.Context, string, store.CustomerPatch) (store.Customer, error) {
	return store.Customer{}, nil
}
func (f *fakeRepo) LinkContact(context.Context, string, string, string) error { return nil }
func (f *fakeRepo) CreateCustomer(_ context.Context, n store.NewCustomer) (store.Customer, error) {
	f.newCustomer = n
	return store.Customer{ID: "cliente-1", Name: n.Name}, f.err
}
func (f *fakeRepo) CustomerDetail(context.Context, string) (store.CustomerDetail, error) {
	return store.CustomerDetail{}, f.err
}
func (f *fakeRepo) ListCustomers(_ context.Context, filter store.CustomerFilter) (store.CustomerPage, error) {
	f.customerFilter = filter
	return store.CustomerPage{Items: []store.CustomerRow{}}, nil
}
func (f *fakeRepo) SimilarCustomers(context.Context, store.SimilarQuery) ([]store.CustomerMatch, error) {
	return []store.CustomerMatch{}, nil
}
func (f *fakeRepo) SuggestedCustomers(context.Context, string) ([]store.CustomerMatch, error) {
	return []store.CustomerMatch{}, nil
}
func (f *fakeRepo) ContactCustomer(context.Context, string) (string, error) {
	return f.contactCustomer, nil
}
func (f *fakeRepo) FreeContacts(context.Context, string, string) ([]store.FreeContact, error) {
	return []store.FreeContact{}, nil
}
func (f *fakeRepo) TripCompanions(context.Context, string) ([]store.Companion, error) {
	return []store.Companion{}, nil
}
func (f *fakeRepo) CopyPassengers(_ context.Context, _ string, ids []string, _ string) ([]store.Passenger, error) {
	f.copied = ids
	return []store.Passenger{}, f.err
}
func (f *fakeRepo) Inbox(context.Context, []string) ([]store.InboxItem, error) {
	return f.inbox, nil
}
func (f *fakeRepo) Sellers(context.Context) ([]store.Seller, error) { return f.sellers, nil }
func (f *fakeRepo) Pending(_ context.Context, today time.Time) ([]store.PendingRow, error) {
	f.pendingDay = today
	return []store.PendingRow{}, nil
}
func (f *fakeRepo) ListTrips(_ context.Context, filter store.TripFilter) (store.TripPage, error) {
	f.filter = filter
	return store.TripPage{Items: []store.TripRow{}}, nil
}

type routeRecorder struct{ patterns []string }

func (r *routeRecorder) Authed(pattern string, _ http.Handler) {
	r.patterns = append(r.patterns, pattern)
}
func (r *routeRecorder) Public(pattern string, _ http.Handler) {
	r.patterns = append(r.patterns, "public "+pattern)
}

var _ modules.Router = (*routeRecorder)(nil)

func testModule(repo repository, events *[]string) *Module {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Module{repo: repo, log: log, clock: time.Now, emit: func(eventType string, _ map[string]any) {
		*events = append(*events, eventType)
	}}
}

func TestModuloMontaTodoElContratoAutenticado(t *testing.T) {
	m := testModule(&fakeRepo{}, &[]string{})
	routes := &routeRecorder{}
	m.Mount(routes)
	want := []string{
		"GET /viajes", "POST /viajes", "GET /viajes/{id}", "PATCH /viajes/{id}",
		"POST /viajes/{id}/pasajeros", "PATCH /pasajeros/{id}", "DELETE /pasajeros/{id}",
		"PUT /viajes/{id}/pago", "POST /controles/{id}/cerrar", "POST /controles/{id}/reabrir",
		"GET /clientes", "POST /clientes", "GET /clientes/parecidos",
		"GET /clientes/{id}", "PATCH /clientes/{id}", "POST /clientes/{id}/contactos", "GET /bandeja",
		"GET /vendedores", "GET /pendientes",
		"PUT /viajes/{id}/itinerario",
		"POST /pasajeros/{id}/documentos", "PATCH /documentos/{id}", "DELETE /documentos/{id}",
		"POST /pasajeros/{id}/permisos", "PATCH /permisos/{id}", "DELETE /permisos/{id}",
		"POST /viajes/{id}/requisitos/confirmar",
		"GET /viajes/{id}/companeros", "POST /viajes/{id}/pasajeros/traer",
		"GET /contactos", "GET /contactos/{id}/clientes",
	}
	if len(routes.patterns) != len(want) {
		t.Fatalf("rutas = %v", routes.patterns)
	}
	for i := range want {
		if routes.patterns[i] != want[i] {
			t.Fatalf("ruta %d = %q, se esperaba %q", i, routes.patterns[i], want[i])
		}
	}
}

func TestAltaHTTPTraduceLasTresEntradasYEmiteDespuesDeGuardar(t *testing.T) {
	repo := &fakeRepo{trip: store.Trip{ID: "trip-1", Number: 10482}}
	var events []string
	m := testModule(repo, &events)
	req := httptest.NewRequest(http.MethodPost, "/viajes", bytes.NewBufferString(
		`{"contactId":"contact-1","destination":"Iguazú","departAt":"2026-09-12","returnAt":"2026-09-15"}`))
	rec := httptest.NewRecorder()

	m.createTrip(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if repo.created.ContactID != "contact-1" || repo.created.Destination != "Iguazú" ||
		repo.created.DepartAt.Format("2006-01-02") != "2026-09-12" {
		t.Fatalf("alta traducida: %#v", repo.created)
	}
	if len(events) != 2 || events[0] != "trip.created" || events[1] != "requirements.changed" {
		t.Fatalf("eventos = %v", events)
	}
}

func TestAltaHTTPPermiteCrearUnBorradorSinDatosDeViaje(t *testing.T) {
	repo := &fakeRepo{trip: store.Trip{ID: "trip-draft", Number: 10483}}
	var events []string
	m := testModule(repo, &events)
	req := httptest.NewRequest(http.MethodPost, "/viajes", bytes.NewBufferString(
		`{"contactId":"contact-1"}`))
	rec := httptest.NewRecorder()

	m.createTrip(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if repo.created.ContactID != "contact-1" || repo.created.Destination != "" ||
		!repo.created.DepartAt.IsZero() || !repo.created.ReturnAt.IsZero() {
		t.Fatalf("borrador traducido: %#v", repo.created)
	}
	if len(events) != 2 || events[0] != "trip.created" || events[1] != "requirements.changed" {
		t.Fatalf("eventos = %v", events)
	}
}

// El borrador manda de dónde sale y a dónde va: el viaje nace con su itinerario.
func TestAltaHTTPCreaElViajeConOrigenEItinerario(t *testing.T) {
	repo := &fakeRepo{trip: store.Trip{ID: "trip-1"}}
	m := testModule(repo, &[]string{})
	req := httptest.NewRequest(http.MethodPost, "/viajes", bytes.NewBufferString(
		`{"contactId":"contact-1","originCountry":"uy","itinerary":[{"country":"pa","role":"escala"},{"country":"MX","role":"destino"}]}`))
	rec := httptest.NewRecorder()

	m.createTrip(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if repo.created.Origin != "UY" || len(repo.created.Itinerary) != 2 || repo.created.Itinerary[0].Country != "PA" ||
		repo.created.Itinerary[1].Role != "destino" {
		t.Fatalf("alta traducida: %#v", repo.created)
	}
}

func TestAltaHTTPRechazaFechasInvalidasAntesDelStore(t *testing.T) {
	repo := &fakeRepo{}
	var events []string
	m := testModule(repo, &events)
	req := httptest.NewRequest(http.MethodPost, "/viajes", bytes.NewBufferString(
		`{"contactId":"contact-1","destination":"Iguazú","departAt":"ayer","returnAt":"2026-09-15"}`))
	rec := httptest.NewRecorder()

	m.createTrip(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !repo.created.DepartAt.IsZero() || len(events) != 0 {
		t.Fatalf("se escribió o emitió con entrada inválida: %#v %v", repo.created, events)
	}
}

func TestVendedoresHTTPDevuelveLaListaParaElSelector(t *testing.T) {
	repo := &fakeRepo{sellers: []store.Seller{{ID: "u-1", Name: "Ana Gil"}}}
	m := testModule(repo, &[]string{})
	rec := httptest.NewRecorder()

	m.sellers(rec, httptest.NewRequest(http.MethodGet, "/vendedores", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body := strings.TrimSpace(rec.Body.String()); body != `{"items":[{"id":"u-1","name":"Ana Gil"}]}` {
		t.Fatalf("body = %q", body)
	}
}

func TestNombreYMigracionesDelModuloSonEstables(t *testing.T) {
	m := testModule(&fakeRepo{}, &[]string{})
	if m.Name() != "operaciones" {
		t.Fatalf("nombre = %q", m.Name())
	}
	entries, err := fs.ReadDir(m.Migrations(), ".")
	if err != nil || len(entries) != 9 || entries[0].Name() != "0001_init.sql" || entries[1].Name() != "0002_trip_drafts.sql" ||
		entries[2].Name() != "0003_documentos_vigentes.sql" || entries[3].Name() != "0004_estado_consulta.sql" ||
		entries[4].Name() != "0005_viajeros.sql" || entries[5].Name() != "0006_requisitos.sql" ||
		entries[6].Name() != "0007_origen.sql" || entries[7].Name() != "0008_documento_automatico.sql" ||
		entries[8].Name() != "0009_detalle_requisitos.sql" {
		t.Fatalf("migraciones = %v, err = %v", entries, err)
	}
}

// «Hoy» es el día en Buenos Aires: a las 23 h de un martes ya es miércoles en
// UTC, y la lista de pendientes no puede adelantarse un día.
func TestPendientesUsaElDiaDeArgentina(t *testing.T) {
	repo := &fakeRepo{}
	m := testModule(repo, &[]string{})
	m.clock = func() time.Time { return time.Date(2026, 10, 21, 2, 0, 0, 0, time.UTC) }
	rec := httptest.NewRecorder()

	m.pending(rec, httptest.NewRequest(http.MethodGet, "/pendientes", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if want := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC); !repo.pendingDay.Equal(want) {
		t.Fatalf("hoy = %v, se esperaba %v", repo.pendingDay, want)
	}
}

func TestListadoHTTPTraduceLosFiltros(t *testing.T) {
	repo := &fakeRepo{}
	m := testModule(repo, &[]string{})
	m.clock = func() time.Time { return time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC) }
	rec := httptest.NewRecorder()

	m.listTrips(rec, httptest.NewRequest(http.MethodGet,
		"/viajes?q=cancun&estado=consulta,en%20viaje&salidaDesde=2026-11-01&salidaHasta=2026-11-30&vendedor=ninguno&atrasados=1&orden=total&cursor=abc", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	f := repo.filter
	if f.Query != "cancun" || len(f.Statuses) != 2 || f.Statuses[0] != "consulta" || f.Statuses[1] != "en viaje" || f.OpenOnly ||
		f.DepartFrom == nil || f.DepartFrom.Format("2006-01-02") != "2026-11-01" ||
		f.DepartTo == nil || !f.SellerNone || !f.LateOnly || f.Sort != store.SortTotal ||
		f.Cursor != "abc" || f.Today.Format("2006-01-02") != "2026-10-20" {
		t.Fatalf("filtro = %#v", f)
	}
}

func TestListadoHTTPPorDefectoMuestraLosAbiertos(t *testing.T) {
	repo := &fakeRepo{}
	m := testModule(repo, &[]string{})
	m.listTrips(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/viajes", nil))
	if !repo.filter.OpenOnly || len(repo.filter.Statuses) != 0 {
		t.Fatalf("filtro por defecto = %#v", repo.filter)
	}
	m.listTrips(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/viajes?estado=todos", nil))
	if repo.filter.OpenOnly || len(repo.filter.Statuses) != 0 {
		t.Fatalf("estado=todos = %#v", repo.filter)
	}
}

func TestListadoHTTPRechazaUnEstadoDesconocido(t *testing.T) {
	m := testModule(&fakeRepo{}, &[]string{})
	rec := httptest.NewRecorder()
	m.listTrips(rec, httptest.NewRequest(http.MethodGet, "/viajes?estado=vendido", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}
