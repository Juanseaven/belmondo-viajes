package postgres

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/db"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/db/dbtest"
	kernelmigrations "github.com/juanseaven/viajes-crm/server/internal/kernel/migrations"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/domain"
	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// Integración del expediente contra Postgres real.
//
// El alta de un viaje no se puede probar con dobles: lo que se verifica es que
// tres escrituras —cliente, viaje y treinta y un controles— ocurran en una sola
// transacción sobre el esquema real, y que las claves foráneas hacia las tablas
// públicas del espejo aguanten.
//
//	TEST_DATABASE_URL='postgres://postgres:clave@127.0.0.1:5432/postgres?sslmode=disable' \
//	    go test ./internal/modules/operaciones/store/postgres/
//
// Sin la variable se saltan: `go test ./...` no puede exigir una base levantada.

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m, "operaciones_postgres"))
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// testDB deja la base con el esquema REAL: las migraciones del kernel, las del
// espejo —de cuyas tablas públicas cuelga este módulo— y las propias.
func testDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	ctx, pool := dbtest.Pool(t)

	if err := db.Migrate(ctx, pool, "kernel", kernelmigrations.FS(), quietLogger()); err != nil {
		t.Fatalf("migraciones del kernel: %v", err)
	}
	if err := db.Migrate(ctx, pool, "mirror",
		os.DirFS("../../../mirror/migrations"), quietLogger()); err != nil {
		t.Fatalf("migraciones del espejo: %v", err)
	}
	if err := db.Migrate(ctx, pool, domain.ModuleName,
		os.DirFS("../../migrations"), quietLogger()); err != nil {
		t.Fatalf("migraciones de operaciones: %v", err)
	}
	return ctx, pool
}

func fecha(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// El alta pide tres campos y hace tres cosas. Que el cliente se cree solo es lo
// que permite que abrir el primer viaje desde el hilo no cueste un paso extra:
// si tuviera que vincularse a mano, nadie lo haría.
func TestAltaCreaElClienteLoVinculaYMaterializaLosControles(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)

	user := seedUser(t, ctx, pool, "vendedora@agencia.test")
	contact := seedContact(t, ctx, pool, "5491133212522@s.whatsapp.net", "Marina Ferreyra")

	viaje, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID:   contact,
		Destination: "Cancún",
		DepartAt:    fecha("2026-12-10"),
		ReturnAt:    fecha("2026-12-24"),
		CreatedBy:   user,
	})
	if err != nil {
		t.Fatalf("alta del viaje: %v", err)
	}

	if viaje.Status != domain.TripCotizado {
		t.Errorf("estado inicial %q, se esperaba %q", viaje.Status, domain.TripCotizado)
	}
	if viaje.Number == 0 {
		t.Error("el viaje no recibió número visible")
	}

	var nombre string
	err = pool.QueryRow(ctx,
		`SELECT c.name FROM customers c
		   JOIN customer_contacts cc ON cc.customer_id = c.id
		  WHERE cc.contact_id = $1`, contact).Scan(&nombre)
	if err != nil {
		t.Fatalf("el contacto quedó sin cliente vinculado: %v", err)
	}
	if nombre != "Marina Ferreyra" {
		t.Errorf("el cliente se llama %q, se esperaba el nombre del contacto", nombre)
	}

	var controles int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM controls WHERE trip_id = $1`, viaje.ID).Scan(&controles); err != nil {
		t.Fatalf("contando controles: %v", err)
	}
	if controles != 31 {
		t.Errorf("se materializaron %d controles, se esperaban 31", controles)
	}
}

// El plazo vive en la plantilla; la fecha se calcula al materializar. Si esto se
// rompe, el chip de la bandeja ordena mal y lo urgente deja de aparecer primero.
func TestAltaCalculaLosVencimientosYDejaSinFechaALosDerivados(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)

	user := seedUser(t, ctx, pool, "vendedora@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000001@s.whatsapp.net", "Diego Balbuena")

	viaje, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID:   contact,
		Destination: "Bariloche",
		DepartAt:    fecha("2026-12-10"),
		ReturnAt:    fecha("2026-12-24"),
		CreatedBy:   user,
	})
	if err != nil {
		t.Fatalf("alta del viaje: %v", err)
	}

	var visa time.Time
	if err := pool.QueryRow(ctx,
		`SELECT due_at FROM controls WHERE trip_id = $1 AND item_id = 'control-visa'`,
		viaje.ID).Scan(&visa); err != nil {
		t.Fatalf("leyendo el control de visa: %v", err)
	}
	if got := visa.Format("2006-01-02"); got != "2026-10-26" {
		t.Errorf("el aviso de visa vence %s, se esperaba 2026-10-26 (salida - 45)", got)
	}

	var derivadosConFecha int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM controls
		  WHERE trip_id = $1 AND closing = 'derivado' AND due_at IS NOT NULL`,
		viaje.ID).Scan(&derivadosConFecha); err != nil {
		t.Fatalf("contando derivados con fecha: %v", err)
	}
	if derivadosConFecha != 0 {
		t.Errorf("%d controles derivados tienen vencimiento; no deberían tener ninguno", derivadosConFecha)
	}
}

// Un cliente puede tener varios viajes a lo largo del tiempo, y puede escribir
// desde más de un número. Lo que no puede es duplicarse porque abrió un segundo
// viaje.
func TestElSegundoViajeDelMismoContactoReutilizaElCliente(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)

	user := seedUser(t, ctx, pool, "vendedora@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000002@s.whatsapp.net", "Grupo Rossi")

	nuevo := store.NewTrip{
		ContactID: contact, Destination: "Iguazú",
		DepartAt: fecha("2026-10-01"), ReturnAt: fecha("2026-10-08"), CreatedBy: user,
	}
	primero, err := s.CreateTrip(ctx, nuevo)
	if err != nil {
		t.Fatalf("primer viaje: %v", err)
	}
	nuevo.Destination = "Salta"
	segundo, err := s.CreateTrip(ctx, nuevo)
	if err != nil {
		t.Fatalf("segundo viaje: %v", err)
	}

	if primero.CustomerID != segundo.CustomerID {
		t.Errorf("el segundo viaje creó otro cliente (%s y %s)", primero.CustomerID, segundo.CustomerID)
	}
	if primero.Number == segundo.Number {
		t.Errorf("los dos viajes comparten el número visible %d", primero.Number)
	}
}

// --- Semillas ---------------------------------------------------------------

func seedUser(t *testing.T, ctx context.Context, p *pgxpool.Pool, email string) string {
	t.Helper()
	var id string
	err := p.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, full_name, role)
		 VALUES ($1, 'x', 'Vendedora', 'admin') RETURNING id`, email).Scan(&id)
	if err != nil {
		t.Fatalf("sembrando usuario: %v", err)
	}
	return id
}

func seedContact(t *testing.T, ctx context.Context, p *pgxpool.Pool, jid, name string) string {
	t.Helper()
	var id string
	err := p.QueryRow(ctx,
		`INSERT INTO contacts (wa_jid, push_name, phone_number)
		 VALUES ($1, $2, '+5491100000000') RETURNING id`, jid, name).Scan(&id)
	if err != nil {
		t.Fatalf("sembrando contacto: %v", err)
	}
	return id
}
