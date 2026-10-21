package fuentes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

// testdata/visa-requirements-argentine.wikitext es la página «Visa requirements
// for Argentine citizens» de Wikipedia en inglés (CC BY-SA 4.0), tal como
// estaba el 2026-10-20.
func page(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/visa-requirements-argentine.wikitext")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func country(t *testing.T, code string) domain.Country {
	t.Helper()
	c, ok := domain.CountryByCode(code)
	if !ok {
		t.Fatalf("%s no está en el catálogo", code)
	}
	return c
}

func TestElRecorteTomaLaFilaDelPaisPorSuCodigo(t *testing.T) {
	text, ok := Snippet(page(t), country(t, "BR"))
	if !ok {
		t.Fatal("Brasil está en la tabla")
	}
	if !strings.Contains(text, "destination=BR") || !strings.Contains(text, "Freedom of Movement") {
		t.Fatalf("la fila de Brasil no está en el recorte:\n%s", text)
	}
	if strings.Contains(text, "destination=UY") {
		t.Error("el recorte trae la fila de Uruguay")
	}
	// La sección de documentos sin pasaporte viaja siempre: es la que dice si
	// alcanza el DNI.
	if !strings.Contains(text, "Passport not required") {
		t.Errorf("falta la sección de documentos:\n%s", text)
	}
}

func TestElRecorteTraeLasVacunasDelPais(t *testing.T) {
	text, ok := Snippet(page(t), country(t, "AO"))
	if !ok {
		t.Fatal("Angola está en la tabla")
	}
	if !strings.Contains(text, "Yellow fever vaccination requirements for international travel (July 2019)") ||
		!strings.Contains(text, "All countries") {
		t.Fatalf("faltan las vacunas de Angola:\n%s", text)
	}
	if strings.Contains(text, "{{flagu|Albania}}") {
		t.Error("el recorte trae vacunas de otro país")
	}
}

// La vigencia del pasaporte no está en la fila sino en una sección aparte, con
// listas de países por meses y una frase para los que no están en ninguna.
func TestElRecorteTraeLaVigenciaDelPasaporte(t *testing.T) {
	text, ok := Snippet(page(t), country(t, "US"))
	if !ok {
		t.Fatal("Estados Unidos está en la tabla")
	}
	for _, want := range []string{
		"Countries requiring passports to be valid at least 6 months on arrival include",
		"Other countries require either a passport valid on arrival or passport valid throughout the period of intended stay.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("falta %q en el recorte:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Medical passport") {
		t.Error("el recorte trae la sección siguiente")
	}
}

func TestSinCodigoLaFilaSeBuscaPorNombre(t *testing.T) {
	wikitext := "==Visa requirements==\n{|\n|-\n| {{flag|Georgia}}\n| {{yes|Visa not required}}\n| 365 days\n|-\n| {{flag|Germany}}\n| {{yes|Visa not required}}\n|}\n"
	text, ok := Snippet(wikitext, country(t, "GE"))
	if !ok || !strings.Contains(text, "365 days") || strings.Contains(text, "Germany") {
		t.Fatalf("recorte = %q, %v", text, ok)
	}
}

func TestUnPaisQueNoEstaEnLaTablaNoTieneRecorte(t *testing.T) {
	if _, ok := Snippet("==Visa requirements==\n{|\n|-\n| {{flag|Chile}}\n|}\n", country(t, "JP")); ok {
		t.Fatal("Japón no está en esta tabla")
	}
}

// La espera de 72 horas compara recortes: una edición en la fila de otro país
// no puede hacer parecer que cambió la de este.
func TestUnaEdicionEnOtraFilaNoCambiaElRecorte(t *testing.T) {
	original := page(t)
	editada := strings.Replace(original, "{{Timatic|nationality=AR|destination=UY}}",
		"{{Timatic|nationality=AR|destination=UY}} vandalismo", 1)
	a, _ := Snippet(original, country(t, "BR"))
	b, _ := Snippet(editada, country(t, "BR"))
	if a != b {
		t.Fatal("una edición en Uruguay cambió el recorte de Brasil")
	}
}

func TestWikipediaPideLaRevisionVigenteAUnaFecha(t *testing.T) {
	var query map[string][]string
	var agent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, agent = r.URL.Query(), r.UserAgent()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":{"pages":[{"title":"Visa requirements for Argentine citizens",
			"revisions":[{"revid":4321,"slots":{"main":{"content":"==Visa requirements=="}}}]}]}}`))
	}))
	defer srv.Close()
	wp := &Wikipedia{BaseURL: srv.URL, Contact: "agencia@example.com", HTTP: srv.Client()}

	at := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	rev, err := wp.Revision(context.Background(), "Visa requirements for Argentine citizens", at)
	if err != nil {
		t.Fatalf("revisión: %v", err)
	}
	if rev.ID != "4321" || rev.Content != "==Visa requirements==" {
		t.Fatalf("revisión = %+v", rev)
	}
	if got := query["rvstart"]; len(got) != 1 || got[0] != "2026-10-17T12:00:00Z" {
		t.Errorf("rvstart = %v", got)
	}
	if got := query["rvdir"]; len(got) != 1 || got[0] != "older" {
		t.Errorf("rvdir = %v", got)
	}
	if !strings.Contains(agent, "agencia@example.com") {
		t.Errorf("User-Agent = %q, debería llevar el contacto", agent)
	}
	if !strings.Contains(rev.URL, "Visa_requirements_for_Argentine_citizens") || !strings.Contains(rev.URL, "oldid=4321") {
		t.Errorf("URL = %q", rev.URL)
	}
}

func TestWikipediaSinFechaPideLaUltimaRevision(t *testing.T) {
	var query map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		_, _ = w.Write([]byte(`{"query":{"pages":[{"revisions":[{"revid":1,"slots":{"main":{"content":"x"}}}]}]}}`))
	}))
	defer srv.Close()
	wp := &Wikipedia{BaseURL: srv.URL, HTTP: srv.Client()}

	if _, err := wp.Revision(context.Background(), "X", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := query["rvstart"]; ok {
		t.Error("sin fecha no debería mandar rvstart")
	}
}

func TestWikipediaAvisaSiLaPaginaNoExiste(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"query":{"pages":[{"title":"X","missing":true}]}}`))
	}))
	defer srv.Close()
	wp := &Wikipedia{BaseURL: srv.URL, HTTP: srv.Client()}

	if _, err := wp.Revision(context.Background(), "X", time.Time{}); err != ErrPageNotFound {
		t.Fatalf("error = %v, se esperaba ErrPageNotFound", err)
	}
}
