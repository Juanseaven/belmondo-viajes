package postgres

import (
	"testing"

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
		Method: "transferencia", UpdatedBy: user,
	})
	if err != nil {
		t.Fatalf("alta del pago: %v", err)
	}
	if payment.TotalUSD == nil || *payment.TotalUSD != 3200 {
		t.Fatalf("total devuelto: %#v", payment.TotalUSD)
	}
	// El total en pesos se calcula: total en dólares por el cambio.
	if payment.TotalARS == nil || *payment.TotalARS != 4641600 {
		t.Fatalf("total en pesos: %#v, se esperaba 4641600", payment.TotalARS)
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

func TestEditarLaSalidaNoInventaPlazosDeControl(t *testing.T) {
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

	var due *string
	if err := pool.QueryRow(ctx,
		`SELECT due_at::text FROM controls WHERE trip_id = $1 AND item_id = 'control-visa'`,
		trip.ID).Scan(&due); err != nil {
		t.Fatalf("leyendo plazo: %v", err)
	}
	if due != nil {
		t.Errorf("editar la salida inventó el plazo de visa %s", *due)
	}
}
