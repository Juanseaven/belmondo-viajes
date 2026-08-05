package store

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSnippet(t *testing.T) {
	cases := []struct {
		name string
		body string
		term string
		want string
	}{
		{
			name: "cabe entero, se devuelve tal cual",
			body: "Hola, tu reserva es ABC123",
			term: "ABC123",
			want: "Hola, tu reserva es ABC123",
		},
		{
			name: "recorta por ambos lados",
			body: strings.Repeat("x", 100) + "ABC123" + strings.Repeat("y", 100),
			term: "ABC123",
			want: "…" + strings.Repeat("x", 40) + "ABC123" + strings.Repeat("y", 40) + "…",
		},
		{
			name: "match al principio: sin elipsis inicial",
			body: "ABC123" + strings.Repeat("y", 200),
			term: "ABC123",
			want: "ABC123" + strings.Repeat("y", 40) + "…",
		},
		{
			name: "match al final: sin elipsis final",
			body: strings.Repeat("x", 200) + "ABC123",
			term: "ABC123",
			want: "…" + strings.Repeat("x", 40) + "ABC123",
		},
		{
			name: "sin distinguir mayúsculas",
			body: strings.Repeat("x", 100) + "abc123" + strings.Repeat("y", 100),
			term: "ABC123",
			want: "…" + strings.Repeat("x", 40) + "abc123" + strings.Repeat("y", 40) + "…",
		},
		{
			name: "cuerpo vacío",
			body: "",
			term: "abc",
			want: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Snippet(c.body, c.term); got != c.want {
				t.Fatalf("Snippet()\n  got  = %q\n  want = %q", got, c.want)
			}
		})
	}
}

// El corte tiene que respetar los límites de runa: cortando por bytes, un texto
// con acentos o emoji se parte por la mitad y sale UTF-8 inválido.
func TestSnippetNoRompeCaracteresMultibyte(t *testing.T) {
	cases := []string{
		strings.Repeat("ñ", 100) + "reserva" + strings.Repeat("á", 100),
		strings.Repeat("🛫", 100) + "reserva" + strings.Repeat("🏖", 100),
		strings.Repeat("日", 100) + "reserva" + strings.Repeat("本", 100),
	}

	for _, body := range cases {
		got := Snippet(body, "reserva")

		if !utf8.ValidString(got) {
			t.Fatalf("el fragmento no es UTF-8 válido: %q", got)
		}
		if !strings.Contains(got, "reserva") {
			t.Fatalf("el fragmento perdió la coincidencia: %q", got)
		}
		if strings.ContainsRune(got, utf8.RuneError) {
			t.Fatalf("el fragmento contiene runas rotas: %q", got)
		}
	}
}

// El contexto se mide en caracteres, no en bytes: un fragmento de texto en
// japonés debe tener la misma longitud percibida que uno en español.
func TestSnippetMideEnCaracteres(t *testing.T) {
	body := strings.Repeat("日", 200) + "reserva" + strings.Repeat("本", 200)

	got := []rune(Snippet(body, "reserva"))

	// 40 de contexto + 7 del término + 40 + las dos elipsis.
	want := 40 + len([]rune("reserva")) + 40 + 2
	if len(got) != want {
		t.Fatalf("longitud = %d runas, se esperaba %d", len(got), want)
	}
}

func TestSnippetTerminoAusente(t *testing.T) {
	body := strings.Repeat("x", 200)

	got := Snippet(body, "noestá")

	// No puede devolver vacío: el usuario tiene que ver algo del mensaje aunque
	// el resaltado falle.
	if got == "" {
		t.Fatal("fragmento vacío cuando el término no aparece")
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("se esperaba un fragmento truncado, se obtuvo %q", got)
	}
}
