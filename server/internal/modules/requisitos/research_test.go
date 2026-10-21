package requisitos

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/fuentes"
)

// fakeResearcher devuelve una búsqueda ya comprobada y anota con qué campos se
// la pidieron.
type fakeResearcher struct {
	res   domain.Research
	err   error
	calls int
	bases []domain.Fields
}

func (f *fakeResearcher) Research(_ context.Context, _ domain.Pair, _ string, base domain.Fields) (domain.Research, fuentes.Usage, error) {
	f.calls++
	f.bases = append(f.bases, base)
	return f.res, fuentes.Usage{Model: "claude-opus-5-5", InputTokens: 100, OutputTokens: 20, WebSearches: 3}, f.err
}

var ircc = domain.Citation{Source: domain.SourceOficial, URL: "https://www.canada.ca/visit", Quote: "visitor visa", ReadAt: "2026-10-20"}

func busquedaCanada() domain.Research {
	return domain.Research{ReadAt: "2026-10-20", Grupos: map[string]domain.Grupo{
		domain.GrupoVisa: {Tramite: []domain.Frase{{Text: "Se pide la visa de visitante online.", Source: ircc}}},
	}}
}

func conBusqueda(t *testing.T) (*escenario, *fakeResearcher) {
	e := nuevoEscenario(t)
	f := &fakeResearcher{res: busquedaCanada()}
	e.r.Researcher = f
	return e, f
}

func TestLaReglaNoEsperaALaBusquedaYDespuesSumaElDetalle(t *testing.T) {
	e, f := conBusqueda(t)
	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatalf("búsquedas = %d", f.calls)
	}
	if len(e.store.saves) != 2 {
		t.Fatalf("se guarda la regla y después el detalle: %d guardados", len(e.store.saves))
	}
	primera := e.store.saves[0].Rule
	if primera.Status != domain.RuleVigente || primera.Details.Status != domain.DetallesBuscando {
		t.Errorf("antes de la búsqueda: %s / %s", primera.Status, primera.Details.Status)
	}

	r := e.rule(canada)
	if r.Details.Status != domain.DetallesListo || len(r.Details.Grupos[domain.GrupoVisa].Tramite) != 1 {
		t.Fatalf("detalle = %+v", r.Details)
	}
	if r.Version != 1 {
		t.Errorf("el detalle no sube la versión: %d", r.Version)
	}
	if r.ResearchDueAt == nil || !r.ResearchDueAt.Equal(e.now.Add(researchTTL)) {
		t.Errorf("próxima búsqueda = %v", r.ResearchDueAt)
	}
	if len(e.changed) != 2 {
		t.Errorf("el detalle nuevo también re-evalúa los viajes: %v", e.changed)
	}
	if u := e.store.saves[1].Usage; u == nil || u.WebSearches != 3 || u.Purpose != "oficial" {
		t.Errorf("consumo = %+v", u)
	}
}

func TestLaBusquedaSeRepiteCadaTresMeses(t *testing.T) {
	e, f := conBusqueda(t)
	_ = e.r.Refresh(context.Background(), canada)

	e.now = e.now.Add(33 * 24 * time.Hour)
	_ = e.r.Refresh(context.Background(), canada)
	if f.calls != 1 {
		t.Fatalf("al mes no se busca de nuevo: %d", f.calls)
	}

	e.now = e.now.Add(60 * 24 * time.Hour)
	_ = e.r.Refresh(context.Background(), canada)
	if f.calls != 2 {
		t.Fatalf("a los tres meses sí: %d", f.calls)
	}
}

// Solo se busca donde hay un trámite que explicar: si no se sabe la visa, o no
// hace falta, no se gasta.
func TestSinTramiteNoSeBusca(t *testing.T) {
	e, f := conBusqueda(t)
	e.claude.ex = domain.Extraction{}
	delete(e.pi.snap.Entries, [2]string{"AR", "CA"})
	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatal(err)
	}
	if r := e.rule(canada); f.calls != 0 || r.Details.Status != "" || r.ResearchDueAt != nil {
		t.Fatalf("búsquedas %d, detalle %q, próxima %v", f.calls, r.Details.Status, r.ResearchDueAt)
	}
}

// Una página que no se deja leer no cuesta una búsqueda por día para siempre.
func TestSinResultadosReintentaTresDiasYDespuesUnaVezPorMes(t *testing.T) {
	e, f := conBusqueda(t)
	f.res = domain.Research{}
	for i := 1; i <= 3; i++ {
		if err := e.r.Refresh(context.Background(), canada); err != nil {
			t.Fatal(err)
		}
		r := e.rule(canada)
		if r.Details.Status != domain.DetallesSinDatos || r.ResearchFailures != i {
			t.Fatalf("intento %d: %s, %d fallas", i, r.Details.Status, r.ResearchFailures)
		}
		want := e.now.Add(24 * time.Hour)
		if i == 3 {
			want = e.now.Add(researchTTL)
		}
		if !r.ResearchDueAt.Equal(want) {
			t.Fatalf("intento %d: próxima búsqueda %v, se esperaba %v", i, r.ResearchDueAt, want)
		}
		e.now = *r.ResearchDueAt
	}

	// Un error de la API cuenta igual.
	f.err = errors.New("timeout")
	_ = e.r.Refresh(context.Background(), canada)
	if r := e.rule(canada); r.ResearchFailures != 4 || r.Status != domain.RuleVigente {
		t.Fatalf("con error: %+v", r)
	}
}

// Si la pasada semanal cambia un campo, el detalle de ese campo deja de
// mostrarse y se busca enseguida.
func TestUnCampoQueCambiaVuelveABuscarEnseguida(t *testing.T) {
	e, f := conBusqueda(t)
	_ = e.r.Refresh(context.Background(), canada)

	// La búsqueda guardada se hizo contra otra visa: quedó vieja.
	r := e.rule(canada)
	r.Research.Seen[domain.GrupoVisa] = "otra-huella"
	e.store.rules[canada] = r
	e.now = e.now.Add(8 * 24 * time.Hour)
	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatal(err)
	}
	if f.calls != 2 {
		t.Fatalf("búsquedas = %d, el campo cambió", f.calls)
	}
	if _, ok := e.rule(canada).Details.Grupos[domain.GrupoVisa]; !ok {
		t.Error("la búsqueda nueva vuelve a traer el trámite")
	}
}

// Si la visa pasa a «verificar», no hay trámite que explicar: el detalle de la
// visa vieja se saca y no se busca de nuevo.
func TestSiLaVisaPasaAVerificarSeSacaElDetalleYNoSeBusca(t *testing.T) {
	e, f := conBusqueda(t)
	_ = e.r.Refresh(context.Background(), canada)

	e.pi.snap.Entries[[2]string{"AR", "CA"}] = "eta"
	e.now = e.now.Add(8 * 24 * time.Hour)
	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatalf("búsquedas = %d", f.calls)
	}
	if _, ok := e.rule(canada).Details.Grupos[domain.GrupoVisa]; ok {
		t.Error("el trámite de la visa vieja ya no se muestra")
	}
}

func TestLaBusquedaQueContradiceDejaLaVisaEnVerificar(t *testing.T) {
	e, f := conBusqueda(t)
	f.res.Control.Visa = &domain.Controlled[domain.Visa]{Value: domain.VisaNoRequiere,
		Source: domain.Citation{Source: domain.SourceOficial, URL: "https://www.canada.ca/visit", Quote: "visa not required"}}
	_ = e.r.Refresh(context.Background(), canada)

	r := e.rule(canada)
	if !r.Fields.Visa.Verify || r.Version != 2 {
		t.Fatalf("visa = %+v, versión %d", r.Fields.Visa, r.Version)
	}
	if len(e.store.conflicts) != 1 || e.store.conflicts[0].Source != domain.SourceOficial {
		t.Fatalf("conflictos = %+v", e.store.conflicts)
	}
}

func TestSinBuscadorNoHayDetalleNiBusquedaPendiente(t *testing.T) {
	e := nuevoEscenario(t)
	_ = e.r.Refresh(context.Background(), canada)
	if r := e.rule(canada); r.Details.Status != "" || r.ResearchDueAt != nil {
		t.Fatalf("sin buscador: %+v / %v", r.Details, r.ResearchDueAt)
	}
}
