package store

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCursorDaLaVuelta(t *testing.T) {
	// Con nanosegundos: el cursor debe conservarlos, porque dos mensajes del mismo
	// milisegundo se distinguen por ahí antes que por el id.
	original := Cursor{
		At: time.Date(2026, 8, 4, 15, 4, 5, 123456789, time.UTC),
		ID: "8f14e45f-ceea-467a-9575-1e1e6b46f14e",
	}

	got, err := ParseCursor(original.Encode())
	if err != nil {
		t.Fatalf("ParseCursor: %v", err)
	}
	if !got.At.Equal(original.At) {
		t.Fatalf("At = %v, se esperaba %v", got.At, original.At)
	}
	if got.ID != original.ID {
		t.Fatalf("ID = %q, se esperaba %q", got.ID, original.ID)
	}
}

func TestCursorNormalizaAUTC(t *testing.T) {
	bogota := time.FixedZone("America/Bogota", -5*60*60)
	original := Cursor{At: time.Date(2026, 8, 4, 10, 0, 0, 0, bogota), ID: "x"}

	got, err := ParseCursor(original.Encode())
	if err != nil {
		t.Fatalf("ParseCursor: %v", err)
	}
	// Mismo instante, expresado en UTC: la paginación compara instantes, no
	// relojes de pared.
	if !got.At.Equal(original.At) {
		t.Fatalf("At = %v, se esperaba el mismo instante que %v", got.At, original.At)
	}
	if got.At.Location() != time.UTC {
		t.Fatalf("zona = %v, se esperaba UTC", got.At.Location())
	}
}

func TestParseCursorVacioEsPrimeraPagina(t *testing.T) {
	got, err := ParseCursor("")
	if err != nil {
		t.Fatalf("ParseCursor(\"\") devolvió error: %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("cursor = %v, se esperaba el cero", got)
	}
}

func TestParseCursorRechazaBasura(t *testing.T) {
	valido := Cursor{At: time.Now(), ID: "abc"}.Encode()

	cases := []struct {
		name  string
		token string
	}{
		{"no es base64", "!!!no-base64!!!"},
		{"base64 sin separador", encodeRaw("1754320000000000000")},
		{"base64 con id vacío", encodeRaw("1754320000000000000|")},
		{"instante no numérico", encodeRaw("ayer|abc")},
		{"instante desbordado", encodeRaw("99999999999999999999999|abc")},
		{"truncado", valido[:len(valido)/2]},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParseCursor(c.token); !errors.Is(err, ErrBadCursor) {
				t.Fatalf("ParseCursor(%q) = %v, se esperaba ErrBadCursor", c.token, err)
			}
		})
	}
}

// El cursor es opaco a propósito: si se filtrara el instante en claro, un cliente
// podría empezar a construirlos y cambiar el criterio de orden rompería a todos.
func TestCursorNoRevelaSuContenido(t *testing.T) {
	token := Cursor{At: time.Now(), ID: "8f14e45f-ceea-467a"}.Encode()

	if strings.Contains(token, "8f14e45f") || strings.Contains(token, "|") {
		t.Fatalf("el cursor filtra su contenido en claro: %s", token)
	}
}

func TestNewPage(t *testing.T) {
	cursorOf := func(n int) Cursor {
		return Cursor{At: time.Unix(int64(n), 0), ID: string(rune('a' + n))}
	}

	t.Run("hay más páginas", func(t *testing.T) {
		// 4 filas para un límite de 3: la extra revela que hay continuación.
		page := NewPage([]int{0, 1, 2, 3}, 3, cursorOf)

		if len(page.Items) != 3 {
			t.Fatalf("items = %d, se esperaba 3 (la fila sonda debe descartarse)", len(page.Items))
		}
		if page.NextCursor == "" {
			t.Fatal("NextCursor vacío: se perdió la continuación")
		}
		// El cursor apunta a la ÚLTIMA fila devuelta, no a la sonda: si apuntara a
		// la sonda, la página siguiente se saltaría una fila.
		want := cursorOf(2).Encode()
		if page.NextCursor != want {
			t.Fatalf("NextCursor apunta a la fila equivocada")
		}
	})

	t.Run("última página", func(t *testing.T) {
		page := NewPage([]int{0, 1}, 3, cursorOf)

		if len(page.Items) != 2 {
			t.Fatalf("items = %d, se esperaba 2", len(page.Items))
		}
		if page.NextCursor != "" {
			t.Fatalf("NextCursor = %q, se esperaba vacío en la última página", page.NextCursor)
		}
	})

	t.Run("página exacta", func(t *testing.T) {
		// Justo `limit` filas: no hay sonda, así que no hay más.
		page := NewPage([]int{0, 1, 2}, 3, cursorOf)

		if page.NextCursor != "" {
			t.Fatal("NextCursor no vacío con exactamente `limit` filas: pediría una página vacía")
		}
	})

	t.Run("sin resultados", func(t *testing.T) {
		page := NewPage([]int{}, 3, cursorOf)

		if len(page.Items) != 0 || page.NextCursor != "" {
			t.Fatalf("página = %+v, se esperaba vacía", page)
		}
	})
}

func TestClampLimit(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"", 30},        // ausente
		{"abc", 30},     // no numérico
		{"0", 30},       // fuera de rango por abajo
		{"-5", 30},      // negativo
		{"50", 50},      // válido
		{"100", 100},    // el máximo exacto
		{"100000", 100}, // el techo que evita el DoS de una sola petición
	}

	for _, c := range cases {
		if got := ClampLimit(c.raw, 30, 100); got != c.want {
			t.Fatalf("ClampLimit(%q) = %d, se esperaba %d", c.raw, got, c.want)
		}
	}
}

// encodeRaw construye un token con contenido arbitrario, para probar el parseo
// con cargas que Encode nunca produciría.
func encodeRaw(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}
