package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/blobs"
)

func newFileServer(t *testing.T) (*blobs.Local, func(key string) *httptest.ResponseRecorder) {
	t.Helper()
	local, err := blobs.NewLocal(t.TempDir(), "http://localhost:8080",
		"0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	handler := filesHandler(local)

	return local, func(key string) *httptest.ResponseRecorder {
		signed, err := local.SignedURL(context.Background(), key, time.Minute)
		if err != nil {
			t.Fatalf("SignedURL(%q): %v", key, err)
		}
		req := httptest.NewRequest("GET", strings.TrimPrefix(signed, "http://localhost:8080"), nil)
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec
	}
}

// El caso que importa: alguien manda un .html o un .svg a una línea conectada.
// El nombre del archivo lo elige el remitente, y de ahí sale la extensión.
//
// Si esto se sirviera como text/html o image/svg+xml, abrir el "documento" desde
// el panel ejecutaría script del remitente en el origen del backend.
func TestContenidoActivoNoSeSirveParaMostrar(t *testing.T) {
	local, get := newFileServer(t)
	ctx := context.Background()

	peligrosos := []string{
		"ana/2026-08/msg.html",
		"ana/2026-08/msg.htm",
		"ana/2026-08/msg.svg",
		"ana/2026-08/msg.xhtml",
		"ana/2026-08/msg.js",
		"ana/2026-08/msg.pdf", // el visor de PDF ejecuta JavaScript embebido
	}

	for _, key := range peligrosos {
		t.Run(key, func(t *testing.T) {
			if err := local.Put(ctx, key, "text/html", []byte(`<script>alert(1)</script>`)); err != nil {
				t.Fatalf("Put: %v", err)
			}

			rec := get(key)

			if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
				t.Fatalf("Content-Type = %q, se esperaba application/octet-stream", ct)
			}
			if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") {
				t.Fatalf("Content-Disposition = %q, se esperaba attachment", cd)
			}
		})
	}
}

// Lo que el panel sí muestra en línea tiene que seguir funcionando: una foto de
// pasaporte se mira, no se descarga.
func TestImagenesYAudiosSeMuestranEnLinea(t *testing.T) {
	local, get := newFileServer(t)
	ctx := context.Background()

	cases := map[string]string{
		"a/foto.jpg":  "image/jpeg",
		"a/foto.PNG":  "image/png", // la extensión puede venir en mayúsculas
		"a/nota.ogg":  "audio/ogg",
		"a/video.mp4": "video/mp4",
	}

	for key, want := range cases {
		t.Run(key, func(t *testing.T) {
			if err := local.Put(ctx, key, want, []byte("binario")); err != nil {
				t.Fatalf("Put: %v", err)
			}

			rec := get(key)

			if ct := rec.Header().Get("Content-Type"); ct != want {
				t.Fatalf("Content-Type = %q, se esperaba %q", ct, want)
			}
			if cd := rec.Header().Get("Content-Disposition"); cd != "inline" {
				t.Fatalf("Content-Disposition = %q, se esperaba inline", cd)
			}
			if rec.Body.String() != "binario" {
				t.Fatalf("cuerpo = %q", rec.Body.String())
			}
		})
	}
}

func TestSiempreLlevaLasCabecerasDeDefensa(t *testing.T) {
	local, get := newFileServer(t)
	_ = local.Put(context.Background(), "a/foto.jpg", "image/jpeg", []byte("x"))

	rec := get("a/foto.jpg")

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "sandbox") {
		t.Fatalf("Content-Security-Policy = %q, se esperaba sandbox", got)
	}
}

// El nombre va dentro de una cabecera entre comillas: sin limpiarlo se pueden
// inyectar cabeceras con un salto de línea o cerrar el valor con una comilla.
func TestNombreDeDescargaNoInyectaCabeceras(t *testing.T) {
	cases := map[string]string{
		`a/normal.zip`:                 "normal.zip",
		"a/con\r\nSet-Cookie: x=1.zip": "conSet-Cookie: x=1.zip",
		`a/con"comillas.zip`:           "concomillas.zip",
		`a/con\barra.zip`:              "conbarra.zip",
		"a/acentos-ñá.zip":             "acentos-.zip",
	}

	for key, want := range cases {
		if got := safeFileName(key); got != want {
			t.Errorf("safeFileName(%q) = %q, se esperaba %q", key, got, want)
		}
		if strings.ContainsAny(safeFileName(key), "\r\n\"\\") {
			t.Errorf("safeFileName(%q) dejó caracteres peligrosos", key)
		}
	}
}

func TestNombreDeDescargaConEntradaDegenerada(t *testing.T) {
	for _, key := range []string{"", "a/", "a/..", "a/."} {
		if got := safeFileName(key); got == "" || got == "." || got == ".." {
			t.Errorf("safeFileName(%q) = %q, se esperaba un nombre utilizable", key, got)
		}
	}
}

func TestEnlaceInvalidoDa404(t *testing.T) {
	local, _ := newFileServer(t)
	_ = local.Put(context.Background(), "a/foto.jpg", "image/jpeg", []byte("x"))
	handler := filesHandler(local)

	// Clave real, firma inventada.
	req := httptest.NewRequest("GET", "/files/a/foto.jpg?expires=99999999999&sig=inventada", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, se esperaba 404", rec.Code)
	}
}
