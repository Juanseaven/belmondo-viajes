package operaciones

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

func (f *fakeRepo) PutItinerary(_ context.Context, _ string, stops []store.Stop) error {
	f.itinerary = stops
	return f.err
}
func (f *fakeRepo) AddDocument(_ context.Context, n store.NewDocument) (store.Document, error) {
	f.newDocument = n
	return store.Document{ID: "doc-1", PassengerID: n.PassengerID, TripID: "trip-1"}, f.err
}
func (f *fakeRepo) UpdateDocument(_ context.Context, _ string, patch store.DocumentPatch) (store.Document, error) {
	f.documentPatch = patch
	return store.Document{ID: "doc-1", TripID: "trip-1"}, f.err
}
func (f *fakeRepo) DeleteDocument(context.Context, string) (string, error) { return "trip-1", f.err }
func (f *fakeRepo) AddPermit(_ context.Context, n store.NewPermit) (store.Permit, error) {
	f.newPermit = n
	return store.Permit{ID: "permit-1", PassengerID: n.PassengerID, TripID: "trip-1"}, f.err
}
func (f *fakeRepo) UpdatePermit(context.Context, string, store.PermitPatch) (store.Permit, error) {
	return store.Permit{ID: "permit-1", TripID: "trip-1"}, f.err
}
func (f *fakeRepo) DeletePermit(context.Context, string) (string, error) { return "trip-1", f.err }

func request(method, target, body string, pathValues ...string) *http.Request {
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	for i := 0; i+1 < len(pathValues); i += 2 {
		req.SetPathValue(pathValues[i], pathValues[i+1])
	}
	return req
}

func TestItinerarioHTTPTraduceLosPaisesYEmite(t *testing.T) {
	repo := &fakeRepo{}
	var events []string
	m := testModule(repo, &events)
	rec := httptest.NewRecorder()

	m.putItinerary(rec, request(http.MethodPut, "/viajes/trip-1/itinerario",
		`{"itinerary":[{"country":"ES","role":"destino","arriveAt":"2026-12-11","leaveAt":"2026-12-16"},{"country":"PA","role":"escala"}]}`,
		"id", "trip-1"))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(repo.itinerary) != 2 || repo.itinerary[0].Country != "ES" || repo.itinerary[0].ArriveAt == nil ||
		repo.itinerary[0].ArriveAt.Format("2006-01-02") != "2026-12-11" || repo.itinerary[1].Role != "escala" ||
		repo.itinerary[1].ArriveAt != nil {
		t.Fatalf("itinerario = %#v", repo.itinerary)
	}
	if len(events) != 2 || events[0] != "trip.updated" || events[1] != "requirements.changed" {
		t.Fatalf("eventos = %v", events)
	}
}

func TestItinerarioHTTPRechazaFechasInvalidasAntesDelStore(t *testing.T) {
	repo := &fakeRepo{}
	var events []string
	m := testModule(repo, &events)
	rec := httptest.NewRecorder()

	m.putItinerary(rec, request(http.MethodPut, "/viajes/trip-1/itinerario",
		`{"itinerary":[{"country":"ES","role":"destino","arriveAt":"mañana"}]}`, "id", "trip-1"))

	if rec.Code != http.StatusBadRequest || repo.itinerary != nil || len(events) != 0 {
		t.Fatalf("status = %d, itinerario = %#v, eventos = %v", rec.Code, repo.itinerary, events)
	}
}

// Un dato que el store rechaza por inválido es un error de quien lo cargó, no
// del servidor.
func TestItinerarioHTTPDevuelve400SiElStoreLoRechaza(t *testing.T) {
	repo := &fakeRepo{err: store.ErrInvalid}
	m := testModule(repo, &[]string{})
	rec := httptest.NewRecorder()

	m.putItinerary(rec, request(http.MethodPut, "/viajes/trip-1/itinerario",
		`{"itinerary":[{"country":"ES","role":"crucero"}]}`, "id", "trip-1"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestDocumentoHTTPCargaElDocumentoDelPasajero(t *testing.T) {
	repo := &fakeRepo{}
	var events []string
	m := testModule(repo, &events)
	rec := httptest.NewRecorder()

	m.addDocument(rec, request(http.MethodPost, "/pasajeros/p-1/documentos",
		`{"type":"Pasaporte","country":"IT","number":"YA1","issuedAt":"2020-05-01","expiresAt":"2030-05-01"}`,
		"id", "p-1"))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	d := repo.newDocument
	if d.PassengerID != "p-1" || d.Type != "Pasaporte" || d.Country != "IT" || d.Number != "YA1" ||
		d.IssuedAt == nil || d.IssuedAt.Format("2006-01-02") != "2020-05-01" ||
		d.ExpiresAt == nil || d.ExpiresAt.Format("2006-01-02") != "2030-05-01" {
		t.Fatalf("documento = %#v", d)
	}
	if len(events) != 2 || events[0] != "passenger.changed" || events[1] != "requirements.changed" {
		t.Fatalf("eventos = %v", events)
	}
}

func TestDocumentoHTTPRechazaUnVencimientoInvalido(t *testing.T) {
	repo := &fakeRepo{}
	m := testModule(repo, &[]string{})
	rec := httptest.NewRecorder()

	m.addDocument(rec, request(http.MethodPost, "/pasajeros/p-1/documentos",
		`{"type":"DNI","country":"AR","expiresAt":"31/12/2030"}`, "id", "p-1"))

	if rec.Code != http.StatusBadRequest || repo.newDocument.PassengerID != "" {
		t.Fatalf("status = %d, documento = %#v", rec.Code, repo.newDocument)
	}
}

func TestDocumentoHTTPEditaYBorra(t *testing.T) {
	repo := &fakeRepo{}
	var events []string
	m := testModule(repo, &events)

	rec := httptest.NewRecorder()
	m.updateDocument(rec, request(http.MethodPatch, "/documentos/doc-1", `{"expiresAt":"2031-01-01"}`, "id", "doc-1"))
	if rec.Code != http.StatusOK || repo.documentPatch.ExpiresAt == nil || *repo.documentPatch.ExpiresAt != "2031-01-01" {
		t.Fatalf("edición: status = %d, patch = %#v", rec.Code, repo.documentPatch)
	}

	rec = httptest.NewRecorder()
	m.deleteDocument(rec, request(http.MethodDelete, "/documentos/doc-1", "", "id", "doc-1"))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("borrado: status = %d", rec.Code)
	}
	if len(events) != 4 || events[0] != "passenger.changed" || events[2] != "passenger.changed" ||
		events[3] != "requirements.changed" {
		t.Fatalf("eventos = %v", events)
	}
}

func TestPermisoHTTPCargaLaVisaDelPasajero(t *testing.T) {
	repo := &fakeRepo{}
	var events []string
	m := testModule(repo, &events)
	rec := httptest.NewRecorder()

	m.addPermit(rec, request(http.MethodPost, "/pasajeros/p-1/permisos",
		`{"kind":"visa","country":"US","label":"B1/B2","validFrom":"2022-01-10","expiresAt":"2032-01-10"}`, "id", "p-1"))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	pm := repo.newPermit
	if pm.PassengerID != "p-1" || pm.Kind != "visa" || pm.Country != "US" || pm.Label != "B1/B2" ||
		pm.ValidFrom == nil || pm.ExpiresAt == nil || pm.ExpiresAt.Format("2006-01-02") != "2032-01-10" {
		t.Fatalf("permiso = %#v", pm)
	}
	if len(events) != 2 || events[0] != "passenger.changed" || events[1] != "requirements.changed" {
		t.Fatalf("eventos = %v", events)
	}
}

func TestPasajeroHTTPAceptaNacimientoResidenciaYDocumentoDeViaje(t *testing.T) {
	repo := &fakeRepo{}
	m := testModule(repo, &[]string{})
	rec := httptest.NewRecorder()

	m.updatePassenger(rec, request(http.MethodPatch, "/pasajeros/p-1",
		`{"birthDate":"2010-03-04","residenceCountry":"ES","travelDocumentId":"doc-2"}`, "id", "p-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	p := repo.passengerPatch
	if p.BirthDate == nil || *p.BirthDate != "2010-03-04" || p.ResidenceCountry == nil || *p.ResidenceCountry != "ES" ||
		p.TravelDocumentID == nil || *p.TravelDocumentID != "doc-2" {
		t.Fatalf("patch = %#v", p)
	}
}
