// Package store define qué necesita el módulo de operaciones de su
// persistencia, sin decir cómo se guarda.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
)

// Los errores que el módulo distingue al cerrar un control. Cada uno existe
// porque, sin él, el control volvería a ser una declaración.
var (
	// ErrEvidenciaRequerida: un control por acción no se cierra sin el mensaje
	// que lo prueba. Permitirlo lo convertiría en un tilde, que es exactamente lo
	// que este diseño evita.
	ErrEvidenciaRequerida = errors.New("operaciones: el control se cierra con el mensaje que lo prueba")

	// ErrEvidenciaAjena: el mensaje existe pero es de otra conversación. Un
	// enlace a un mensaje que no le habla a este cliente no prueba nada.
	ErrEvidenciaAjena = errors.New("operaciones: el mensaje no es de una conversación de este cliente")

	// ErrDerivadoNoSeCierraAMano: lo cierra el dato, no una persona.
	ErrDerivadoNoSeCierraAMano = errors.New("operaciones: un control derivado lo cierra el dato")

	ErrControlInexistente = errors.New("operaciones: el control no existe")
)

// CloseControl es la orden de cerrar un renglón.
//
// MessageID es obligatorio cuando el renglón se cierra por acción y opcional en
// los manuales, que se conforman con la firma de quien los confirmó.
type CloseControl struct {
	ControlID string
	UserID    string
	MessageID string
}

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
