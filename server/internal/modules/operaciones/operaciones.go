// Package operaciones implementa los clientes, viajes y controles de la agencia.
package operaciones

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"time"
	// La imagen del backend no trae la base de husos horarios del sistema: sin
	// esto, cargar la hora de Argentina fallaría en producción y no en los tests.
	_ "time/tzdata"

	"github.com/juanseaven/viajes-crm/server/internal/kernel"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/ws"
	"github.com/juanseaven/viajes-crm/server/internal/modules"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/evaluacion"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store/postgres"
	reglas "github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
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
	DeletePassenger(context.Context, string) (string, error)
	PutPayment(context.Context, store.Payment) (store.Payment, error)
	CloseControl(context.Context, store.CloseControl) error
	ReopenControl(context.Context, string) error
	Customer(context.Context, string) (store.Customer, error)
	UpdateCustomer(context.Context, string, store.CustomerPatch) (store.Customer, error)
	LinkContact(context.Context, string, string, string) error
	Inbox(context.Context, []string) ([]store.InboxItem, error)
	Sellers(context.Context) ([]store.Seller, error)
	Pending(context.Context, time.Time) ([]store.PendingRow, error)
	ListTrips(context.Context, store.TripFilter) (store.TripPage, error)
	PutItinerary(context.Context, string, store.Itinerary) error
	AddDocument(context.Context, store.NewDocument) (store.Document, error)
	UpdateDocument(context.Context, string, store.DocumentPatch) (store.Document, error)
	DeleteDocument(context.Context, string) (string, error)
	AddPermit(context.Context, store.NewPermit) (store.Permit, error)
	UpdatePermit(context.Context, string, store.PermitPatch) (store.Permit, error)
	DeletePermit(context.Context, string) (string, error)
	EvaluationInput(context.Context, string) (store.EvaluationInput, error)
	SaveRequirements(context.Context, string, []evaluacion.Requirement, map[string]string) error
	Confirm(context.Context, store.NewConfirmation) error
	OpenTripsUsing(context.Context, []reglas.Pair) ([]string, error)
	OpenTrips(context.Context) ([]string, error)
}

type Module struct {
	repo  repository
	rules ruleSource
	log   *slog.Logger
	emit  func(string, map[string]any)
	clock func() time.Time
}

// New construye el módulo. rules es el módulo requisitos; sin él, los
// requisitos de todos los viajes quedan en «verificar».
func New(k *kernel.Kernel, rules ruleSource) *Module {
	m := &Module{repo: postgres.New(k.DB), rules: rules, log: k.Log.With("module", Name), clock: time.Now}
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
	r.Authed("GET /vendedores", handler(m.sellers))
	r.Authed("GET /pendientes", handler(m.pending))
	r.Authed("PUT /viajes/{id}/itinerario", handler(m.putItinerary))
	r.Authed("POST /pasajeros/{id}/documentos", handler(m.addDocument))
	r.Authed("PATCH /documentos/{id}", handler(m.updateDocument))
	r.Authed("DELETE /documentos/{id}", handler(m.deleteDocument))
	r.Authed("POST /pasajeros/{id}/permisos", handler(m.addPermit))
	r.Authed("PATCH /permisos/{id}", handler(m.updatePermit))
	r.Authed("DELETE /permisos/{id}", handler(m.deletePermit))
	r.Authed("POST /viajes/{id}/requisitos/confirmar", handler(m.confirmRequirement))
}

// Start evalúa en segundo plano los viajes abiertos: los que existían antes de
// los requisitos y los que cambiaron con el servidor apagado.
func (m *Module) Start(ctx context.Context) error {
	go m.evaluateOpenTrips(ctx)
	return nil
}

func (m *Module) Shutdown(context.Context) error { return nil }
