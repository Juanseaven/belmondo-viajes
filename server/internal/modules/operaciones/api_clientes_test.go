package operaciones

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// Un viaje es de un chat o de un cliente cargado a mano. Con los dos, o sin
// ninguno, no se sabe de quién es.
func TestAltaHTTPPideUnContactoOUnClienteUnoDeLosDos(t *testing.T) {
	for name, body := range map[string]string{
		"ninguno":   `{"destination":"Salta"}`,
		"los dos":   `{"contactId":"contact-1","customerId":"cliente-1"}`,
		"en blanco": `{"contactId":"  ","customerId":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			m := testModule(&fakeRepo{}, &[]string{})
			rec := httptest.NewRecorder()
			m.createTrip(rec, httptest.NewRequest(http.MethodPost, "/viajes", bytes.NewBufferString(body)))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, se esperaba 400", rec.Code)
			}
		})
	}
}

func TestAltaHTTPConClienteNoPasaPorElContacto(t *testing.T) {
	repo := &fakeRepo{trip: store.Trip{ID: "trip-1", Number: 7, CustomerID: "cliente-1"}}
	var events []string
	m := testModule(repo, &events)
	rec := httptest.NewRecorder()
	m.createTrip(rec, httptest.NewRequest(http.MethodPost, "/viajes",
		bytes.NewBufferString(`{"customerId":"cliente-1","destination":"Salta"}`)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if repo.created.CustomerID != "cliente-1" || repo.created.ContactID != "" {
		t.Fatalf("alta traducida: %#v", repo.created)
	}
	if len(events) == 0 || events[0] != "trip.created" {
		t.Fatalf("eventos = %v", events)
	}
}

func TestAltaDeClienteHTTPPideElNombreYAvisa(t *testing.T) {
	repo := &fakeRepo{}
	var events []string
	m := testModule(repo, &events)

	rec := httptest.NewRecorder()
	m.createCustomer(rec, httptest.NewRequest(http.MethodPost, "/clientes",
		bytes.NewBufferString(`{"name":"   ","phoneNumber":"11 4788-2210"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin nombre: status = %d, se esperaba 400", rec.Code)
	}

	rec = httptest.NewRecorder()
	m.createCustomer(rec, httptest.NewRequest(http.MethodPost, "/clientes",
		bytes.NewBufferString(`{"name":" Marta Pérez ","phoneNumber":" 11 4788-2210 ","documentType":"DNI"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if repo.newCustomer.Name != "Marta Pérez" || repo.newCustomer.PhoneNumber != "11 4788-2210" ||
		repo.newCustomer.DocumentType != "DNI" {
		t.Fatalf("alta traducida: %#v", repo.newCustomer)
	}
	if len(events) != 1 || events[0] != "customer.created" {
		t.Fatalf("eventos = %v", events)
	}
}

func TestListaDeClientesHTTPTraduceFiltroYOrden(t *testing.T) {
	repo := &fakeRepo{}
	m := testModule(repo, &[]string{})

	rec := httptest.NewRecorder()
	m.listCustomers(rec, httptest.NewRequest(http.MethodGet, "/clientes?q=marta&filtro=sin-whatsapp&orden=nombre", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if repo.customerFilter.Query != "marta" || repo.customerFilter.Chip != store.ChipSinWhatsApp ||
		repo.customerFilter.Sort != store.SortNombre {
		t.Fatalf("filtro = %#v", repo.customerFilter)
	}

	for _, bad := range []string{"/clientes?filtro=raros", "/clientes?orden=edad", "/clientes?limit=0"} {
		rec := httptest.NewRecorder()
		m.listCustomers(rec, httptest.NewRequest(http.MethodGet, bad, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, se esperaba 400", bad, rec.Code)
		}
	}
}

func TestTraerPasajerosHTTPPideAlMenosUno(t *testing.T) {
	repo := &fakeRepo{}
	var events []string
	m := testModule(repo, &events)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/viajes/trip-1/pasajeros/traer", bytes.NewBufferString(`{"passengerIds":[]}`))
	req.SetPathValue("id", "trip-1")
	m.copyPassengers(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin pasajeros: status = %d, se esperaba 400", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/viajes/trip-1/pasajeros/traer", bytes.NewBufferString(`{"passengerIds":["p-1","p-2"]}`))
	req.SetPathValue("id", "trip-1")
	m.copyPassengers(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(repo.copied) != 2 || len(events) != 2 || events[0] != "passenger.changed" || events[1] != "requirements.changed" {
		t.Fatalf("copiados = %v, eventos = %v", repo.copied, events)
	}
}

// Un chat que ya es de un cliente no sugiere nada: vincularlo sería unir dos
// clientes.
func TestElContactoDiceSuClienteYSoloSugiereSiNoTiene(t *testing.T) {
	repo := &fakeRepo{contactCustomer: "cliente-1"}
	m := testModule(repo, &[]string{})
	req := httptest.NewRequest(http.MethodGet, "/contactos/c-1/clientes", nil)
	req.SetPathValue("id", "c-1")
	rec := httptest.NewRecorder()
	m.contactCustomers(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"customerId":"cliente-1"`)) ||
		!bytes.Contains(rec.Body.Bytes(), []byte(`"items":[]`)) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
