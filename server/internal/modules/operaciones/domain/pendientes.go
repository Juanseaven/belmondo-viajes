package domain

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PendingGroup es el grupo de la lista de pendientes en el que cae un viaje.
type PendingGroup string

const (
	GroupAtrasado PendingGroup = "atrasado"
	GroupSemana   PendingGroup = "semana"
	GroupSalen    PendingGroup = "salen"
)

// Las ventanas de la lista. Una semana es lo que se puede planificar hoy; dos
// semanas es lo que hay que tener a la vista antes de que un viaje salga.
const (
	weekWindow   = 7
	departWindow = 14
)

type PendingKind string

const (
	PendingControl PendingKind = "control"
	PendingDeposit PendingKind = "sena"
)

// PendingItem es algo de un viaje que tiene plazo: un control abierto o la seña
// sin cobrar. Las fechas son días de calendario, a la medianoche UTC, como las
// devuelve Postgres para una columna date.
type PendingItem struct {
	Kind PendingKind `json:"kind"`
	// ID es el control, o vacío para la seña.
	ID       string    `json:"id,omitempty"`
	ItemID   string    `json:"itemId,omitempty"`
	Task     string    `json:"task"`
	DueAt    time.Time `json:"dueAt"`
	Critical bool      `json:"critical"`
}

// DepositItem es la seña sin cobrar como pendiente. Cuenta como crítica: es la
// pregunta que la agencia se hace todas las mañanas.
func DepositItem(dueAt time.Time, amountUSD *float64) PendingItem {
	task := "Cobrar la seña"
	if amountUSD != nil {
		task += " de USD " + formatAmount(*amountUSD)
	}
	return PendingItem{Kind: PendingDeposit, Task: task, DueAt: dueAt, Critical: true}
}

// PendingTrip es dónde cae un viaje y qué hay que hacer con él.
type PendingTrip struct {
	Group PendingGroup
	// Main es la tarea que encabeza el renglón. Nil en un viaje que sale pronto
	// sin nada abierto con plazo: «Todo listo».
	Main *PendingItem
	// More cuenta lo demás que está atrasado o vence esta semana.
	More int
}

// ClassifyPending decide si un viaje entra en la lista de pendientes, en qué
// grupo y con qué tarea. today es el día calendario en la hora de la agencia.
//
// Un viaje cae en un solo grupo, el más urgente que le toque. Lo que vence más
// allá de la semana no lo hace aparecer: no pide nada todavía.
func ClassifyPending(today time.Time, departAt *time.Time, items []PendingItem) (PendingTrip, bool) {
	weekEnd := today.AddDate(0, 0, weekWindow)
	var overdue, week []PendingItem
	for _, item := range items {
		switch {
		case item.DueAt.Before(today):
			overdue = append(overdue, item)
		case !item.DueAt.After(weekEnd):
			week = append(week, item)
		}
	}

	switch {
	case len(overdue) > 0:
		return PendingTrip{Group: GroupAtrasado, Main: NextPending(overdue), More: len(overdue) + len(week) - 1}, true
	case len(week) > 0:
		return PendingTrip{Group: GroupSemana, Main: NextPending(week), More: len(week) - 1}, true
	case departAt != nil && !departAt.Before(today) && !departAt.After(today.AddDate(0, 0, departWindow)):
		// Lo próximo que vence, aunque sea más allá de la semana: es lo que va a
		// pedir el viaje antes de salir.
		return PendingTrip{Group: GroupSalen, Main: earliest(items)}, true
	default:
		return PendingTrip{}, false
	}
}

// NextPending es la misma regla que «Lo que sigue» en la ficha: lo crítico
// primero, después el plazo más cercano.
func NextPending(items []PendingItem) *PendingItem {
	if len(items) == 0 {
		return nil
	}
	sorted := append([]PendingItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Critical != sorted[j].Critical {
			return sorted[i].Critical
		}
		return sorted[i].DueAt.Before(sorted[j].DueAt)
	})
	return &sorted[0]
}

func earliest(items []PendingItem) *PendingItem {
	if len(items) == 0 {
		return nil
	}
	first := items[0]
	for _, item := range items[1:] {
		if item.DueAt.Before(first.DueAt) {
			first = item
		}
	}
	return &first
}

// formatAmount escribe un monto como se lee en Argentina: 5.900 o 5.900,50.
func formatAmount(value float64) string {
	whole := strconv.FormatInt(int64(value), 10)
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	if cents := int64(value*100+0.5) % 100; cents != 0 {
		fmt.Fprintf(&b, ",%02d", cents)
	}
	return b.String()
}
