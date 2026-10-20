package operaciones

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

type fakeRepo struct {
	created store.NewTrip
	trip    store.Trip
	inbox   []store.InboxItem
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
	return &Module{repo: repo, log: log, emit: func(eventType string, _ map[string]any) {
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

var _ = time.Time{}
