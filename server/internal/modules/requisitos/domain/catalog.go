package domain

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
	"golang.org/x/text/unicode/norm"
)

// El catálogo de países sale de Wikidata (CC0) y se guarda en catalog_data.go.
// Se generó con dos consultas SPARQL:
//
//   - las páginas de Wikipedia en inglés que son instancia de Q28739943
//     («visa requirements for citizens of …») con su público (P2360) y el ISO
//     alfa-2 (P297) de ese país;
//   - los países con ISO alfa-2 (P297) y sin fecha de disolución (P576), con el
//     título de su artículo en Wikipedia en inglés.
//
// Los nombres en español salen de CLDR (golang.org/x/text), no del archivo.

type countryData struct {
	// visaPage es el título de la página «Visa requirements for X citizens».
	visaPage string
	// wikiNames son los nombres con que las tablas de Wikipedia pueden
	// referirse al país ({{flag|…}}), del más probable al menos.
	wikiNames []string
}

// Country es un país del catálogo.
type Country struct {
	Code      string   `json:"code"`
	Name      string   `json:"name"`
	VisaPage  string   `json:"-"`
	WikiNames []string `json:"-"`
}

var spanish = display.Spanish.Regions()

func CountryByCode(code string) (Country, bool) {
	data, ok := catalogData[code]
	if !ok {
		return Country{}, false
	}
	return Country{Code: code, Name: spanishName(code), VisaPage: data.visaPage, WikiNames: data.wikiNames}, true
}

func spanishName(code string) string {
	region, err := language.ParseRegion(code)
	if err != nil {
		return code
	}
	if name := spanish.Name(region); name != "" {
		return name
	}
	return code
}

// Countries devuelve el catálogo ordenado por nombre en español, sin
// distinguir tildes ni mayúsculas.
func Countries() []Country {
	out := make([]Country, 0, len(catalogData))
	for code := range catalogData {
		c, _ := CountryByCode(code)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return collate(out[i].Name) < collate(out[j].Name) })
	return out
}

// collate es la clave de orden: sin tildes y en minúsculas, para que «Árabe»
// no quede después de «Zambia».
func collate(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// Los 29 países del espacio Schengen.
var schengen = set("AT", "BE", "BG", "HR", "CZ", "DK", "EE", "FI", "FR", "DE", "GR", "HU", "IS",
	"IT", "LV", "LI", "LT", "LU", "MT", "NL", "NO", "PL", "PT", "RO", "SK", "SI", "ES", "SE", "CH")

// Los que circulan libres por Schengen: Unión Europea, EEE y Suiza.
var freeMovement = set("AT", "BE", "BG", "HR", "CY", "CZ", "DK", "EE", "FI", "FR", "DE", "GR", "HU",
	"IE", "IT", "LV", "LT", "LU", "MT", "NL", "PL", "PT", "RO", "SK", "SI", "ES", "SE",
	"IS", "LI", "NO", "CH")

func InSchengen(country string) bool { return schengen[country] }

func FreeMovementInSchengen(nationality string) bool { return freeMovement[nationality] }

func set(values ...string) map[string]bool {
	m := make(map[string]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}
