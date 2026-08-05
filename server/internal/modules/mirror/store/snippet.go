package store

import (
	"errors"
	"strings"
	"unicode"
)

var (
	// ErrSearchTooShort: el término no llega al mínimo que hace útil el índice de
	// trigramas.
	ErrSearchTooShort = errors.New("término de búsqueda demasiado corto")

	// ErrSearchTooBroad: la consulta agotó su tiempo. Le pasa a los términos muy
	// comunes, donde el candidato del índice es enorme.
	ErrSearchTooBroad = errors.New("búsqueda demasiado amplia")
)

// snippetContext son los caracteres de contexto a cada lado del match.
const snippetContext = 40

// Snippet recorta el fragmento del mensaje alrededor de la coincidencia.
//
// Se hace en Go y no en SQL a propósito: ts_headline pesa, exige un tsvector que
// aquí no tenemos (la búsqueda es por subcadena, no por lenguaje natural) y
// obligaría a la base a formatear texto en la ruta de la consulta.
//
// El corte respeta los límites de runa: cortar por bytes parte los acentos y las
// eñes por la mitad y produce texto inválido.
func Snippet(body, term string) string {
	runes := []rune(body)
	if len(runes) <= snippetContext*2+len([]rune(term)) {
		return body // cabe entero: no hay nada que recortar
	}

	at := indexFold(body, term)
	if at < 0 {
		// La coincidencia la encontró Postgres, así que esto solo ocurre si su
		// noción de mayúsculas difiere de la de Go en algún alfabeto. Se devuelve
		// el principio en vez de un fragmento vacío.
		return string(runes[:snippetContext*2]) + "…"
	}

	start := max(at-snippetContext, 0)
	end := min(at+len([]rune(term))+snippetContext, len(runes))

	var b strings.Builder
	if start > 0 {
		b.WriteString("…")
	}
	b.WriteString(string(runes[start:end]))
	if end < len(runes) {
		b.WriteString("…")
	}
	return b.String()
}

// indexFold busca `term` en `body` sin distinguir mayúsculas y devuelve la
// posición en RUNAS, no en bytes: el llamador indexa un []rune.
func indexFold(body, term string) int {
	lowerBody := []rune(strings.ToLower(body))
	lowerTerm := []rune(strings.ToLower(term))
	if len(lowerTerm) == 0 || len(lowerTerm) > len(lowerBody) {
		return -1
	}

	for i := 0; i+len(lowerTerm) <= len(lowerBody); i++ {
		match := true
		for j, r := range lowerTerm {
			if unicode.ToLower(lowerBody[i+j]) != r {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
