// Package operaciones implementa los clientes, viajes y controles de la agencia.
package operaciones

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/juanseaven/viajes-crm/server/internal/kernel"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/ws"
	"github.com/juanseaven/viajes-crm/server/internal/modules"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store/postgres"
)

const Name = domain.ModuleName

//go:embed migrations/*.sql
var migrationFiles embed.FS

type repository interface {
	CreateTrip(context.Context, store.NewTrip) (store.Trip, error)
	UpdateTrip(context.Context, string, store.TripPatch) (store.Trip, error)
	TripsByContact(context.Context, string) ([]store.TripSummary, error)
	TripDetail(context.Context, string) (store.TripDetailData, error)
	AddPassenger(context.Context, store.NewPassenger) (store.Passenger, error)
	UpdatePassenger(context.Context, string, store.PassengerPatch) (store.Passenger, error)
	DeletePassenger(context.Context, string) error
	PutPayment(context.Context, store.Payment) (store.Payment, error)
	CloseControl(context.Context, store.CloseControl) error
	ReopenControl(context.Context, string) error
	Customer(context.Context, string) (store.Customer, error)
	UpdateCustomer(context.Context, string, store.CustomerPatch) (store.Customer, error)
	LinkContact(context.Context, string, string, string) error
	Inbox(context.Context, []string) ([]store.InboxItem, error)
}

type Module struct {
	repo repository
	log  *slog.Logger
	emit func(string, map[string]any)
}

func New(k *kernel.Kernel) *Module {
	m := &Module{repo: postgres.New(k.DB), log: k.Log.With("module", Name)}
	m.emit = func(eventType string, payload map[string]any) {
		raw, err := jsonBytes(payload)
		if err != nil {
			m.log.Error("no se pudo serializar el evento", "type", eventType, "error", err)
			return
		}
		k.Hub.Broadcast(ws.Event{Module: Name, Type: eventType, Payload: raw, At: nowUTC()})
	}
	return m
}

var _ modules.Module = (*Module)(nil)

func (m *Module) Name() string { return Name }

func (m *Module) Migrations() fs.FS {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		panic(fmt.Sprintf("operaciones: migraciones embebidas ilegibles: %v", err))
	}
	return sub
}

func (m *Module) Mount(r modules.Router) {
	r.Authed("GET /viajes", handler(m.listTrips))
	r.Authed("POST /viajes", handler(m.createTrip))
	r.Authed("GET /viajes/{id}", handler(m.getTrip))
	r.Authed("PATCH /viajes/{id}", handler(m.updateTrip))
	r.Authed("POST /viajes/{id}/pasajeros", handler(m.addPassenger))
	r.Authed("PATCH /pasajeros/{id}", handler(m.updatePassenger))
	r.Authed("DELETE /pasajeros/{id}", handler(m.deletePassenger))
	r.Authed("PUT /viajes/{id}/pago", handler(m.putPayment))
	r.Authed("POST /controles/{id}/cerrar", handler(m.closeControl))
	r.Authed("POST /controles/{id}/reabrir", handler(m.reopenControl))
	r.Authed("GET /clientes/{id}", handler(m.getCustomer))
	r.Authed("PATCH /clientes/{id}", handler(m.updateCustomer))
	r.Authed("POST /clientes/{id}/contactos", handler(m.linkContact))
	r.Authed("GET /bandeja", handler(m.inbox))
}

func (m *Module) Start(context.Context) error    { return nil }
func (m *Module) Shutdown(context.Context) error { return nil }
