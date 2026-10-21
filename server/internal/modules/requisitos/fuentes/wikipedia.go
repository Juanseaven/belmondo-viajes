// Package fuentes trae los datos de los que salen las reglas: Wikipedia,
// Passport Index y la extracción con Claude.
package fuentes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

// ErrPageNotFound: la nacionalidad no tiene página de requisitos de visa.
var ErrPageNotFound = errors.New("requisitos: la página de Wikipedia no existe")

// Wikipedia lee páginas por la API de MediaWiki, que está pensada para lectura
// automatizada. Pide un User-Agent con un contacto.
type Wikipedia struct {
	BaseURL string
	Contact string
	HTTP    *http.Client
}

func NewWikipedia(contact string) *Wikipedia {
	return &Wikipedia{BaseURL: "https://en.wikipedia.org/w/api.php", Contact: contact,
		HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Revision es una versión de una página.
type Revision struct {
	ID      string
	Content string
	// URL apunta a esa versión exacta, para que la cita se pueda verificar.
	URL string
}

// Revision devuelve la versión de la página vigente en `at`. Con `at` cero,
// la última.
func (w *Wikipedia) Revision(ctx context.Context, title string, at time.Time) (Revision, error) {
	q := url.Values{
		"action": {"query"}, "prop": {"revisions"}, "titles": {title}, "rvprop": {"ids|content"},
		"rvslots": {"main"}, "rvlimit": {"1"}, "format": {"json"}, "formatversion": {"2"},
	}
	if !at.IsZero() {
		q.Set("rvstart", at.UTC().Format(time.RFC3339))
		q.Set("rvdir", "older")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.BaseURL+"?"+q.Encode(), nil)
	if err != nil {
		return Revision{}, err
	}
	req.Header.Set("User-Agent", userAgent(w.Contact))

	resp, err := w.HTTP.Do(req)
	if err != nil {
		return Revision{}, fmt.Errorf("consultando Wikipedia: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Revision{}, fmt.Errorf("consultando Wikipedia: status %d", resp.StatusCode)
	}

	var body struct {
		Query struct {
			Pages []struct {
				Missing   bool `json:"missing"`
				Revisions []struct {
					RevID int64 `json:"revid"`
					Slots struct {
						Main struct {
							Content string `json:"content"`
						} `json:"main"`
					} `json:"slots"`
				} `json:"revisions"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Revision{}, fmt.Errorf("leyendo la respuesta de Wikipedia: %w", err)
	}
	if len(body.Query.Pages) == 0 || body.Query.Pages[0].Missing {
		return Revision{}, ErrPageNotFound
	}
	revs := body.Query.Pages[0].Revisions
	if len(revs) == 0 {
		// La página no existía todavía en esa fecha.
		return Revision{}, ErrPageNotFound
	}
	id := fmt.Sprint(revs[0].RevID)
	return Revision{ID: id, Content: revs[0].Slots.Main.Content,
		URL: "https://en.wikipedia.org/w/index.php?title=" +
			url.QueryEscape(strings.ReplaceAll(title, " ", "_")) + "&oldid=" + id}, nil
}

// Snippet recorta de la página de una nacionalidad lo que importa para un país:
// su fila en la tabla de visas, la sección de documentos que reemplazan al
// pasaporte, la de vigencia del pasaporte y sus filas en las tablas de
// vacunación.
//
// El recorte es lo que se compara para la espera de 72 horas y lo que lee
// Claude: una edición en la fila de otro país no lo cambia.
func Snippet(wikitext string, c domain.Country) (string, bool) {
	row, ok := findRow(rows(section(wikitext, "==Visa requirements==")), c)
	if !ok {
		return "", false
	}

	var b strings.Builder
	b.WriteString("Fila de la tabla de visas:\n")
	b.WriteString(strings.TrimSpace(row))
	if dni := section(wikitext, "===Passport not required==="); dni != "" {
		b.WriteString("\n\nDocumentos aceptados en lugar del pasaporte:\n===Passport not required===\n")
		b.WriteString(strings.TrimSpace(dni))
	}
	// La vigencia no está en la fila: la página la da en una lista general,
	// por meses, con una frase para los países que no están en ninguna.
	if vigencia := section(wikitext, "===Passport validity length==="); vigencia != "" {
		b.WriteString("\n\nVigencia del pasaporte:\n===Passport validity length===\n")
		b.WriteString(strings.TrimSpace(vigencia))
	}
	if vacunas := vaccinationRows(section(wikitext, "==Vaccination requirements=="), c); vacunas != "" {
		b.WriteString("\n\nVacunación:\n")
		b.WriteString(vacunas)
	}
	return b.String(), true
}

// section devuelve el cuerpo de una sección hasta el próximo título del mismo
// nivel o de uno superior.
func section(wikitext, heading string) string {
	start := strings.Index(wikitext, "\n"+heading)
	if start < 0 {
		if !strings.HasPrefix(wikitext, heading) {
			return ""
		}
		start = 0
	} else {
		start++
	}
	body := wikitext[start+len(heading):]
	level := strings.Count(heading, "=") / 2
	offset := 0
	for _, line := range strings.SplitAfter(body, "\n") {
		if l := headingLevel(strings.TrimSpace(line)); l > 0 && l <= level {
			return body[:offset]
		}
		offset += len(line)
	}
	return body
}

func headingLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '=' {
		n++
	}
	if n == 0 || !strings.HasSuffix(line, strings.Repeat("=", n)) {
		return 0
	}
	return n
}

func rows(table string) []string { return strings.Split(table, "\n|-") }

// findRow busca la fila por el código ISO de la plantilla de Timatic que cita
// cada fila y, si no lo tiene, por los nombres del país.
func findRow(rows []string, c domain.Country) (string, bool) {
	for _, row := range rows {
		if strings.Contains(row, "destination="+c.Code+"}}") || strings.Contains(row, "destination="+c.Code+"|") {
			return row, true
		}
	}
	for _, row := range rows {
		if mentions(row, c) {
			return row, true
		}
	}
	return "", false
}

func mentions(text string, c domain.Country) bool {
	for _, name := range c.WikiNames {
		for _, tpl := range []string{"{{flag|", "{{flagu|", "{{flagcountry|"} {
			if strings.Contains(text, tpl+name+"}}") || strings.Contains(text, tpl+name+"|") {
				return true
			}
		}
	}
	return false
}

// vaccinationRows junta, de cada tabla de vacunación, el título, la leyenda
// (que dice de cuándo son los datos) y la fila del país.
func vaccinationRows(sectionText string, c domain.Country) string {
	var b strings.Builder
	for _, sub := range strings.Split(sectionText, "\n===")[1:] {
		heading := "===" + strings.SplitN(sub, "\n", 2)[0]
		var caption string
		for _, line := range strings.Split(sub, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "|+") {
				caption = strings.TrimSpace(line)
				break
			}
		}
		for _, row := range rows(sub) {
			if mentions(row, c) {
				b.WriteString(heading + "\n")
				if caption != "" {
					b.WriteString(caption + "\n")
				}
				b.WriteString(strings.TrimSpace(row) + "\n")
				break
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// userAgent identifica al sistema ante los sitios que consulta, con un
// contacto si está configurado.
func userAgent(contact string) string {
	if contact == "" {
		return "viajes-crm/1.0"
	}
	return "viajes-crm/1.0 (" + contact + ")"
}
