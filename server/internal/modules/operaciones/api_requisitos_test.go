package operaciones

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/evaluacion"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
	reglas "github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

type evalRepo struct {
	fakeRepo
	input        store.EvaluationInput
	saved        map[string][]evaluacion.Requirement
	confirmation store.NewConfirmation
	openTrips    []string
	usingPairs   []reglas.Pair
	detail       store.TripDetailData
	docs         map[string]string
}

func (r *evalRepo) EvaluationInput(context.Context, string) (store.EvaluationInput, error) {
	return r.input, nil
}
func (r *evalRepo) SaveRequirements(_ context.Context, tripID string, reqs []evaluacion.Requirement, docs map[string]string) error {
	r.docs = docs
	if r.saved == nil {
		r.saved = map[string][]evaluacion.Requirement{}
	}
	r.saved[tripID] = reqs
	return nil
}
func (r *evalRepo) Confirm(_ context.Context, c store.NewConfirmation) error {
	r.confirmation = c
	return nil
}
func (r *evalRepo) OpenTripsUsing(_ context.Context, pairs []reglas.Pair) ([]string, error) {
	r.usingPairs = pairs
	return r.openTrips, nil
}
func (r *evalRepo) OpenTrips(context.Context) ([]string, error) { return r.openTrips, nil }
func (r *evalRepo) TripDetail(context.Context, string) (store.TripDetailData, error) {
	return r.detail, nil
}

type fakeRules struct {
	asked []reglas.Pair
	rules map[reglas.Pair]reglas.Rule
}

func (f *fakeRules) Rules(_ context.Context, pairs []reglas.Pair) (map[reglas.Pair]reglas.Rule, error) {
	f.asked = pairs
	return f.rules, nil
}

func fecha(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}

func viajeACanada() store.EvaluationInput {
	return store.EvaluationInput{Trip: evaluacion.Trip{DepartAt: fecha("2026-12-10"), ReturnAt: fecha("2026-12-24"),
		Itinerary: []evaluacion.Stop{{Country: "CA", Role: "destino"}},
		Passengers: []evaluacion.Passenger{{ID: "ana", Name: "Ana", ResidenceCountry: "AR",
			Doc: &evaluacion.Document{Type: "Pasaporte", Country: "AR", ExpiresAt: fecha("2031-01-01")}}}}}
}

func reglaCanada() map[reglas.Pair]reglas.Rule {
	visa := reglas.VisaConsular
	pair := reglas.Pair{Nationality: "AR", Country: "CA"}
	return map[reglas.Pair]reglas.Rule{pair: {Pair: pair, Status: reglas.RuleVigente,
		Fields: reglas.Fields{Visa: reglas.Field[reglas.Visa]{Value: &visa}}}}
}

func evalModule(repo *evalRepo, rules ruleSource, events *[]string) *Module {
	m := testModule(repo, events)
	m.rules = rules
	return m
}

// Cargar el itinerario dispara la evaluación: la ficha muestra lo que falta en
// la misma respuesta, sin esperar a nadie.
func TestCargarElItinerarioEvaluaElViaje(t *testing.T) {
	repo := &evalRepo{input: viajeACanada()}
	rules := &fakeRules{rules: reglaCanada()}
	var events []string
	m := evalModule(repo, rules, &events)

	m.putItinerary(httptest.NewRecorder(), request(http.MethodPut, "/viajes/trip-1/itinerario",
		`{"itinerary":[{"country":"CA","role":"destino"}]}`, "id", "trip-1"))

	if len(rules.asked) != 1 || rules.asked[0] != (reglas.Pair{Nationality: "AR", Country: "CA"}) {
		t.Fatalf("reglas pedidas = %v", rules.asked)
	}
	var visa *evaluacion.Requirement
	for i, r := range repo.saved["trip-1"] {
		if r.Key == evaluacion.KeyVisa {
			visa = &repo.saved["trip-1"][i]
		}
	}
	if visa == nil || visa.Status != evaluacion.Falta {
		t.Fatalf("requisitos guardados = %+v", repo.saved["trip-1"])
	}
	if len(events) == 0 || events[len(events)-1] != "requirements.changed" {
		t.Fatalf("eventos = %v", events)
	}
}

// Sin el módulo de requisitos, el viaje se evalúa igual: todo queda en
// verificar, nunca en «OK».
func TestSinReglasTodoQuedaEnVerificar(t *testing.T) {
	repo := &evalRepo{input: viajeACanada()}
	m := evalModule(repo, nil, &[]string{})

	if err := m.reevaluate(context.Background(), "trip-1"); err != nil {
		t.Fatal(err)
	}
	for _, r := range repo.saved["trip-1"] {
		if r.Key == evaluacion.KeyVisa && r.Status != evaluacion.Verificar {
			t.Fatalf("visa sin reglas = %+v", r)
		}
	}
}

func TestCambiarUnaReglaReevaluaLosViajesQueLaUsan(t *testing.T) {
	repo := &evalRepo{input: viajeACanada(), openTrips: []string{"trip-1", "trip-2"}}
	var events []string
	m := evalModule(repo, &fakeRules{rules: reglaCanada()}, &events)

	pair := reglas.Pair{Nationality: "AR", Country: "CA"}
	m.OnRulesChanged(context.Background(), []reglas.Pair{pair})

	if len(repo.usingPairs) != 1 || repo.usingPairs[0] != pair {
		t.Fatalf("combinaciones = %v", repo.usingPairs)
	}
	if len(repo.saved) != 2 || len(events) != 2 {
		t.Fatalf("re-evaluados = %d, eventos = %v", len(repo.saved), events)
	}
}

func TestConfirmarUnRequisitoHTTP(t *testing.T) {
	repo := &evalRepo{input: viajeACanada()}
	var events []string
	m := evalModule(repo, &fakeRules{rules: reglaCanada()}, &events)
	rec := httptest.NewRecorder()

	m.confirmRequirement(rec, request(http.MethodPost, "/viajes/trip-1/requisitos/confirmar",
		`{"passengerId":"ana","country":"CA","key":"visa"}`, "id", "trip-1"))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if c := repo.confirmation; c.TripID != "trip-1" || c.PassengerID != "ana" || c.Country != "CA" || c.Key != "visa" {
		t.Fatalf("confirmación = %+v", c)
	}
	if repo.saved["trip-1"] == nil {
		t.Fatal("confirmar debería re-evaluar")
	}
}

func TestConfirmarAlgoNoConfirmableEsUnErrorDeQuienLoPide(t *testing.T) {
	repo := &failingConfirm{evalRepo: evalRepo{input: viajeACanada()}}
	m := evalModule(&repo.evalRepo, nil, &[]string{})
	m.repo = repo
	rec := httptest.NewRecorder()

	m.confirmRequirement(rec, request(http.MethodPost, "/viajes/trip-1/requisitos/confirmar",
		`{"passengerId":"ana","country":"CA","key":"vigencia"}`, "id", "trip-1"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

type failingConfirm struct{ evalRepo }

func (f *failingConfirm) Confirm(context.Context, store.NewConfirmation) error {
	return store.ErrNoConfirmable
}

// La ficha trae el aviso de requisitos ya armado con lo que falta.
func TestLaFichaTraeElAvisoDeRequisitosArmado(t *testing.T) {
	repo := &evalRepo{detail: store.TripDetailData{
		Trip:      store.Trip{ID: "trip-1"},
		Customer:  store.Customer{Name: "Juan Pérez"},
		Itinerary: []store.Stop{{Country: "CA", Role: "destino"}},
		Passengers: []store.Passenger{{ID: "ana", FirstName: "Ana", IsHolder: true,
			Documents: []store.Document{}, Permits: []store.Permit{}}},
		Requirements: []store.TripRequirement{{Requirement: evaluacion.Requirement{PassengerID: "ana", Country: "CA",
			Key: evaluacion.KeyVisa, Status: evaluacion.Falta, ClientText: "para Canadá necesita visa"}}},
		Controls: []store.Control{{ItemID: "control-visa", Message: "{pax}, te confirmo los requisitos"},
			{ItemID: "control-flight-plus", Message: "{pax}, te envío la Flight Plus"}},
	}}
	m := evalModule(repo, nil, &[]string{})
	rec := httptest.NewRecorder()

	m.getTrip(rec, request(http.MethodGet, "/viajes/trip-1", "", "id", "trip-1"))

	var body store.TripDetailData
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if msg := body.Controls[0].Message; !strings.Contains(msg, "Para Canadá necesita visa.") || !strings.HasPrefix(msg, "Ana") {
		t.Fatalf("aviso = %q", msg)
	}
	if body.Controls[1].Message != "{pax}, te envío la Flight Plus" {
		t.Errorf("los demás mensajes no cambian: %q", body.Controls[1].Message)
	}
}

// Sin pasajeros, la ficha muestra lo que piden los países a un pasaporte
// argentino. Pedir las reglas también las pone a buscar.
func TestLaFichaSinPasajerosTraeLaVistaPrevia(t *testing.T) {
	repo := &evalRepo{detail: store.TripDetailData{Trip: store.Trip{ID: "trip-1"}, OriginCountry: "AR",
		Itinerary: []store.Stop{{Country: "CA", Role: "destino"}}}}
	rules := &fakeRules{rules: reglaCanada()}
	m := evalModule(repo, rules, &[]string{})
	rec := httptest.NewRecorder()

	m.getTrip(rec, request(http.MethodGet, "/viajes/trip-1", "", "id", "trip-1"))

	var body struct {
		Preview []evaluacion.PreviewLine `json:"preview"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(rules.asked) != 1 || rules.asked[0] != (reglas.Pair{Nationality: "AR", Country: "CA"}) {
		t.Fatalf("reglas pedidas = %v", rules.asked)
	}
	var visa bool
	for _, l := range body.Preview {
		if l.Country == "CA" && l.Key == evaluacion.KeyVisa && strings.Contains(l.Text, "pide visa") {
			visa = true
		}
	}
	if !visa {
		t.Fatalf("vista previa = %+v", body.Preview)
	}

	// Con un pasajero, no hay vista previa.
	repo.detail.Passengers = []store.Passenger{{ID: "ana"}}
	rec = httptest.NewRecorder()
	m.getTrip(rec, request(http.MethodGet, "/viajes/trip-1", "", "id", "trip-1"))
	if strings.Contains(rec.Body.String(), `"preview"`) {
		t.Fatalf("con pasajeros: %s", rec.Body.String())
	}
}
