package postgres

import (
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

func float64ptr(v float64) *float64 { return &v }

// Protege el upsert completo y el recálculo: si method se limpia, su control
// vuelve a abierto aunque los otros tres importes continúen cargados.
func TestPagoHaceUpsertYLosDerivadosSiguenSusCampos(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "pagos@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000103@s.whatsapp.net", "Diego Gómez")
	trip, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID: contact, Destination: "Cancún", DepartAt: fecha("2026-12-10"),
		ReturnAt: fecha("2026-12-20"), CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("alta del viaje: %v", err)
	}

	payment, err := s.PutPayment(ctx, store.Payment{
		TripID: trip.ID, TotalUSD: float64ptr(3200), Rate: float64ptr(1450.5),
		TotalARS: float64ptr(4641600), Method: "transferencia", UpdatedBy: user,
	})
	if err != nil {
		t.Fatalf("alta del pago: %v", err)
	}
	if payment.TotalUSD == nil || *payment.TotalUSD != 3200 {
		t.Fatalf("total devuelto: %#v", payment.TotalUSD)
	}
	for _, id := range []string{"carga-usd", "carga-pesos", "carga-cambio", "carga-metodo"} {
		if got := estado(t, ctx, pool, trip.ID, id); got != "cerrado" {
			t.Errorf("%s quedó %s, se esperaba cerrado", id, got)
		}
	}

	payment.Method = ""
	if _, err := s.PutPayment(ctx, payment); err != nil {
		t.Fatalf("edición del pago: %v", err)
	}
	if got := estado(t, ctx, pool, trip.ID, "carga-metodo"); got != "abierto" {
		t.Errorf("método vacío dejó el control %s, se esperaba abierto", got)
	}
}

// El pago se guarda entero en cada edición, y el handler fecha la seña con la
// hora del pedido. Si la base aceptara esa fecha cada vez, corregir el método
// de pago una semana después movería la seña a hoy: la fecha de cobro tiene que
// ser la primera vez que se marcó, y solo se borra al desmarcarla.
func TestLaSeñaConservaLaFechaEnQueSeCobró(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "senas@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000105@s.whatsapp.net", "Lucía Pérez")
	trip, err := s.CreateTrip(ctx, store.NewTrip{ContactID: contact, CreatedBy: user})
	if err != nil {
		t.Fatalf("alta del viaje: %v", err)
	}

	cobrada := time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)
	payment := store.Payment{TripID: trip.ID, DepositUSD: float64ptr(500),
		DepositPaidAt: &cobrada, UpdatedBy: user}
	if _, err := s.PutPayment(ctx, payment); err != nil {
		t.Fatalf("seña cobrada: %v", err)
	}

	despues := cobrada.AddDate(0, 0, 7)
	payment.Method = "transferencia"
	payment.DepositPaidAt = &despues
	saved, err := s.PutPayment(ctx, payment)
	if err != nil {
		t.Fatalf("edición del pago: %v", err)
	}
	if saved.DepositPaidAt == nil || !saved.DepositPaidAt.Equal(cobrada) {
		t.Fatalf("la seña quedó cobrada el %v, se esperaba %v", saved.DepositPaidAt, cobrada)
	}

	payment.DepositPaidAt = nil
	saved, err = s.PutPayment(ctx, payment)
	if err != nil {
		t.Fatalf("desmarcar la seña: %v", err)
	}
	if saved.DepositPaidAt != nil {
		t.Fatalf("desmarcar dejó la seña cobrada el %v", saved.DepositPaidAt)
	}
}

// Protege la fecha relativa del checklist: mover la salida debe mover los
// controles de auditoría y conservar los plazos de carga basados en el alta.
func TestEditarLaSalidaRecalculaSoloLosPlazosDeControl(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "fechas@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000104@s.whatsapp.net", "Marina Díaz")
	trip, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID: contact, Destination: "Mendoza", DepartAt: fecha("2026-12-10"),
		ReturnAt: fecha("2026-12-20"), CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("alta del viaje: %v", err)
	}

	newDepart := fecha("2027-01-10")
	newReturn := fecha("2027-01-20")
	if _, err := s.UpdateTrip(ctx, trip.ID, store.TripPatch{
		DepartAt: &newDepart, ReturnAt: &newReturn,
	}); err != nil {
		t.Fatalf("edición del viaje: %v", err)
	}

	var due string
	if err := pool.QueryRow(ctx,
		`SELECT due_at::text FROM controls WHERE trip_id = $1 AND item_id = 'control-visa'`,
		trip.ID).Scan(&due); err != nil {
		t.Fatalf("leyendo plazo: %v", err)
	}
	if due != "2026-11-26" {
		t.Errorf("visa vence %s, se esperaba 2026-11-26", due)
	}
}
