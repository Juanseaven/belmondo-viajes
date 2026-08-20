package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// El cierre derivado es la pieza que saca a diez renglones de la lista sin que
// nadie los tilde. Su regla es una sola: el control sigue al dato. Si el dato
// está, el control está cerrado; si el dato se va, el control vuelve a abrirse.
//
// La segunda mitad importa tanto como la primera. Un control que se cierra y ya
// no vuelve a mirarse es la misma mentira que un tilde, con más pasos.
func TestElCierreDerivadoSigueAlDato(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)

	user := seedUser(t, ctx, pool, "vendedora@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000003@s.whatsapp.net", "Marina Ferreyra")
	viaje, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID: contact, Destination: "Cancún",
		DepartAt: fecha("2026-12-10"), ReturnAt: fecha("2026-12-24"), CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("alta del viaje: %v", err)
	}

	// El alta pide destino y fechas, así que esos tres renglones nacen cerrados.
	// No son preguntas: son hechos que el formulario ya exigió.
	for _, id := range []string{"carga-destino", "carga-salida", "carga-llegada"} {
		if estado(t, ctx, pool, viaje.ID, id) != "cerrado" {
			t.Errorf("%s: debería nacer cerrado, el alta ya pidió el dato", id)
		}
	}
	// Nadie cargó pasajeros todavía.
	for _, id := range []string{"carga-nombres", "carga-apellidos", "control-titular"} {
		if estado(t, ctx, pool, viaje.ID, id) != "abierto" {
			t.Errorf("%s: no hay pasajeros, debería estar abierto", id)
		}
	}

	pasajero := seedPassenger(t, ctx, pool, viaje.ID, "Marina", "Ferreyra", false)
	if err := s.RefreshDerived(ctx, viaje.ID); err != nil {
		t.Fatalf("recalculando derivados: %v", err)
	}
	for _, id := range []string{"carga-nombres", "carga-apellidos"} {
		if estado(t, ctx, pool, viaje.ID, id) != "cerrado" {
			t.Errorf("%s: hay un pasajero con nombre y apellido, debería cerrarse solo", id)
		}
	}
	if estado(t, ctx, pool, viaje.ID, "control-titular") != "abierto" {
		t.Error("control-titular: ningún pasajero es el titular todavía")
	}

	if _, err := pool.Exec(ctx,
		`UPDATE passengers SET is_holder = true WHERE id = $1`, pasajero); err != nil {
		t.Fatalf("marcando titular: %v", err)
	}
	if err := s.RefreshDerived(ctx, viaje.ID); err != nil {
		t.Fatalf("recalculando derivados: %v", err)
	}
	if estado(t, ctx, pool, viaje.ID, "control-titular") != "cerrado" {
		t.Error("control-titular: hay titular, debería cerrarse solo")
	}

	// Y el paso que prueba que esto no es un tilde con otro nombre.
	if _, err := pool.Exec(ctx, `DELETE FROM passengers WHERE id = $1`, pasajero); err != nil {
		t.Fatalf("borrando el pasajero: %v", err)
	}
	if err := s.RefreshDerived(ctx, viaje.ID); err != nil {
		t.Fatalf("recalculando derivados: %v", err)
	}
	for _, id := range []string{"carga-nombres", "carga-apellidos", "control-titular"} {
		if estado(t, ctx, pool, viaje.ID, id) != "abierto" {
			t.Errorf("%s: se fue el dato, el control tiene que volver a abrirse", id)
		}
	}
}

// Los cuatro renglones de plata de la hoja de carga son la tabla de valores de
// la planilla. Existen como control solo porque en papel había que acordarse de
// escribirlos.
func TestLosRenglonesDePlataSiguenAlPago(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)

	user := seedUser(t, ctx, pool, "vendedora@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000004@s.whatsapp.net", "Grupo Rossi")
	viaje, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID: contact, Destination: "Bariloche",
		DepartAt: fecha("2026-12-10"), ReturnAt: fecha("2026-12-24"), CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("alta del viaje: %v", err)
	}

	dinero := []string{"carga-usd", "carga-pesos", "carga-cambio", "carga-metodo"}
	for _, id := range dinero {
		if estado(t, ctx, pool, viaje.ID, id) != "abierto" {
			t.Errorf("%s: todavía no hay pago cargado", id)
		}
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO payments (trip_id, total_usd, rate, total_ars, method)
		 VALUES ($1, 3200, 1450.5, 4641600, 'transferencia')`, viaje.ID); err != nil {
		t.Fatalf("cargando el pago: %v", err)
	}
	if err := s.RefreshDerived(ctx, viaje.ID); err != nil {
		t.Fatalf("recalculando derivados: %v", err)
	}
	for _, id := range dinero {
		if estado(t, ctx, pool, viaje.ID, id) != "cerrado" {
			t.Errorf("%s: el pago está cargado, debería cerrarse solo", id)
		}
	}
}

// Recalcular es idempotente y no toca lo que no le corresponde: un control de
// acción cerrado con su evidencia no puede reabrirse porque alguien cargó un
// pasajero.
func TestRecalcularNoTocaLosQueNoSonDerivados(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)

	user := seedUser(t, ctx, pool, "vendedora@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000005@s.whatsapp.net", "Diego Balbuena")
	viaje, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID: contact, Destination: "Iguazú",
		DepartAt: fecha("2026-12-10"), ReturnAt: fecha("2026-12-24"), CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("alta del viaje: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`UPDATE controls SET status = 'cerrado'
		  WHERE trip_id = $1 AND item_id = 'control-visa'`, viaje.ID); err != nil {
		t.Fatalf("cerrando el control de visa: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := s.RefreshDerived(ctx, viaje.ID); err != nil {
			t.Fatalf("recalculando derivados: %v", err)
		}
	}

	if estado(t, ctx, pool, viaje.ID, "control-visa") != "cerrado" {
		t.Error("control-visa: recalcular derivados no puede reabrir un control por acción")
	}
}

func estado(t *testing.T, ctx context.Context, p *pgxpool.Pool, tripID, itemID string) string {
	t.Helper()
	var status string
	err := p.QueryRow(ctx,
		`SELECT status FROM controls WHERE trip_id = $1 AND item_id = $2`,
		tripID, itemID).Scan(&status)
	if err != nil {
		t.Fatalf("leyendo el control %s: %v", itemID, err)
	}
	return status
}

func seedPassenger(t *testing.T, ctx context.Context, p *pgxpool.Pool,
	tripID, first, last string, holder bool) string {
	t.Helper()
	var id string
	err := p.QueryRow(ctx,
		`INSERT INTO passengers (trip_id, first_name, last_name, is_holder)
		 VALUES ($1, $2, $3, $4) RETURNING id`, tripID, first, last, holder).Scan(&id)
	if err != nil {
		t.Fatalf("sembrando pasajero: %v", err)
	}
	return id
}
