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
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

type fakeRepo struct {
	created store.NewTrip
	trip    store.Trip
	inbox   []store.InboxItem
	sellers []store.Seller

	pendingDay time.Time
	filter     store.TripFilter
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
func (f *fakeRepo) UpdatePassenger(context.Context, string, store.PassengerPatch) (store.Passenger, error) {
	return store.Passenger{}, nil
}
func (f *fakeRepo) DeletePassenger(context.Context, string) error { return nil }
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
		"GET /clientes/{id}", "PATCH /clientes/{id}", "POST /clientes/{id}/contactos", "GET /bandeja",
		"GET /vendedores", "GET /pendientes",
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
	if len(events) != 1 || events[0] != "trip.created" {
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
	if len(events) != 1 || events[0] != "trip.created" {
		t.Fatalf("eventos = %v", events)
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
	if err != nil || len(entries) != 2 || entries[0].Name() != "0001_init.sql" || entries[1].Name() != "0002_trip_drafts.sql" {
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
		"/viajes?q=cancun&estado=confirmado,en%20viaje&salidaDesde=2026-11-01&salidaHasta=2026-11-30&vendedor=ninguno&atrasados=1&orden=total&cursor=abc", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	f := repo.filter
	if f.Query != "cancun" || len(f.Statuses) != 2 || f.Statuses[1] != "en viaje" || f.OpenOnly ||
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
