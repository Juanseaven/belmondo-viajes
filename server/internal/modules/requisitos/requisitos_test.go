package requisitos

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

type fakeQueue struct {
	due     []domain.Pair
	ensured []domain.Pair
	created []domain.Pair
}

func (f *fakeQueue) Due(context.Context, time.Time, int) ([]domain.Pair, error) {
	due := f.due
	f.due = nil
	return due, nil
}
func (f *fakeQueue) Ensure(_ context.Context, pairs []domain.Pair) (map[domain.Pair]domain.Rule, []domain.Pair, error) {
	f.ensured = pairs
	out := map[domain.Pair]domain.Rule{}
	for _, p := range pairs {
		out[p] = domain.Rule{Pair: p, Status: domain.RuleBuscando}
	}
	return out, f.created, nil
}

type fakeRefresher struct{ refreshed []domain.Pair }

func (f *fakeRefresher) Refresh(_ context.Context, p domain.Pair) error {
	f.refreshed = append(f.refreshed, p)
	return nil
}

func testModule(q *fakeQueue, r *fakeRefresher) *Module {
	return &Module{queue: q, refresher: r, log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		now: time.Now, wake: make(chan struct{}, 1)}
}

func TestElCatalogoDePaisesSeSirveEnEspanol(t *testing.T) {
	m := testModule(&fakeQueue{}, &fakeRefresher{})
	rec := httptest.NewRecorder()

	m.countries(rec, httptest.NewRequest(http.MethodGet, "/paises", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Items []domain.Country `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range body.Items {
		if c.Code == "AR" && c.Name == "Argentina" {
			found = true
		}
	}
	if !found || len(body.Items) < 190 {
		t.Fatalf("países = %d, Argentina = %v", len(body.Items), found)
	}
}

// Pedir una regla que no existe despierta a quien busca: la ficha no puede
// quedarse en «buscando» hasta la próxima vuelta del reloj.
func TestPedirUnaReglaNuevaDespiertaALaBusqueda(t *testing.T) {
	brasil := domain.Pair{Nationality: "AR", Country: "BR"}
	q := &fakeQueue{created: []domain.Pair{brasil}}
	m := testModule(q, &fakeRefresher{})

	rules, err := m.Rules(context.Background(), []domain.Pair{brasil})
	if err != nil || rules[brasil].Status != domain.RuleBuscando {
		t.Fatalf("reglas = %+v, %v", rules, err)
	}
	select {
	case <-m.wake:
	default:
		t.Fatal("una regla nueva debería despertar la búsqueda")
	}
}

func TestUnaVueltaRevisaLasReglasPendientes(t *testing.T) {
	pairs := []domain.Pair{{Nationality: "AR", Country: "BR"}, {Nationality: "AR", Country: "CA"}}
	r := &fakeRefresher{}
	m := testModule(&fakeQueue{due: pairs}, r)

	m.runOnce(context.Background())

	if len(r.refreshed) != 2 || r.refreshed[1] != pairs[1] {
		t.Fatalf("revisadas = %v", r.refreshed)
	}
}
