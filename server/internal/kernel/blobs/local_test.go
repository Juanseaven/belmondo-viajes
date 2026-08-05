package blobs

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func newLocal(t *testing.T) *Local {
	t.Helper()
	l, err := NewLocal(t.TempDir(), "http://localhost:8080", testSecret)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	return l
}

// parseSigned extrae clave, expiración y firma de la URL emitida, que es lo que
// después llega por la ruta de archivos.
func parseSigned(t *testing.T, signed string) (key, expires, sig string) {
	t.Helper()
	u, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("URL ilegible: %v", err)
	}
	key = strings.TrimPrefix(u.Path, "/files/")
	return key, u.Query().Get("expires"), u.Query().Get("sig")
}

func TestGuardaYDevuelveElArchivo(t *testing.T) {
	l := newLocal(t)
	ctx := context.Background()
	contenido := []byte("contenido del pasaporte")

	if err := l.Put(ctx, "ana/2026-08/msg-1.jpg", "image/jpeg", contenido); err != nil {
		t.Fatalf("Put: %v", err)
	}

	signed, err := l.SignedURL(ctx, "ana/2026-08/msg-1.jpg", time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}

	got, err := l.Open(parseSigned(t, signed))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if string(got) != string(contenido) {
		t.Fatalf("contenido = %q, se esperaba %q", got, contenido)
	}
}

func TestPutCreaLosDirectoriosIntermedios(t *testing.T) {
	l := newLocal(t)

	if err := l.Put(context.Background(), "a/b/c/d.bin", "application/octet-stream", []byte("x")); err != nil {
		t.Fatalf("Put con ruta anidada: %v", err)
	}
	if _, err := os.Stat(filepath.Join(l.dir, "a", "b", "c", "d.bin")); err != nil {
		t.Fatalf("el archivo no quedó en disco: %v", err)
	}
}

// La propiedad que hace segura esta implementación: sin la firma correcta no se
// sirve nada, aunque se sepa la clave.
func TestOpenRechazaFirmasInvalidas(t *testing.T) {
	l := newLocal(t)
	ctx := context.Background()
	_ = l.Put(ctx, "secreto.jpg", "image/jpeg", []byte("x"))

	signed, err := l.SignedURL(ctx, "secreto.jpg", time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	key, expires, sig := parseSigned(t, signed)

	cases := []struct {
		name                    string
		key, expires, signature string
	}{
		{"sin firma", key, expires, ""},
		{"firma alterada", key, expires, sig[:len(sig)-2] + "aa"},
		{"otra clave con la misma firma", "otro.jpg", expires, sig},
		{"expiración estirada", key, "99999999999", sig},
		{"expiración no numérica", key, "manana", sig},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := l.Open(c.key, c.expires, c.signature); !errors.Is(err, ErrNotAuthorized) {
				t.Fatalf("Open(%s) = %v, se esperaba ErrNotAuthorized", c.name, err)
			}
		})
	}
}

// Una URL reenviada por error tiene que dejar de servir sola.
func TestOpenRechazaEnlacesCaducados(t *testing.T) {
	l := newLocal(t)
	ctx := context.Background()
	_ = l.Put(ctx, "x.jpg", "image/jpeg", []byte("x"))

	signed, err := l.SignedURL(ctx, "x.jpg", -time.Minute) // nace caducado
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}

	if _, err := l.Open(parseSigned(t, signed)); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("se sirvió un enlace caducado: %v", err)
	}
}

// El fallo clásico: la clave viene de la URL, así que sin comprobarla se puede
// pedir cualquier archivo de la máquina.
func TestNoSeEscapaDelDirectorio(t *testing.T) {
	l := newLocal(t)

	// Un archivo fuera del directorio, que es lo que un atacante querría leer.
	fuera := filepath.Join(filepath.Dir(l.dir), "secreto.txt")
	if err := os.WriteFile(fuera, []byte("no deberías ver esto"), 0o644); err != nil {
		t.Fatalf("preparando el archivo externo: %v", err)
	}

	claves := []string{
		"../secreto.txt",
		"../../secreto.txt",
		"a/../../secreto.txt",
		"%2e%2e/secreto.txt", // escapado, para esquivar un filtro ingenuo
		"/etc/passwd",
		"",
	}

	for _, key := range claves {
		t.Run(key, func(t *testing.T) {
			if err := l.Put(context.Background(), key, "text/plain", []byte("x")); err == nil {
				t.Fatalf("Put aceptó la clave %q", key)
			}
			if _, err := l.SignedURL(context.Background(), key, time.Minute); err == nil {
				t.Fatalf("SignedURL aceptó la clave %q", key)
			}
		})
	}
}

func TestNewLocalExigeUnSecretoDecente(t *testing.T) {
	if _, err := NewLocal(t.TempDir(), "http://x", "corto"); err == nil {
		t.Fatal("se aceptó un secreto de firma de 5 caracteres")
	}
}

func TestOpenDeUnaClaveInexistente(t *testing.T) {
	l := newLocal(t)

	signed, err := l.SignedURL(context.Background(), "no-existe.jpg", time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	if _, err := l.Open(parseSigned(t, signed)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, se esperaba ErrNotFound", err)
	}
}
