package requisitos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/fuentes"
	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/store"
)

// --- Dobles -----------------------------------------------------------------

// fakeWiki devuelve una versión actual y otra para «hace 72 horas».
type fakeWiki struct {
	now, old string
	err      error
	calls    int
}

func (f *fakeWiki) Revision(_ context.Context, _ string, at time.Time) (fuentes.Revision, error) {
	f.calls++
	if f.err != nil {
		return fuentes.Revision{}, f.err
	}
	if at.IsZero() {
		return fuentes.Revision{ID: "200", Content: f.now, URL: "https://wiki/now"}, nil
	}
	return fuentes.Revision{ID: "100", Content: f.old, URL: "https://wiki/old"}, nil
}

type fakeExtractor struct {
	ex    domain.Extraction
	calls int
	texts []string
}

func (f *fakeExtractor) Extract(_ context.Context, _ domain.Pair, _ string, text string) (domain.Extraction, fuentes.Usage, error) {
	f.calls++
	f.texts = append(f.texts, text)
	return f.ex, fuentes.Usage{Model: "claude-opus-5-5", InputTokens: 10, OutputTokens: 5}, nil
}

type fakePI struct {
	snap  fuentes.PassportIndexSnapshot
	err   error
	calls int
}

func (f *fakePI) Download(context.Context) (fuentes.PassportIndexSnapshot, error) {
	f.calls++
	return f.snap, f.err
}

type memStore struct {
	rules     map[domain.Pair]store.StoredRule
	saves     []store.Save
	pi        *store.PassportIndexCopy
	conflicts []domain.Conflict
}

func newMemStore() *memStore { return &memStore{rules: map[domain.Pair]store.StoredRule{}} }

func (m *memStore) StoredRule(_ context.Context, p domain.Pair) (store.StoredRule, error) {
	r, ok := m.rules[p]
	if !ok {
		return store.StoredRule{}, store.ErrNoRule
	}
	return r, nil
}
func (m *memStore) SaveRule(_ context.Context, s store.Save) error {
	m.saves = append(m.saves, s)
	m.rules[s.Rule.Pair] = s.Rule
	m.conflicts = append(m.conflicts, s.Conflicts...)
	return nil
}
func (m *memStore) PassportIndex(context.Context) (*store.PassportIndexCopy, error) { return m.pi, nil }
func (m *memStore) SavePassportIndex(_ context.Context, c store.PassportIndexCopy) error {
	m.pi = &c
	return nil
}

// --- Escenario --------------------------------------------------------------

func pagina(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("fuentes/testdata/visa-requirements-argentine.wikitext")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

var canada = domain.Pair{Nationality: "AR", Country: "CA"}

func extraccionCanada() domain.Extraction {
	visa := domain.VisaConsular
	return domain.Extraction{Visa: domain.Extracted[domain.Visa]{Value: &visa, Quote: "Visa required"}}
}

type escenario struct {
	wiki    *fakeWiki
	claude  *fakeExtractor
	pi      *fakePI
	store   *memStore
	changed [][]domain.Pair
	r       *Refresher
	now     time.Time
}

func nuevoEscenario(t *testing.T) *escenario {
	t.Helper()
	page := pagina(t)
	e := &escenario{
		wiki:   &fakeWiki{now: page, old: page},
		claude: &fakeExtractor{ex: extraccionCanada()},
		pi: &fakePI{snap: fuentes.PassportIndexSnapshot{DataDate: "2026-02-17",
			Entries: map[[2]string]string{{"AR", "CA"}: "visa required", {"AR", "JP"}: "visa required"}}},
		store: newMemStore(),
		now:   time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC),
	}
	e.r = &Refresher{Wiki: e.wiki, Claude: e.claude, PI: e.pi, Store: e.store,
		Now: func() time.Time { return e.now }, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		OnChange: func(_ context.Context, pairs []domain.Pair) { e.changed = append(e.changed, pairs) }}
	e.store.rules[canada] = store.StoredRule{Rule: domain.Rule{Pair: canada, Status: domain.RuleBuscando}}
	return e
}

func (e *escenario) rule(p domain.Pair) store.StoredRule { return e.store.rules[p] }

// --- Tests ------------------------------------------------------------------

func TestUnaReglaNuevaSeExtraeYQuedaVigente(t *testing.T) {
	e := nuevoEscenario(t)

	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatalf("refresco: %v", err)
	}

	got := e.rule(canada)
	if got.Status != domain.RuleVigente || got.Fields.Visa.Value == nil || *got.Fields.Visa.Value != domain.VisaConsular {
		t.Fatalf("regla = %+v", got.Rule)
	}
	if e.claude.calls != 1 || !strings.Contains(e.claude.texts[0], "destination=CA") {
		t.Fatalf("extracciones = %d", e.claude.calls)
	}
	if got.WikiRevision != "200" || got.WikiHash == "" || got.WikiFields == nil || got.Version != 1 {
		t.Errorf("regla guardada = %+v", got)
	}
	if len(e.changed) != 1 || e.changed[0][0] != canada {
		t.Errorf("avisos = %v", e.changed)
	}
	if last := e.store.saves[len(e.store.saves)-1]; last.Usage == nil || last.Usage.InputTokens != 10 {
		t.Errorf("consumo = %+v", last.Usage)
	}
}

// Solo se le paga a Claude cuando cambió el texto.
func TestSiElTextoNoCambioNoSeVuelveAExtraer(t *testing.T) {
	e := nuevoEscenario(t)
	_ = e.r.Refresh(context.Background(), canada)
	e.now = e.now.Add(8 * 24 * time.Hour)

	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatalf("segundo refresco: %v", err)
	}
	if e.claude.calls != 1 {
		t.Fatalf("extracciones = %d, se esperaba 1", e.claude.calls)
	}
	if len(e.changed) != 1 {
		t.Errorf("sin cambios no se avisa: %v", e.changed)
	}
	if got := e.rule(canada); got.CheckedAt == nil || !got.CheckedAt.Equal(e.now) {
		t.Errorf("revisada = %v", got.CheckedAt)
	}
}

// Cuando la extracción aprende a sacar algo nuevo, las reglas ya guardadas se
// vuelven a extraer aunque el texto de Wikipedia no haya cambiado.
func TestUnaExtraccionNuevaVuelveALeerElMismoTexto(t *testing.T) {
	e := nuevoEscenario(t)
	_ = e.r.Refresh(context.Background(), canada)
	text, _ := fuentes.Snippet(pagina(t), mustCountry(t, "CA"))
	viejo := e.rule(canada)
	sum := sha256.Sum256([]byte(text)) // como se guardaba antes de versionar la extracción
	viejo.WikiHash = hex.EncodeToString(sum[:])
	e.store.rules[canada] = viejo
	e.now = e.now.Add(8 * 24 * time.Hour)

	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatal(err)
	}
	if e.claude.calls != 2 {
		t.Fatalf("extracciones = %d, se esperaba volver a extraer", e.claude.calls)
	}
}

func mustCountry(t *testing.T, code string) domain.Country {
	t.Helper()
	c, ok := domain.CountryByCode(code)
	if !ok {
		t.Fatalf("%s no está en el catálogo", code)
	}
	return c
}

// Un cambio en la fila que tiene menos de 72 horas no se usa: las ediciones
// malas se suelen revertir antes.
func TestUnCambioRecienteNoSeUsaHastaQueSobreviva72Horas(t *testing.T) {
	e := nuevoEscenario(t)
	_ = e.r.Refresh(context.Background(), canada)
	vigente := e.rule(canada).Fields

	e.wiki.now = strings.Replace(e.wiki.old, "{{no|Visa required}}<ref>{{Timatic|nationality=AR|destination=CA}}",
		"{{yes|Visa not required}}<ref>{{Timatic|nationality=AR|destination=CA}}", 1)
	e.now = e.now.Add(8 * 24 * time.Hour)
	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatalf("refresco: %v", err)
	}

	got := e.rule(canada)
	if e.claude.calls != 1 {
		t.Fatalf("extracciones = %d: un cambio de menos de 72 horas no se extrae", e.claude.calls)
	}
	if got.PendingSince == nil || *got.Fields.Visa.Value != *vigente.Visa.Value {
		t.Fatalf("regla = %+v, debería seguir con el texto anterior y quedar en espera", got)
	}

	// Pasadas las 72 horas, la versión vieja ya es la nueva: se acepta.
	e.wiki.old = e.wiki.now
	e.now = e.now.Add(4 * 24 * time.Hour)
	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatalf("refresco: %v", err)
	}
	if e.claude.calls != 2 || e.rule(canada).PendingSince != nil {
		t.Fatalf("extracciones = %d, en espera = %v", e.claude.calls, e.rule(canada).PendingSince)
	}
}

// En una regla nueva, si la fila cambió hace poco, no se usa ninguna de las dos
// versiones: la de hace 72 horas puede ser justo la vandalizada que después se
// revirtió. La regla espera a que la fila se estabilice, en verificar.
func TestUnaReglaNuevaConLaFilaEnMovimientoEsperaEnVerificar(t *testing.T) {
	e := nuevoEscenario(t)
	e.wiki.now = strings.Replace(e.wiki.old, "{{no|Visa required}}<ref>{{Timatic|nationality=AR|destination=CA}}",
		"{{yes|Visa not required}}<ref>{{Timatic|nationality=AR|destination=CA}}", 1)

	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatalf("refresco: %v", err)
	}
	if e.claude.calls != 0 {
		t.Fatalf("extracciones = %d: una fila en movimiento no se extrae", e.claude.calls)
	}
	got := e.rule(canada)
	if got.Status != domain.RuleSinFuentes || got.PendingSince == nil {
		t.Errorf("regla = %+v, debería esperar en verificar y revisarse cada día", got)
	}
	if !got.Effective().Visa.Verify {
		t.Errorf("mientras espera, todo cuenta como verificar")
	}
}

func TestSinClaveDeAnthropicUnaReglaNuevaQuedaSinFuentes(t *testing.T) {
	e := nuevoEscenario(t)
	e.r.Claude = nil

	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatalf("refresco: %v", err)
	}
	if got := e.rule(canada); got.Status != domain.RuleSinFuentes {
		t.Fatalf("estado = %s, se esperaba sin_fuentes", got.Status)
	}
}

func TestSiWikipediaSeCaeSeMantieneLaReglaVigente(t *testing.T) {
	e := nuevoEscenario(t)
	_ = e.r.Refresh(context.Background(), canada)
	vigente := e.rule(canada)

	e.wiki.err = errors.New("timeout")
	e.now = e.now.Add(8 * 24 * time.Hour)
	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatalf("refresco: %v", err)
	}
	got := e.rule(canada)
	if got.Status != domain.RuleVigente || got.Version != vigente.Version {
		t.Fatalf("regla = %+v, debería seguir la vigente", got.Rule)
	}
}

func TestSiWikipediaSeCaeYNuncaHuboReglaQuedaSinFuentes(t *testing.T) {
	e := nuevoEscenario(t)
	e.wiki.err = errors.New("timeout")

	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatalf("refresco: %v", err)
	}
	if got := e.rule(canada); got.Status != domain.RuleSinFuentes {
		t.Fatalf("estado = %s", got.Status)
	}
}

func TestLosConflictosConPassportIndexSeRegistran(t *testing.T) {
	e := nuevoEscenario(t)
	e.pi.snap.Entries[[2]string{"AR", "CA"}] = "visa free"

	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatalf("refresco: %v", err)
	}
	if got := e.rule(canada); !got.Fields.Visa.Verify {
		t.Fatalf("visa = %+v, se esperaba verificar", got.Fields.Visa)
	}
	if len(e.store.conflicts) != 1 || e.store.conflicts[0].Other != "visa free" {
		t.Fatalf("conflictos = %+v", e.store.conflicts)
	}
}

func TestConPassportIndexApagadoWikipediaSolaAlcanza(t *testing.T) {
	e := nuevoEscenario(t)
	e.r.PI = nil
	e.pi.snap.Entries[[2]string{"AR", "CA"}] = "visa free"

	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatalf("refresco: %v", err)
	}
	if got := e.rule(canada); got.Fields.Visa.Verify || len(e.store.conflicts) != 0 {
		t.Fatalf("visa = %+v, conflictos = %v", got.Fields.Visa, e.store.conflicts)
	}
}

// La matriz se baja una vez por semana y se guarda: si GitHub no responde, se
// usa la última copia.
func TestLaMatrizSeBajaUnaVezPorSemanaYSeGuarda(t *testing.T) {
	e := nuevoEscenario(t)
	_ = e.r.Refresh(context.Background(), canada)
	e.now = e.now.Add(2 * 24 * time.Hour)
	_ = e.r.Refresh(context.Background(), canada)
	if e.pi.calls != 1 || e.store.pi == nil || e.store.pi.DataDate != "2026-02-17" {
		t.Fatalf("descargas = %d, copia = %+v", e.pi.calls, e.store.pi)
	}

	e.pi.err = errors.New("github caído")
	e.now = e.now.Add(8 * 24 * time.Hour)
	e.pi.snap.Entries = map[[2]string]string{{"AR", "CA"}: "visa free"}
	if err := e.r.Refresh(context.Background(), canada); err != nil {
		t.Fatalf("refresco: %v", err)
	}
	if got := e.rule(canada); got.Fields.Visa.Verify {
		t.Fatalf("con GitHub caído debería usar la copia guardada: %+v", got.Fields.Visa)
	}
}

// Una nacionalidad sin página de visas no tiene de dónde sacar datos.
func TestUnaNacionalidadSinPaginaQuedaSinFuentes(t *testing.T) {
	e := nuevoEscenario(t)
	e.wiki.err = fuentes.ErrPageNotFound
	p := domain.Pair{Nationality: "AQ", Country: "CA"}
	e.store.rules[p] = store.StoredRule{Rule: domain.Rule{Pair: p, Status: domain.RuleBuscando}}

	if err := e.r.Refresh(context.Background(), p); err != nil {
		t.Fatalf("refresco: %v", err)
	}
	if got := e.rule(p); got.Status != domain.RuleSinFuentes {
		t.Fatalf("estado = %s", got.Status)
	}
}
