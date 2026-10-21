package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
)

// ResearchOutput es lo que devuelve Claude después de buscar en las páginas
// oficiales: frases con su URL y su cita, y lo que dicen las páginas de cuatro
// valores. Todavía no se comprobó nada: CheckResearch decide qué creerle.
type ResearchOutput struct {
	Grupos  []GrupoBuscado `json:"grupos"`
	Control ControlBuscado `json:"control"`
}

type GrupoBuscado struct {
	Grupo   string         `json:"grupo"`
	Pide    []FraseBuscada `json:"pide"`
	Tramite []FraseBuscada `json:"tramite"`
}

type FraseBuscada struct {
	Text  string `json:"text"`
	URL   string `json:"url"`
	Quote string `json:"quote"`
}

// ControlBuscado es lo que las páginas dicen de los valores que se comparan
// con la regla. Nil: las páginas no lo dicen.
type ControlBuscado struct {
	Visa           *Buscado[Visa]      `json:"visa"`
	Documento      *Buscado[Documento] `json:"documento"`
	Vigencia       *Buscado[Vigencia]  `json:"vigencia"`
	EstadiaMaxDias *Buscado[int]       `json:"estadiaMaxDias"`
}

type Buscado[T any] struct {
	Value *T     `json:"value"`
	URL   string `json:"url"`
	Quote string `json:"quote"`
}

// Research es la búsqueda comprobada: solo lo que tiene una cita que está en
// la página. Seen es la huella de los campos de la regla contra los que se
// hizo, por grupo: si después cambia un campo, lo que la búsqueda dijo de él
// deja de valer.
type Research struct {
	ReadAt  string            `json:"readAt"`
	Grupos  map[string]Grupo  `json:"grupos"`
	Control ResearchControl   `json:"control"`
	Seen    map[string]string `json:"seen"`
}

type ResearchControl struct {
	Visa           *Controlled[Visa]      `json:"visa,omitempty"`
	Documento      *Controlled[Documento] `json:"documento,omitempty"`
	Vigencia       *Controlled[Vigencia]  `json:"vigencia,omitempty"`
	EstadiaMaxDias *Controlled[int]       `json:"estadiaMaxDias,omitempty"`
}

type Controlled[T any] struct {
	Value  T        `json:"value"`
	Source Citation `json:"source"`
}

// Empty dice si la búsqueda no dejó nada que mostrar ni comparar.
func (r Research) Empty() bool {
	c := r.Control
	return len(r.Grupos) == 0 && c.Visa == nil && c.Documento == nil && c.Vigencia == nil && c.EstadiaMaxDias == nil
}

var (
	scriptOrStyle = regexp.MustCompile(`(?is)<(script|style|noscript|template)\b.*?</(script|style|noscript|template)\s*>`)
	htmlComment   = regexp.MustCompile(`(?s)<!--.*?-->`)
	htmlTag       = regexp.MustCompile(`(?s)<[^>]*>`)
	digits        = regexp.MustCompile(`\d+`)
	// Un enlace de markdown, «[texto](url "título")»: queda el texto.
	markdownLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
)

// PageText es el texto que se ve de una página: sin scripts, estilos ni
// etiquetas, con las entidades resueltas. Contra esto se comprueban las citas.
func PageText(page string) string {
	s := scriptOrStyle.ReplaceAllString(page, " ")
	s = htmlComment.ReplaceAllString(s, " ")
	s = htmlTag.ReplaceAllString(s, " ")
	return html.UnescapeString(s)
}

// normalizeQuote compara sin espacios, mayúsculas, comillas tipográficas ni
// marcas de markdown: el texto de la herramienta de lectura viene en markdown,
// y una cita puede copiar las marcas o no.
func normalizeQuote(s string) string {
	s = markdownLink.ReplaceAllString(s, "$1")
	s = strings.NewReplacer(" ", " ", "’", "'", "‘", "'", "“", `"`, "”", `"`,
		"–", "-", "—", "-", "**", "", "__", "", "*", "", "`", "").Replace(s)
	return normalize(s)
}

// CheckResearch le cree a la búsqueda solo lo que se puede comprobar:
//
//   - la cita aparece en el texto de la página citada (pages tiene solo las
//     páginas que se leyeron en esa búsqueda, así que una URL inventada o no
//     leída se descarta);
//   - cada número de la frase está en su cita, para que una cifra mal
//     traducida no llegue a la ficha;
//   - la frase es una frase: sin enlaces y corta.
//
// Lo que no pasa se descarta: el renglón queda con menos detalle, nunca con un
// detalle inventado.
func CheckResearch(out ResearchOutput, pages map[string]string, readAt string) Research {
	haystacks := make(map[string]string, len(pages))
	for u, text := range pages {
		haystacks[u] = normalizeQuote(text)
	}
	cite := func(u, quote string) (Citation, bool) {
		page, ok := haystacks[u]
		q := normalizeQuote(quote)
		if !ok || q == "" || !strings.Contains(page, q) {
			return Citation{}, false
		}
		return Citation{Source: SourceOficial, URL: u, Quote: strings.TrimSpace(quote), ReadAt: readAt}, true
	}
	frases := func(in []FraseBuscada) []Frase {
		var out []Frase
		for _, f := range in {
			if !plainSentence(f.Text) || !numbersIn(f.Text, f.Quote) {
				continue
			}
			if c, ok := cite(f.URL, f.Quote); ok {
				out = append(out, Frase{Text: strings.TrimSpace(f.Text), Source: c})
			}
		}
		return out
	}

	r := Research{ReadAt: readAt, Grupos: map[string]Grupo{}}
	for _, g := range out.Grupos {
		if !knownGroup(g.Grupo) {
			continue
		}
		grupo := Grupo{Pide: frases(g.Pide), Tramite: frases(g.Tramite)}
		if len(grupo.Pide)+len(grupo.Tramite) == 0 {
			continue
		}
		prev := r.Grupos[g.Grupo]
		prev.Pide, prev.Tramite = append(prev.Pide, grupo.Pide...), append(prev.Tramite, grupo.Tramite...)
		r.Grupos[g.Grupo] = prev
	}

	c := out.Control
	if b := c.Visa; b != nil && b.Value != nil {
		if src, ok := cite(b.URL, b.Quote); ok {
			r.Control.Visa = &Controlled[Visa]{Value: *b.Value, Source: src}
		}
	}
	if b := c.Documento; b != nil && b.Value != nil {
		if src, ok := cite(b.URL, b.Quote); ok {
			r.Control.Documento = &Controlled[Documento]{Value: *b.Value, Source: src}
		}
	}
	if b := c.Vigencia; b != nil && b.Value != nil && (b.Value.Amount == 0 || hasNumber(b.Quote, b.Value.Amount)) {
		if src, ok := cite(b.URL, b.Quote); ok {
			r.Control.Vigencia = &Controlled[Vigencia]{Value: *b.Value, Source: src}
		}
	}
	if b := c.EstadiaMaxDias; b != nil && b.Value != nil && hasNumber(b.Quote, *b.Value) {
		if src, ok := cite(b.URL, b.Quote); ok {
			r.Control.EstadiaMaxDias = &Controlled[int]{Value: *b.Value, Source: src}
		}
	}
	return r
}

func knownGroup(g string) bool {
	switch g {
	case GrupoDocumento, GrupoVisa, GrupoVigencia, GrupoAntiguedad, GrupoEstadia, GrupoVacunas, GrupoTransito:
		return true
	}
	return strings.HasPrefix(g, CondicionPrefix)
}

// plainSentence: la frase la lee la agencia y puede ir al cliente; no trae
// enlaces ni es un párrafo.
func plainSentence(s string) bool {
	lower := strings.ToLower(s)
	return strings.TrimSpace(s) != "" && len([]rune(s)) <= 400 &&
		!strings.Contains(lower, "http") && !strings.Contains(lower, "www.") && !strings.Contains(s, "@")
}

// numbersIn dice si cada número de la frase está en la cita.
func numbersIn(text, quote string) bool {
	have := map[string]bool{}
	for _, n := range digits.FindAllString(quote, -1) {
		have[strings.TrimLeft(n, "0")] = true
	}
	for _, n := range digits.FindAllString(text, -1) {
		if !have[strings.TrimLeft(n, "0")] {
			return false
		}
	}
	return true
}

// NeedsResearch dice si vale la pena buscar el detalle en las páginas
// oficiales: solo donde hay un trámite que explicar (una visa, una
// autorización electrónica, una visa a la llegada). Donde no hace falta visa,
// o no se sabe, alcanza con lo que arma el sistema, y la búsqueda cuesta.
func NeedsResearch(f Fields) bool {
	if f.Visa.Verify || f.Visa.Value == nil {
		return false
	}
	switch *f.Visa.Value {
	case VisaALaLlegada, VisaAutorizacionElectronica, VisaElectronica, VisaConsular:
		return true
	}
	return false
}

// FieldPrints es la huella de cada grupo: los valores de la regla que la
// búsqueda explica. Si cambia la huella de un grupo, su detalle y su control
// dejan de valer.
func FieldPrints(f Fields) map[string]string {
	out := map[string]string{
		GrupoDocumento:  fingerprint(f.Documento.Value, f.Documento.Verify),
		GrupoVisa:       fingerprint(f.Visa.Value, f.Visa.Verify, f.Exenciones.Value),
		GrupoVigencia:   fingerprint(f.Vigencia.Value, f.Vigencia.Verify),
		GrupoAntiguedad: fingerprint(f.EmisionMaxAnios.Value, f.EmisionMaxAnios.Verify),
		GrupoEstadia:    fingerprint(f.EstadiaMaxDias.Value, f.EstadiaMaxDias.Verify),
		GrupoVacunas:    fingerprint(f.Vacunas.Value, f.Vacunas.Verify),
		GrupoTransito:   fingerprint(f.Visa.Value, f.Visa.Verify),
	}
	if f.Condiciones.Value != nil {
		for i, key := range CondicionKeys(*f.Condiciones.Value) {
			out[key] = fingerprint((*f.Condiciones.Value)[i])
		}
	}
	return out
}

func fingerprint(parts ...any) string {
	raw, _ := json.Marshal(parts)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

// ApplyResearch cruza la búsqueda con la regla ya combinada.
//
// El detalle se usa solo en los grupos cuyos campos no cambiaron desde la
// búsqueda; si alguno cambió, stale avisa que hay que buscar de nuevo.
//
// El control nunca afloja la regla:
//   - coincide, o la búsqueda no dice nada: la regla queda igual y suma la cita;
//   - no coincide con un valor que tiene fuente: «verificar», y el conflicto se
//     registra;
//   - contra un valor por defecto, sin fuente, solo endurece: si la búsqueda
//     es más exigente, «verificar»; si es más permisiva, se ignora;
//   - un campo en «verificar» sigue así, y las constantes no se tocan.
func ApplyResearch(p Pair, base Fields, r *Research) (Fields, map[string]Grupo, []Conflict, bool) {
	if r == nil {
		return base, nil, nil, false
	}
	prints := FieldPrints(base)
	fresh := func(group string) bool { return r.Seen[group] != "" && r.Seen[group] == prints[group] }
	stale := false
	grupos := map[string]Grupo{}
	for group, g := range r.Grupos {
		if fresh(group) {
			grupos[group] = g
		} else {
			stale = true
		}
	}
	for _, group := range []string{GrupoVisa, GrupoDocumento, GrupoVigencia, GrupoEstadia} {
		if r.Seen[group] != prints[group] {
			stale = true
		}
	}

	f := base
	var conflicts []Conflict
	add := func(c *Conflict) {
		if c != nil {
			c.Pair = p
			conflicts = append(conflicts, *c)
		}
	}
	if c := r.Control.Visa; c != nil && fresh(GrupoVisa) {
		var conflict *Conflict
		f.Visa, conflict = control(f.Visa, c, "visa", func(a, b Visa) bool { return visaRank[a] > visaRank[b] })
		add(conflict)
	}
	if c := r.Control.Documento; c != nil && fresh(GrupoDocumento) {
		var conflict *Conflict
		f.Documento, conflict = control(f.Documento, c, "documento", func(a, b Documento) bool {
			return a == DocumentoPasaporte && b != DocumentoPasaporte
		})
		add(conflict)
	}
	if c := r.Control.Vigencia; c != nil && fresh(GrupoVigencia) {
		var conflict *Conflict
		f.Vigencia, conflict = control(f.Vigencia, c, "vigencia", func(a, b Vigencia) bool { return vigenciaDays(a) > vigenciaDays(b) })
		add(conflict)
	}
	if c := r.Control.EstadiaMaxDias; c != nil && fresh(GrupoEstadia) {
		var conflict *Conflict
		f.EstadiaMaxDias, conflict = control(f.EstadiaMaxDias, c, "estadia", func(a, b int) bool { return a < b })
		add(conflict)
	}
	return f, grupos, conflicts, stale
}

// visaRank ordena las categorías de la menos a la más exigente.
var visaRank = map[Visa]int{VisaNoRequiere: 0, VisaALaLlegada: 1, VisaAutorizacionElectronica: 2,
	VisaElectronica: 3, VisaConsular: 4, VisaNoAdmitido: 5}

// vigenciaDays ordena las vigencias: más días es más exigente, y contar desde
// la salida es más exigente que desde la llegada.
func vigenciaDays(v Vigencia) int {
	days := v.Amount
	if v.Unit == "meses" {
		days *= 30
	}
	days *= 2
	if v.Base == "salida_del_pais" {
		days++
	}
	return days
}

// control compara un valor de la búsqueda con el campo. stricter(a, b) dice si
// a es más exigente que b.
func control[T comparable](f Field[T], c *Controlled[T], name string, stricter func(a, b T) bool) (Field[T], *Conflict) {
	if f.Verify || hasConstant(f.Sources) {
		return f, nil
	}
	official := fmt.Sprintf("la página oficial (%s)", host(c.Source.URL))
	switch {
	case f.Value == nil:
		// Sin valor ni «verificar»: ninguna fuente lo exige. Que la página lo
		// exija es más exigente.
		return verify[T](fmt.Sprintf("solo %s lo dice: %v", official, c.Value), c.Source), nil
	case *f.Value == c.Value:
		f.Sources = append(f.Sources, c.Source)
		return f, nil
	case len(f.Sources) == 0:
		if !stricter(c.Value, *f.Value) {
			return f, nil
		}
		return verify[T](fmt.Sprintf("se tomaba %v por defecto y %s dice %v", *f.Value, official, c.Value),
			c.Source), nil
	}
	conflict := &Conflict{Field: name, Wikipedia: fmt.Sprint(*f.Value), Other: fmt.Sprint(c.Value),
		Source: SourceOficial, DataDate: c.Source.ReadAt}
	return verify[T](fmt.Sprintf("las fuentes no coinciden: las demás dicen %v y %s dice %v", *f.Value, official, c.Value),
		append(f.Sources, c.Source)...), conflict
}

func hasConstant(sources []Citation) bool {
	for _, s := range sources {
		if s.Source == SourceConstante {
			return true
		}
	}
	return false
}

func host(u string) string {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" {
		return u
	}
	return strings.TrimPrefix(parsed.Host, "www.")
}
