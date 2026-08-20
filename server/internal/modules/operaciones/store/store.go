// Package store define qué necesita el módulo de operaciones de su
// persistencia, sin decir cómo se guarda.
package store

import (
	"context"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
)

// NewTrip es el alta mínima: tres campos y quién la hace.
//
// Es corta a propósito. La lapicera le gana a cualquier formulario en velocidad
// de entrada; si abrir un viaje costara más que anotarlo en la planilla, la
// agencia volvería al papel. Todo lo demás —pasajeros, plata, documentos— se
// carga después y de a poco.
type NewTrip struct {
	ContactID   string
	Destination string
	DepartAt    time.Time
	ReturnAt    time.Time
	CreatedBy   string
}

// Trip es el expediente tal como sale de la base.
type Trip struct {
	ID           string
	Number       int64
	CustomerID   string
	Status       domain.TripStatus
	Destination  string
	DepartAt     time.Time
	ReturnAt     time.Time
	SellerUserID string
	CreatedAt    time.Time
}

// TripStore es la escritura del expediente.
type TripStore interface {
	// CreateTrip abre un viaje para el contacto dado. Si ese contacto todavía no
	// tiene cliente, lo crea con el nombre del contacto y los vincula: el vínculo
	// tiene que ser invisible, porque un paso extra al vender es un paso que
	// nadie da.
	//
	// Materializa además la copia de la plantilla, con los vencimientos ya
	// calculados. Todo en una transacción: un viaje sin controles sería un
	// expediente que afirma no tener nada pendiente.
	CreateTrip(ctx context.Context, n NewTrip) (Trip, error)
}
