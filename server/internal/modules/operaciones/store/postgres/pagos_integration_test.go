package postgres

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/db"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/db/dbtest"
	kernelmigrations "github.com/juanseaven/viajes-crm/server/internal/kernel/migrations"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// Los pagos registrados: cada plata que entró suma a la barra, y la seña se da
// por cobrada sola cuando los pagos llegan a su monto.

func pay(t *testing.T, s *Store, tripID, user string, amount float64, currency string, rate *float64, paidAt string) store.PaymentRecord {
	t.Helper()
	r, err := s.AddPaymentRecord(t.Context(), store.NewPaymentRecord{TripID: tripID, Amount: amount, Currency: currency,
		Rate: rate, PaidAt: fecha(paidAt), Method: "Transferencia", CreatedBy: user})
	if err != nil {
		t.Fatalf("pago de %v %s: %v", amount, currency, err)
	}
	return r
}

func depositPaidAt(t *testing.T, s *Store, tripID string) *time.Time {
	t.Helper()
	p, err := readPayment(t.Context(), s.pool, tripID)
	if err != nil || p == nil {
		t.Fatalf("leyendo la cotización: %v", err)
	}
	return p.DepositPaidAt
}

func TestUnPagoEnPesosGuardaSuEquivalenteEnDolares(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "pagos@agencia.test")
	cliente := newCustomer(t, s, user, store.NewCustomer{Name: "Marta"})
	trip := tripFor(t, s, user, cliente.ID, "Japón")

	r := pay(t, s, trip.ID, user, 1513000, "ARS", float64ptr(1513), "2026-10-21")
	if r.AmountUSD != 1000 || r.Currency != "ARS" || r.Rate == nil || *r.Rate != 1513 || r.CreatedByName == "" {
		t.Fatalf("pago en pesos = %+v", r)
	}
	pay(t, s, trip.ID, user, 1800, "USD", nil, "2026-10-12")

	detail, err := s.TripDetail(ctx, trip.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.PaidUSD != 2800 || len(detail.PaymentRecords) != 2 || detail.PaymentRecords[0].ID != r.ID {
		t.Fatalf("pagado = %v, pagos = %+v; el más nuevo primero", detail.PaidUSD, detail.PaymentRecords)
	}

	if _, err := s.AddPaymentRecord(ctx, store.NewPaymentRecord{TripID: trip.ID, Amount: 100, Currency: "ARS",
		PaidAt: fecha("2026-10-21"), CreatedBy: user}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("un pago en pesos sin cambio: %v", err)
	}
	if _, err := s.AddPaymentRecord(ctx, store.NewPaymentRecord{TripID: trip.ID, Amount: 0, Currency: "USD",
		PaidAt: fecha("2026-10-21"), CreatedBy: user}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("un pago de cero: %v", err)
	}

	page, err := s.ListTrips(ctx, store.TripFilter{Statuses: []domain.TripStatus{domain.TripConsulta}})
	if err != nil || len(page.Items) != 1 || page.Items[0].PaidUSD != 2800 {
		t.Fatalf("la lista trae lo pagado: %+v, %v", page.Items, err)
	}
}

func TestLaSeñaSeCobraConElPagoQueLaAlcanza(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "pagos@agencia.test")
	cliente := newCustomer(t, s, user, store.NewCustomer{Name: "Marta"})
	trip := tripFor(t, s, user, cliente.ID, "Japón")
	due := fecha("2026-10-30")
	cotizacion := store.Payment{TripID: trip.ID, TotalUSD: float64ptr(5000), Rate: float64ptr(1513.37),
		DepositUSD: float64ptr(1500), DepositDueAt: &due, UpdatedBy: user}
	if _, err := s.PutPayment(ctx, cotizacion); err != nil {
		t.Fatal(err)
	}

	pay(t, s, trip.ID, user, 500, "USD", nil, "2026-10-10")
	if got := depositPaidAt(t, s, trip.ID); got != nil {
		t.Fatalf("con USD 500 de 1.500 la seña quedó cobrada el %v", got)
	}
	// $ 1.513.000 al cambio de 1.513,37 son USD 999,76: con los 500 suma
	// 1.499,76, corto por centavos, y cuenta igual.
	alcanza := pay(t, s, trip.ID, user, 1513000, "ARS", float64ptr(1513.37), "2026-10-15")
	if alcanza.AmountUSD != 999.76 {
		t.Fatalf("equivalente = %v", alcanza.AmountUSD)
	}
	got := depositPaidAt(t, s, trip.ID)
	if got == nil || got.In(agencyLocation(t)).Format("2006-01-02") != "2026-10-15" {
		t.Fatalf("la seña se cobró el %v, se esperaba el 15/10 (con tolerancia de centavos)", got)
	}

	// Volver a guardar la cotización no la refecha ni la borra.
	cotizacion.Method = "Efectivo"
	if _, err := s.PutPayment(ctx, cotizacion); err != nil {
		t.Fatal(err)
	}
	if again := depositPaidAt(t, s, trip.ID); again == nil || !again.Equal(*got) {
		t.Fatalf("reeditar la cotización movió la seña a %v", again)
	}

	// Subir la seña la vuelve a dejar sin cobrar; borrar el pago, también.
	cotizacion.DepositUSD = float64ptr(2000)
	if _, err := s.PutPayment(ctx, cotizacion); err != nil {
		t.Fatal(err)
	}
	if after := depositPaidAt(t, s, trip.ID); after != nil {
		t.Fatalf("una seña más alta quedó cobrada el %v", after)
	}
	cotizacion.DepositUSD = float64ptr(1500)
	if _, err := s.PutPayment(ctx, cotizacion); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeletePaymentRecord(ctx, alcanza.ID); err != nil {
		t.Fatal(err)
	}
	if after := depositPaidAt(t, s, trip.ID); after != nil {
		t.Fatalf("borrar el pago dejó la seña cobrada el %v", after)
	}
}

func agencyLocation(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// La migración convierte en pago la seña cobrada con monto, deja marcada la
// que se cobró sin monto y recalcula el total en pesos.
func TestLaMigracionDeLosPagosConservaLasSeñasCobradas(t *testing.T) {
	ctx, pool := dbtest.Pool(t)
	if err := db.Migrate(ctx, pool, "kernel", kernelmigrations.FS(), quietLogger()); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, pool, "mirror", os.DirFS("../../../mirror/migrations"), quietLogger()); err != nil {
		t.Fatal(err)
	}
	before := fstest.MapFS{}
	entries, err := fs.ReadDir(os.DirFS("../../migrations"), ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "0010_") {
			continue
		}
		data, err := os.ReadFile("../../migrations/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		before[e.Name()] = &fstest.MapFile{Data: data}
	}
	if err := db.Migrate(ctx, pool, domain.ModuleName, before, quietLogger()); err != nil {
		t.Fatalf("migraciones anteriores: %v", err)
	}

	s := New(pool)
	user := seedUser(t, ctx, pool, "viejo@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000901@s.whatsapp.net", "Viejo")
	conMonto, err := s.CreateTrip(ctx, store.NewTrip{ContactID: contact, CreatedBy: user})
	if err != nil {
		t.Fatal(err)
	}
	sinMonto, err := s.CreateTrip(ctx, store.NewTrip{ContactID: contact, CreatedBy: user})
	if err != nil {
		t.Fatal(err)
	}
	// Así quedaban con el tilde de «seña cobrada» y el total en pesos a mano.
	if _, err := pool.Exec(ctx, `
		INSERT INTO payments (trip_id, total_usd, rate, total_ars, deposit_usd, deposit_due_at, deposit_paid_at, method)
		VALUES ($1, 5000, 1513, 100000, 800, '2026-10-01', '2026-10-05 15:00-03', 'Efectivo'),
		       ($2, NULL, NULL, NULL, NULL, '2026-10-01', '2026-10-06 10:00-03', NULL)`,
		conMonto.ID, sinMonto.ID); err != nil {
		t.Fatal(err)
	}

	if err := db.Migrate(ctx, pool, domain.ModuleName, os.DirFS("../../migrations"), quietLogger()); err != nil {
		t.Fatalf("migración de pagos: %v", err)
	}

	detail, err := s.TripDetail(ctx, conMonto.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.PaymentRecords) != 1 || detail.PaymentRecords[0].AmountUSD != 800 ||
		detail.PaymentRecords[0].PaidAt.Format("2006-01-02") != "2026-10-05" || detail.PaymentRecords[0].Note != "Seña" {
		t.Fatalf("la seña cobrada no pasó a ser un pago: %+v", detail.PaymentRecords)
	}
	if detail.Payment.TotalARS == nil || *detail.Payment.TotalARS != 7565000 {
		t.Fatalf("total en pesos = %v, se esperaba 7565000", detail.Payment.TotalARS)
	}

	// La cobrada sin monto conserva su fecha aunque después se le cargue uno.
	legacy, err := readPayment(ctx, pool, sinMonto.ID)
	if err != nil || legacy == nil || !legacy.DepositPaidBefore || legacy.DepositPaidAt == nil {
		t.Fatalf("seña sin monto = %+v, %v", legacy, err)
	}
	legacy.DepositUSD = float64ptr(300)
	legacy.UpdatedBy = user
	saved, err := s.PutPayment(ctx, *legacy)
	if err != nil {
		t.Fatal(err)
	}
	if saved.DepositPaidAt == nil || !saved.DepositPaidBefore {
		t.Fatalf("cargarle monto a la seña vieja la volvió a pedir: %+v", saved)
	}
	// Cargarla de nuevo desde cero le saca la marca.
	saved.DepositUSD, saved.DepositDueAt = nil, nil
	saved.UpdatedBy = user
	if saved, err = s.PutPayment(ctx, saved); err != nil || saved.DepositPaidBefore || saved.DepositPaidAt != nil {
		t.Fatalf("seña cargada de nuevo = %+v, %v", saved, err)
	}
}
