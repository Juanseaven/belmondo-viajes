package store

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrBadCursor indica un cursor ilegible. Se traduce a 400: no es un fallo del
// servidor, es un cliente mandando algo que no le dimos nosotros.
var ErrBadCursor = errors.New("cursor inválido")

// Cursor es la posición en un listado paginado por keyset.
//
// Se usa keyset y no OFFSET porque estas listas se reordenan solas: cada mensaje
// que entra mueve una conversación al principio de la bandeja. Con OFFSET, el
// scroll devuelve filas repetidas o se salta otras mientras el usuario lee. El
// keyset ancla la posición a una fila concreta, así que la inserción de filas
// nuevas por arriba no lo afecta.
//
// El par (At, ID) es siempre el mismo que la clave del índice que sirve la
// consulta: (last_message_at, id) en la bandeja, (wa_timestamp, id) en el hilo.
// El id desempata las filas con el mismo instante, sin el cual una página podría
// saltarse mensajes enviados en el mismo segundo.
type Cursor struct {
	At time.Time
	ID string
}

// Encode produce un token opaco. Deliberadamente opaco: el cliente no debe
// construirlo ni razonar sobre él, así que cambiar el criterio de orden más
// adelante no rompe a nadie.
//
// No va firmado: un cursor manipulado solo puede mover al atacante por un
// listado que ya tiene permiso de ver, y el parseo valida la forma. Firmarlo
// añadiría criptografía a cada scroll sin proteger nada.
func (c Cursor) Encode() string {
	raw := strconv.FormatInt(c.At.UTC().UnixNano(), 10) + "|" + c.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// ParseCursor lee un token. Una cadena vacía devuelve el cursor cero, que
// significa "desde el principio" y es válido.
func ParseCursor(token string) (Cursor, error) {
	if token == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Cursor{}, ErrBadCursor
	}

	nanos, id, found := strings.Cut(string(raw), "|")
	if !found || id == "" {
		return Cursor{}, ErrBadCursor
	}
	n, err := strconv.ParseInt(nanos, 10, 64)
	if err != nil {
		return Cursor{}, ErrBadCursor
	}
	return Cursor{At: time.Unix(0, n).UTC(), ID: id}, nil
}

// IsZero distingue "primera página" de "continuación".
func (c Cursor) IsZero() bool { return c.ID == "" }

// Page es una página de resultados con el cursor para pedir la siguiente.
//
// NextCursor vacío significa que no hay más. Se calcula desde la última fila
// devuelta, no contando el total: contar exige un COUNT(*) que escanea toda la
// tabla en cada scroll.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// NewPage recorta el exceso y calcula el cursor siguiente.
//
// El truco es pedir a la base limit+1 filas: si vuelven todas, hay más página, y
// eso se sabe sin un COUNT aparte. La fila extra se descarta.
func NewPage[T any](rows []T, limit int, cursorOf func(T) Cursor) Page[T] {
	if len(rows) <= limit {
		return Page[T]{Items: rows}
	}
	rows = rows[:limit]
	return Page[T]{
		Items:      rows,
		NextCursor: cursorOf(rows[len(rows)-1]).Encode(),
	}
}

// ClampLimit acota el tamaño de página pedido por el cliente.
//
// El techo no es cosmético: sin él, un `limit=100000` convierte cualquier
// endpoint de listado en una denegación de servicio de una sola petición.
func ClampLimit(raw string, def, max int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func (c Cursor) String() string {
	return fmt.Sprintf("Cursor(%s, %s)", c.At.Format(time.RFC3339Nano), c.ID)
}
