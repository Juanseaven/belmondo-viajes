package postgres

import (
	"testing"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

func TestDetalleDelViajeHidrataElExpedienteCompleto(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "detalle@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000201@s.whatsapp.net", "Ana Pérez")
	trip, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID: contact, Destination: "Iguazú", DepartAt: fecha("2026-11-10"),
		ReturnAt: fecha("2026-11-17"), CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("alta del viaje: %v", err)
	}
	if _, err := s.AddPassenger(ctx, store.NewPassenger{
		TripID: trip.ID, FirstName: "Ana", LastName: "Pérez", Kind: "adulto",
		IsHolder: true, CreatedBy: user,
	}); err != nil {
		t.Fatalf("alta del pasajero: %v", err)
	}
	if _, err := s.PutPayment(ctx, store.Payment{
		TripID: trip.ID, TotalUSD: float64ptr(1840), Rate: float64ptr(1171),
		TotalARS: float64ptr(2154640), Method: "transferencia", UpdatedBy: user,
	}); err != nil {
		t.Fatalf("alta del pago: %v", err)
	}
	var controlID string
	if err := pool.QueryRow(ctx,
		`SELECT id FROM controls WHERE trip_id = $1 AND item_id = 'control-comidas'`, trip.ID,
	).Scan(&controlID); err != nil {
		t.Fatalf("control manual: %v", err)
	}
	if err := s.CloseControl(ctx, store.CloseControl{ControlID: controlID, UserID: user}); err != nil {
		t.Fatalf("cierre manual: %v", err)
	}

	detail, err := s.TripDetail(ctx, trip.ID)
	if err != nil {
		t.Fatalf("detalle: %v", err)
	}
	if detail.Customer.Name != "Ana Pérez" || len(detail.Customer.ContactIDs) != 1 || detail.Customer.ContactIDs[0] != contact {
		t.Fatalf("cliente hidratado: %#v", detail.Customer)
	}
	if len(detail.Passengers) != 1 || detail.Passengers[0].FirstName != "Ana" {
		t.Fatalf("pasajeros: %#v", detail.Passengers)
	}
	if detail.Payment == nil || detail.Payment.TotalUSD == nil || *detail.Payment.TotalUSD != 1840 {
		t.Fatalf("pago: %#v", detail.Payment)
	}
	if len(detail.Controls) != 31 {
		t.Fatalf("controles: %d, se esperaban 31", len(detail.Controls))
	}
	var meals *store.Control
	for i := range detail.Controls {
		if detail.Controls[i].ItemID == "control-comidas" {
			meals = &detail.Controls[i]
			break
		}
	}
	if meals == nil || meals.Title != "Régimen de comidas" || len(meals.Evidence) != 1 || meals.Evidence[0].ClosedByName != "Vendedora" {
		t.Fatalf("control con evidencia: %#v", meals)
	}
}

func TestBandejaConservaContactosSinViajeYEligeElAbiertoMasReciente(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "bandeja@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000202@s.whatsapp.net", "Familia Rossi")
	withoutTrip := seedContact(t, ctx, pool, "5491100000203@s.whatsapp.net", "Consulta Nueva")

	first, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID: contact, Destination: "Salta", DepartAt: fecha("2026-12-10"),
		ReturnAt: fecha("2026-12-17"), CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("primer viaje: %v", err)
	}
	closed := domain.TripCerrado
	if _, err := s.UpdateTrip(ctx, first.ID, store.TripPatch{Status: &closed}); err != nil {
		t.Fatalf("cerrando viaje: %v", err)
	}
	second, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID: contact, Destination: "Bariloche", DepartAt: fecha("2027-01-10"),
		ReturnAt: fecha("2027-01-17"), CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("segundo viaje: %v", err)
	}

	// Aísla el criterio: crítico gana sobre fecha, y entre críticos vence antes.
	if _, err := pool.Exec(ctx, `UPDATE controls SET status = 'cerrado' WHERE trip_id = $1`, second.ID); err != nil {
		t.Fatalf("cerrando controles: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE controls SET status = 'abierto', critical = false, due_at = '2026-08-21'
		  WHERE trip_id = $1 AND item_id = 'carga-preexistencia'`, second.ID); err != nil {
		t.Fatalf("preparando prioridades: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE controls SET status = 'abierto', critical = true, due_at = '2026-08-25'
		  WHERE trip_id = $1 AND item_id = 'carga-acepto'`, second.ID); err != nil {
		t.Fatalf("preparando control crítico: %v", err)
	}

	items, err := s.Inbox(ctx, []string{withoutTrip, contact})
	if err != nil {
		t.Fatalf("bandeja: %v", err)
	}
	if len(items) != 2 || items[0].ContactID != withoutTrip || items[0].Trip != nil {
		t.Fatalf("contacto sin viaje: %#v", items)
	}
	if items[1].Trip == nil || items[1].Trip.ID != second.ID || items[1].Trip.Destination != "Bariloche" {
		t.Fatalf("viaje elegido: %#v", items[1].Trip)
	}
	if items[1].Trip.NextControl == nil || items[1].Trip.NextControl.ItemID != "carga-acepto" {
		t.Fatalf("próximo control: %#v", items[1].Trip.NextControl)
	}

	trips, err := s.TripsByContact(ctx, contact)
	if err != nil {
		t.Fatalf("viajes del contacto: %v", err)
	}
	if len(trips) != 2 || trips[0].ID != second.ID || trips[1].ID != first.ID {
		t.Fatalf("orden de viajes: %#v", trips)
	}
}

func TestClienteSeEditaYVinculaOtroContactoSinDuplicarlo(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "clientes@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000204@s.whatsapp.net", "Ana Pérez")
	other := seedContact(t, ctx, pool, "5491100000205@s.whatsapp.net", "Ana oficina")
	trip, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID: contact, Destination: "Rosario", DepartAt: fecha("2026-10-10"),
		ReturnAt: fecha("2026-10-12"), CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("alta del viaje: %v", err)
	}
	email, phone := "ana@example.com", "+5491112345678"
	customer, err := s.UpdateCustomer(ctx, trip.CustomerID, store.CustomerPatch{Email: &email, PhoneNumber: &phone})
	if err != nil {
		t.Fatalf("edición del cliente: %v", err)
	}
	if customer.Email != email || customer.PhoneNumber != phone {
		t.Fatalf("cliente editado: %#v", customer)
	}
	if got := estado(t, ctx, pool, trip.ID, "carga-contacto"); got != "cerrado" {
		t.Errorf("mail y teléfono dejaron el control %s", got)
	}
	if err := s.LinkContact(ctx, trip.CustomerID, other, user); err != nil {
		t.Fatalf("vinculando contacto: %v", err)
	}
	customer, err = s.Customer(ctx, trip.CustomerID)
	if err != nil {
		t.Fatalf("leyendo cliente: %v", err)
	}
	if len(customer.ContactIDs) != 2 {
		t.Fatalf("contactos vinculados: %#v", customer.ContactIDs)
	}
}
