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
	ErrNotFound           = errors.New("operaciones: no existe")
	ErrConflict           = errors.New("operaciones: conflicto")
	ErrBadCursor          = errors.New("operaciones: cursor inválido")
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
	ID            string            `json:"id"`
	Number        int64             `json:"number"`
	CustomerID    string            `json:"customerId"`
	Status        domain.TripStatus `json:"status"`
	Destination   string            `json:"destination"`
	DepartAt      *time.Time        `json:"departAt,omitempty"`
	ReturnAt      *time.Time        `json:"returnAt,omitempty"`
	SellerUserID  string            `json:"sellerUserId,omitempty"`
	SellerName    string            `json:"sellerName,omitempty"`
	CreatedByName string            `json:"createdByName,omitempty"`
	CreatedAt     time.Time         `json:"createdAt"`
}

type TripPatch struct {
	Destination  *string
	DepartAt     *time.Time
	ReturnAt     *time.Time
	Status       *domain.TripStatus
	SellerUserID *string
}

type NewPassenger struct {
	TripID            string
	FirstName         string
	LastName          string
	Kind              string
	IsHolder          bool
	DocumentType      string
	DocumentNumber    string
	DocumentExpiresAt *time.Time
	CreatedBy         string
}

type Passenger struct {
	ID                string     `json:"id"`
	TripID            string     `json:"tripId"`
	FirstName         string     `json:"firstName"`
	LastName          string     `json:"lastName"`
	Kind              string     `json:"kind"`
	IsHolder          bool       `json:"isHolder"`
	DocumentType      string     `json:"documentType,omitempty"`
	DocumentNumber    string     `json:"documentNumber,omitempty"`
	DocumentExpiresAt *time.Time `json:"documentExpiresAt,omitempty"`
}

type PassengerPatch struct {
	FirstName         *string
	LastName          *string
	Kind              *string
	IsHolder          *bool
	DocumentType      *string
	DocumentNumber    *string
	DocumentExpiresAt *string
}

type Payment struct {
	TripID           string     `json:"tripId"`
	TotalUSD         *float64   `json:"totalUsd,omitempty"`
	Rate             *float64   `json:"rate,omitempty"`
	TotalARS         *float64   `json:"totalArs,omitempty"`
	Method           string     `json:"method,omitempty"`
	DepositUSD       *float64   `json:"depositUsd,omitempty"`
	DepositDueAt     *time.Time `json:"depositDueAt,omitempty"`
	DepositPaidAt    *time.Time `json:"depositPaidAt,omitempty"`
	NonRefundableUSD *float64   `json:"nonRefundableUsd,omitempty"`
	UpdatedBy        string     `json:"-"`
}

// Seller es un usuario al que se le puede asignar un viaje. Se lee de la tabla
// users del kernel; operaciones nunca la escribe.
type Seller struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Customer struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Email          string    `json:"email,omitempty"`
	PhoneNumber    string    `json:"phoneNumber,omitempty"`
	DocumentType   string    `json:"documentType,omitempty"`
	DocumentNumber string    `json:"documentNumber,omitempty"`
	CustomerSince  time.Time `json:"customerSince"`
	ContactIDs     []string  `json:"contactIds"`
}

type CustomerPatch struct {
	Name           *string
	Email          *string
	PhoneNumber    *string
	DocumentType   *string
	DocumentNumber *string
	CustomerSince  *string
}

type ControlEvidence struct {
	ID           string    `json:"id"`
	ClosedBy     string    `json:"closedBy,omitempty"`
	ClosedByName string    `json:"closedByName,omitempty"`
	ClosedAt     time.Time `json:"closedAt"`
	MessageID    string    `json:"messageId,omitempty"`
}

type Control struct {
	ID          string               `json:"id"`
	ItemID      string               `json:"itemId"`
	Stage       domain.Stage         `json:"stage"`
	Closing     domain.Closing       `json:"closing"`
	Title       string               `json:"title"`
	Prompt      string               `json:"prompt,omitempty"`
	ActionLabel string               `json:"actionLabel,omitempty"`
	Message     string               `json:"message,omitempty"`
	Critical    bool                 `json:"critical"`
	Status      domain.ControlStatus `json:"status"`
	DueAt       *time.Time           `json:"dueAt,omitempty"`
	Evidence    []ControlEvidence    `json:"evidence"`
}

type ControlSummary struct {
	ID       string         `json:"id"`
	ItemID   string         `json:"itemId"`
	Title    string         `json:"title"`
	Critical bool           `json:"critical"`
	DueAt    *time.Time     `json:"dueAt,omitempty"`
	Closing  domain.Closing `json:"closing"`
}

type TripSummary struct {
	Trip
	PassengerCount int             `json:"passengerCount"`
	TotalUSD       *float64        `json:"totalUsd,omitempty"`
	NextControl    *ControlSummary `json:"nextControl,omitempty"`
}

type TripDetailData struct {
	Trip
	Customer   Customer    `json:"customer"`
	Passengers []Passenger `json:"passengers"`
	Payment    *Payment    `json:"payment,omitempty"`
	Controls   []Control   `json:"controls"`
}

// TripRow es un viaje como se ve en una lista: lo justo para leerlo de un
// vistazo y saltar a su conversación.
type TripRow struct {
	ID             string            `json:"id"`
	Number         int64             `json:"number"`
	Status         domain.TripStatus `json:"status"`
	Destination    string            `json:"destination"`
	DepartAt       *time.Time        `json:"departAt,omitempty"`
	ReturnAt       *time.Time        `json:"returnAt,omitempty"`
	CustomerName   string            `json:"customerName"`
	PassengerCount int               `json:"passengerCount"`
	SellerName     string            `json:"sellerName,omitempty"`
	TotalUSD       *float64          `json:"totalUsd,omitempty"`
	DepositUSD     *float64          `json:"depositUsd,omitempty"`
	DepositDueAt   *time.Time        `json:"depositDueAt,omitempty"`
	DepositPaidAt  *time.Time        `json:"depositPaidAt,omitempty"`
	// Next es lo más urgente del viaje, sin importar cuándo vence. Nil: al día.
	Next *domain.PendingItem `json:"next,omitempty"`
	// ConversationID es la conversación más reciente de cualquiera de los
	// contactos del cliente. Vacío si el cliente no tiene ninguna.
	ConversationID string `json:"conversationId,omitempty"`
}

// PendingRow es un viaje en la lista de pendientes.
type PendingRow struct {
	TripRow
	Group domain.PendingGroup `json:"group"`
	// Task encabeza el renglón. Nil en un viaje que sale pronto y está al día.
	Task *domain.PendingItem `json:"task,omitempty"`
	More int                 `json:"more"`
}

type TripSort string

const (
	SortSalida TripSort = "salida"
	SortNumero TripSort = "numero"
	SortTotal  TripSort = "total"
)

// TripFilter es la consulta de «Todos los viajes». Today es el día calendario
// en la hora de la agencia, para el filtro de atrasados.
type TripFilter struct {
	Query string
	// OpenOnly deja afuera los cerrados. Statuses, si no está vacío, manda.
	OpenOnly   bool
	Statuses   []domain.TripStatus
	DepartFrom *time.Time
	DepartTo   *time.Time
	// SellerID filtra por vendedor; SellerNone, los viajes sin asignar.
	SellerID   string
	SellerNone bool
	LateOnly   bool
	Sort       TripSort
	Cursor     string
	Limit      int
	Today      time.Time
}

type TripPage struct {
	Items      []TripRow `json:"items"`
	Total      int       `json:"total"`
	NextCursor string    `json:"nextCursor,omitempty"`
}

type InboxItem struct {
	ContactID string       `json:"contactId"`
	Trip      *TripSummary `json:"trip,omitempty"`
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
