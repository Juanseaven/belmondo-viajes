package fuentes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// testdata/passport-index-ar-it.csv son las filas de pasaportes argentinos e
// italianos de imorte/passport-index-data (MIT), febrero de 2026.
func TestLaMatrizSeLeePorPasaporteYDestino(t *testing.T) {
	f, err := os.Open("testdata/passport-index-ar-it.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	m, err := ParsePassportIndex(f)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	casos := map[[2]string]string{
		{"AR", "BR"}: "90", {"AR", "US"}: "visa required", {"AR", "GB"}: "eta", {"AR", "AU"}: "e-visa",
		{"AR", "AR"}: "-1",
	}
	for k, want := range casos {
		if got, ok := m[k]; !ok || got != want {
			t.Errorf("%v = %q, se esperaba %q", k, got, want)
		}
	}
	if _, ok := m[[2]string{"BR", "AR"}]; ok {
		t.Error("el fixture no tiene pasaportes brasileños")
	}
}

func TestUnaMatrizSinEncabezadoSeRechaza(t *testing.T) {
	if _, err := ParsePassportIndex(strings.NewReader("AR,BR,90\n")); err == nil {
		t.Fatal("sin encabezado no es la matriz que se espera")
	}
}

func TestLaDescargaTraeLaMatrizYLaFechaDeSusDatos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".csv"):
			_, _ = w.Write([]byte("Passport,Destination,Requirement\nAR,BR,90\n"))
		case strings.Contains(r.URL.Path, "/commits"):
			if r.URL.Query().Get("path") != passportIndexFile {
				t.Errorf("path = %q", r.URL.Query().Get("path"))
			}
			_, _ = w.Write([]byte(`[{"commit":{"committer":{"date":"2026-02-17T21:42:15Z"}}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	pi := &PassportIndex{RawURL: srv.URL + "/" + passportIndexFile, CommitsURL: srv.URL + "/commits", HTTP: srv.Client()}

	snap, err := pi.Download(context.Background())
	if err != nil {
		t.Fatalf("descarga: %v", err)
	}
	if snap.DataDate != "2026-02-17" || snap.Entries[[2]string{"AR", "BR"}] != "90" {
		t.Fatalf("matriz = %+v", snap)
	}
}
