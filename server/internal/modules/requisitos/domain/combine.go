package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Extraction es lo que Claude devuelve al leer Wikipedia: cada dato con la cita
// textual que lo sostiene. No es una regla todavía: FromWikipedia comprueba
// las citas antes de creerle.
type Extraction struct {
	Documento       Extracted[Documento] `json:"documento"`
	Visa            Extracted[Visa]      `json:"visa"`
	EstadiaMaxDias  Extracted[int]       `json:"estadiaMaxDias"`
	Vigencia        Extracted[Vigencia]  `json:"vigencia"`
	EmisionMaxAnios Extracted[int]       `json:"emisionMaxAnios"`
	Exenciones      []ExencionCitada     `json:"exenciones"`
	Vacunas         []VacunaCitada       `json:"vacunas"`
}

type Extracted[T any] struct {
	Value *T     `json:"value"`
	Quote string `json:"quote"`
}

type ExencionCitada struct {
	Exencion
	Quote string `json:"quote"`
}

type VacunaCitada struct {
	Vacuna
	Quote string `json:"quote"`
}

const (
	noLoDice       = "Wikipedia no lo dice"
	citaInventada  = "la cita no aparece en Wikipedia"
	citaNoSostiene = "la cita no sostiene el valor"
)

// FromWikipedia convierte una extracción en campos, creyéndole solo a lo que
// tiene una cita que (1) aparece literalmente en el texto, sin contar espacios
// ni mayúsculas, y (2) dice lo que se extrajo: «no requiere visa» necesita una
// cita con «not required», una estadía de 90 días una cita con el 90, una
// exención una cita que nombre al país del permiso.
//
// El texto sale de una página que cualquiera puede editar, y quien lo lee es un
// modelo: que la cita exista no prueba que sostenga el valor. Cuando una cita
// falla, el campo tira hacia lo más exigente: una exención o un DNI sin
// sostén se descartan; lo demás queda en verificar.
func FromWikipedia(ex Extraction, text string, base Citation) Fields {
	haystack := normalize(text)
	cited := func(quote string) bool {
		q := normalize(quote)
		return q != "" && strings.Contains(haystack, q)
	}
	cite := func(quote string) Citation {
		c := base
		c.Source, c.Quote = SourceWikipedia, quote
		return c
	}
	// check devuelve "" si la cita vale, o el motivo por el que no.
	check := func(quote string, supports bool) string {
		switch {
		case !cited(quote):
			return citaInventada
		case !supports:
			return citaNoSostiene
		}
		return ""
	}

	var f Fields
	if ex.Visa.Value == nil {
		f.Visa = verify[Visa](noLoDice)
	} else if reason := check(ex.Visa.Quote, matches(ex.Visa.Quote, visaKeywords[*ex.Visa.Value])); reason != "" {
		f.Visa = verify[Visa](reason)
	} else {
		f.Visa = known(*ex.Visa.Value, cite(ex.Visa.Quote))
	}

	// Que alcance el DNI facilita el viaje: sin una cita que hable del
	// documento de identidad, no vale.
	if d := ex.Documento.Value; d != nil {
		supports := *d == DocumentoPasaporte || matches(ex.Documento.Quote, idCardKeywords)
		if check(ex.Documento.Quote, supports) == "" {
			f.Documento = known(*d, cite(ex.Documento.Quote))
		}
	}

	if v := ex.EstadiaMaxDias.Value; v == nil {
		f.EstadiaMaxDias = Field[int]{Reason: noLoDice}
	} else if reason := check(ex.EstadiaMaxDias.Quote, hasNumber(ex.EstadiaMaxDias.Quote, *v)); reason != "" {
		f.EstadiaMaxDias = verify[int](reason)
	} else {
		f.EstadiaMaxDias = known(*v, cite(ex.EstadiaMaxDias.Quote))
	}

	if v := ex.Vigencia.Value; v == nil {
		f.Vigencia = verify[Vigencia]("Wikipedia no dice la vigencia del pasaporte")
	} else if reason := check(ex.Vigencia.Quote, hasNumber(ex.Vigencia.Quote, v.Amount) &&
		matches(ex.Vigencia.Quote, unitKeywords[v.Unit])); reason != "" {
		f.Vigencia = verify[Vigencia](reason)
	} else {
		f.Vigencia = known(*v, cite(ex.Vigencia.Quote))
	}

	if v := ex.EmisionMaxAnios.Value; v == nil {
		f.EmisionMaxAnios = Field[int]{}
	} else if reason := check(ex.EmisionMaxAnios.Quote, hasNumber(ex.EmisionMaxAnios.Quote, *v) &&
		matches(ex.EmisionMaxAnios.Quote, unitKeywords["anios"])); reason != "" {
		f.EmisionMaxAnios = verify[int](reason)
	} else {
		f.EmisionMaxAnios = known(*v, cite(ex.EmisionMaxAnios.Quote))
	}

	exenciones := []Exencion{}
	var exSources []Citation
	for _, e := range ex.Exenciones {
		country, ok := CountryByCode(e.PermitCountry)
		if ok && check(e.Quote, namesCountry(e.Quote, country)) == "" {
			exenciones = append(exenciones, e.Exencion)
			exSources = append(exSources, cite(e.Quote))
		}
	}
	f.Exenciones = known(exenciones, exSources...)

	vacunas := []Vacuna{}
	var vacSources []Citation
	for _, v := range ex.Vacunas {
		if reason := check(v.Quote, matches(v.Quote, scopeKeywords[v.Scope])); reason != "" {
			f.Vacunas = verify[[]Vacuna](reason)
			return f
		}
		vacunas = append(vacunas, v.Vacuna)
		vacSources = append(vacSources, cite(v.Quote))
	}
	f.Vacunas = known(vacunas, vacSources...)
	return f
}

// Las palabras que una cita tiene que tener para sostener cada valor. Son
// expresiones regulares sin distinguir mayúsculas.
var visaKeywords = map[Visa]*regexp.Regexp{
	VisaNoRequiere:              regexp.MustCompile(`(?i)not required|visa[- ]free|freedom of movement|free movement|visa waiver`),
	VisaALaLlegada:              regexp.MustCompile(`(?i)on arrival`),
	VisaAutorizacionElectronica: regexp.MustCompile(`(?i)\b(k-)?eta\b|\besta\b|\betias\b|travel authori[sz]ation`),
	VisaElectronica:             regexp.MustCompile(`(?i)\be-?visa\b|online visa|electronic visa`),
	VisaConsular:                regexp.MustCompile(`(?i)visa required`),
	VisaNoAdmitido:              regexp.MustCompile(`(?i)refused|banned|not allowed|no admission|prohibited`),
}

var idCardKeywords = regexp.MustCompile(`(?i)id card|identity card|identity document|national id|documento nacional`)

var unitKeywords = map[string]*regexp.Regexp{
	"meses": regexp.MustCompile(`(?i)month`),
	"dias":  regexp.MustCompile(`(?i)day`),
	"anios": regexp.MustCompile(`(?i)year`),
}

var scopeKeywords = map[string]*regexp.Regexp{
	VacunaParaTodos:       regexp.MustCompile(`(?i)all countries|\byes\b|required`),
	VacunaDesdePaisRiesgo: regexp.MustCompile(`(?i)risk`),
}

func matches(quote string, re *regexp.Regexp) bool { return re != nil && re.MatchString(quote) }

func hasNumber(quote string, n int) bool {
	return regexp.MustCompile(`(^|\D)` + strconv.Itoa(n) + `(\D|$)`).MatchString(quote)
}

// namesCountry dice si la cita nombra al país: por alguno de sus nombres en
// Wikipedia, sin distinguir mayúsculas, o por su código ISO tal cual («US»).
func namesCountry(quote string, c Country) bool {
	lower := strings.ToLower(quote)
	for _, name := range c.WikiNames {
		if strings.Contains(lower, strings.ToLower(name)) {
			return true
		}
	}
	return regexp.MustCompile(`\b` + c.Code + `\b`).MatchString(quote)
}

func normalize(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// PassportIndexEntry es una fila de la matriz de Passport Index.
type PassportIndexEntry struct {
	Requirement string
	// DataDate es la fecha de los datos de la matriz.
	DataDate string
}

const passportIndexURL = "https://github.com/imorte/passport-index-data"

// Visa traduce el valor de la matriz. Los números son días sin visa.
func (e PassportIndexEntry) Visa() (visa Visa, days int, ok bool) {
	if n, err := strconv.Atoi(e.Requirement); err == nil {
		if n < 0 {
			return "", 0, false
		}
		return VisaNoRequiere, n, true
	}
	switch e.Requirement {
	case "visa free":
		return VisaNoRequiere, 0, true
	case "visa on arrival":
		return VisaALaLlegada, 0, true
	case "eta":
		return VisaAutorizacionElectronica, 0, true
	case "e-visa":
		return VisaElectronica, 0, true
	case "visa required":
		return VisaConsular, 0, true
	case "no admission":
		return VisaNoAdmitido, 0, true
	}
	return "", 0, false
}

// Conflict es una diferencia entre fuentes. Se registra para poder decidir si
// apagar Passport Index cuando sus datos queden viejos.
type Conflict struct {
	Pair
	Field         string `json:"field"`
	Wikipedia     string `json:"wikipedia"`
	PassportIndex string `json:"passportIndex"`
	DataDate      string `json:"dataDate"`
}

// Combine junta Wikipedia, Passport Index y las constantes en una regla.
//
// Wikipedia sola alcanza: que Passport Index no tenga la combinación no es un
// conflicto. Conflicto es cuando las dos dicen algo distinto sobre la visa, y
// entonces el campo queda en verificar. En estadía gana la más exigente. Las
// constantes ganan sobre las fuentes en sus campos.
func Combine(p Pair, w Fields, pi *PassportIndexEntry) (Fields, []Conflict) {
	f := w
	var conflicts []Conflict

	if pi != nil {
		if piVisa, days, ok := pi.Visa(); ok {
			piCite := Citation{Source: SourcePassportIndex, Quote: pi.Requirement, URL: passportIndexURL, Revision: pi.DataDate}
			switch {
			case f.Visa.Value == nil:
				f.Visa = known(piVisa, piCite)
			case *f.Visa.Value != piVisa:
				conflicts = append(conflicts, Conflict{Pair: p, Field: "visa",
					Wikipedia: string(*f.Visa.Value), PassportIndex: pi.Requirement, DataDate: pi.DataDate})
				f.Visa = verify[Visa](fmt.Sprintf("las fuentes no coinciden: Wikipedia dice %s y Passport Index (%s) dice %s",
					*f.Visa.Value, pi.DataDate, pi.Requirement), append(f.Visa.Sources, piCite)...)
			default:
				f.Visa.Sources = append(f.Visa.Sources, piCite)
			}

			if days > 0 {
				switch {
				case f.EstadiaMaxDias.Value == nil && !f.EstadiaMaxDias.Verify:
					f.EstadiaMaxDias = known(days, piCite)
				case f.EstadiaMaxDias.Value != nil && *f.EstadiaMaxDias.Value != days:
					wikiDays := *f.EstadiaMaxDias.Value
					f.EstadiaMaxDias = known(min(wikiDays, days), append(f.EstadiaMaxDias.Sources, piCite)...)
					f.EstadiaMaxDias.Reason = fmt.Sprintf("Wikipedia dice %d días y Passport Index %d; se toma el menor", wikiDays, days)
				case f.EstadiaMaxDias.Value != nil:
					f.EstadiaMaxDias.Sources = append(f.EstadiaMaxDias.Sources, piCite)
				}
			}
		}
	}

	if p.Nationality == "AR" && dniArgentino[p.Country] {
		f.Documento = known(DocumentoDNIOPasaporte, dniArgentinoCite)
	} else if f.Documento.Value == nil {
		f.Documento = known(DocumentoPasaporte)
		f.Documento.Reason = "ninguna fuente dice que alcance otro documento"
	}

	if InSchengen(p.Country) && !FreeMovementInSchengen(p.Nationality) {
		f.Vigencia = known(Vigencia{Amount: 3, Unit: "meses", Base: "salida_del_pais"}, schengenCite)
		f.EmisionMaxAnios = known(10, schengenCite)
	}
	return f, conflicts
}

// Las constantes: reglas que ninguna fuente cubre bien y que casi no cambian.
// Viajan con el sistema, cada una con su fuente.

// Con DNI argentino vigente alcanza en los países del Mercosur y asociados que
// lo aceptan. Venezuela y Surinam quedan afuera a propósito: pedir pasaporte es
// lo más exigente y nunca deja a nadie varado.
var dniArgentino = set("BR", "PY", "UY", "BO", "CL", "CO", "EC", "PE")

var dniArgentinoCite = Citation{Source: SourceConstante,
	Quote: "Documentos de viaje del MERCOSUR: el DNI argentino vigente alcanza para viajar a los países del bloque y asociados",
	URL:   "https://www.argentina.gob.ar/interior/migraciones/documentos-de-viaje-del-mercosur"}

// Código de Fronteras Schengen, Reglamento (UE) 2016/399, artículo 6.1.a.
var schengenCite = Citation{Source: SourceConstante,
	Quote: "valid for at least three months after the intended date of departure from the territory of the Member States […] issued within the previous 10 years",
	URL:   "https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX:32016R0399"}
