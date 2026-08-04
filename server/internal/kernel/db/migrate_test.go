package db

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// availableVersions decide el ORDEN en que se aplica el esquema. Es lógica pura,
// así que se prueba sin base.
func TestAvailableVersions(t *testing.T) {
	files := fstest.MapFS{
		"0010_indices.sql": {Data: []byte("")},
		"0002_users.sql":   {Data: []byte("")},
		"0001_init.sql":    {Data: []byte("")},
		"README.md":        {Data: []byte("")}, // no es .sql: se ignora
		"notas.txt":        {Data: []byte("")},
	}

	got, err := availableVersions(files)
	if err != nil {
		t.Fatalf("availableVersions: %v", err)
	}

	// Lexicográfico con prefijo numérico = cronológico. Por eso 0010 va al final
	// y no entre 0001 y 0002, que es lo que pasaría sin ceros a la izquierda.
	assertVersions(t, got, []string{"0001_init.sql", "0002_users.sql", "0010_indices.sql"})
}

func TestAvailableVersionsAceptaFSVacio(t *testing.T) {
	got, err := availableVersions(fstest.MapFS{})
	if err != nil {
		t.Fatalf("availableVersions: %v", err)
	}
	assertVersions(t, got, nil)
}

func assertVersions(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("versiones = %v, se esperaba %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("versiones = %v, se esperaba %v", got, want)
		}
	}
}

// --- Integración ------------------------------------------------------------
//
// Requiere un Postgres de usar y tirar:
//
//	docker run --rm -e POSTGRES_PASSWORD=test -p 5433:5432 postgres:16
//	TEST_DATABASE_URL=postgres://postgres:test@localhost:5433/postgres go test ./internal/kernel/db/
//
// Sin la variable se salta: `go test ./...` no puede exigir una base levantada.

func testPool(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL sin definir: se salta la integración con Postgres")
	}
	ctx := context.Background()
	p, err := Connect(ctx, url, 4)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(p.Close)

	// Cada test parte de cero: el runner se prueba contra una base limpia.
	if _, err := p.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("limpiando el esquema: %v", err)
	}
	return ctx, p
}

func countMigrations(t *testing.T, ctx context.Context, p *pgxpool.Pool, module string) int {
	t.Helper()
	var n int
	if err := p.QueryRow(ctx,
		`SELECT count(*) FROM schema_migrations WHERE module = $1`, module).Scan(&n); err != nil {
		t.Fatalf("contando migraciones de %s: %v", module, err)
	}
	return n
}

func TestMigrateAplicaUnaVezYEsIdempotente(t *testing.T) {
	ctx, p := testPool(t)

	files := fstest.MapFS{
		"0001_init.sql": {Data: []byte(`CREATE TABLE cosas (id int PRIMARY KEY)`)},
	}

	if err := Migrate(ctx, p, "demo", files, quietLogger()); err != nil {
		t.Fatalf("primera migración: %v", err)
	}
	// La segunda pasada NO puede reejecutar el CREATE TABLE: sin el registro en
	// schema_migrations, esto fallaría con "relation already exists".
	if err := Migrate(ctx, p, "demo", files, quietLogger()); err != nil {
		t.Fatalf("segunda migración (debía ser no-op): %v", err)
	}

	if n := countMigrations(t, ctx, p, "demo"); n != 1 {
		t.Fatalf("filas en schema_migrations = %d, se esperaba 1", n)
	}
}

// El punto del diseño: dos módulos numeran su 0001 sin coordinarse.
func TestMigrateAislaLosModulosEntreSi(t *testing.T) {
	ctx, p := testPool(t)

	moduloA := fstest.MapFS{"0001_init.sql": {Data: []byte(`CREATE TABLE a (id int)`)}}
	moduloB := fstest.MapFS{"0001_init.sql": {Data: []byte(`CREATE TABLE b (id int)`)}}

	if err := Migrate(ctx, p, "a", moduloA, quietLogger()); err != nil {
		t.Fatalf("migrando a: %v", err)
	}
	if err := Migrate(ctx, p, "b", moduloB, quietLogger()); err != nil {
		t.Fatalf("migrando b: %v", err)
	}

	for _, tabla := range []string{"a", "b"} {
		var exists bool
		if err := p.QueryRow(ctx,
			`SELECT to_regclass($1) IS NOT NULL`, "public."+tabla).Scan(&exists); err != nil {
			t.Fatalf("comprobando la tabla %s: %v", tabla, err)
		}
		if !exists {
			t.Fatalf("la tabla %s no se creó: un módulo pisó al otro", tabla)
		}
	}
}

// Una migración que falla no puede quedar registrada como aplicada: si lo hiciera,
// el arreglo no volvería a intentarse nunca.
func TestMigrateNoRegistraLoQueFallo(t *testing.T) {
	ctx, p := testPool(t)

	rota := fstest.MapFS{"0001_init.sql": {Data: []byte(`ESTO NO ES SQL`)}}
	if err := Migrate(ctx, p, "demo", rota, quietLogger()); err == nil {
		t.Fatal("Migrate no devolvió error con SQL inválido")
	}

	if n := countMigrations(t, ctx, p, "demo"); n != 0 {
		t.Fatalf("se registró una migración fallida (%d filas): nunca se reintentaría", n)
	}
}

// Añadir un archivo nuevo aplica solo ese, no reaplica los anteriores.
func TestMigrateAplicaSoloLoNuevo(t *testing.T) {
	ctx, p := testPool(t)
	log := quietLogger()

	const init = `CREATE TABLE cosas (id int PRIMARY KEY)`

	if err := Migrate(ctx, p, "demo", fstest.MapFS{
		"0001_init.sql": {Data: []byte(init)},
	}, log); err != nil {
		t.Fatalf("migración 1: %v", err)
	}

	if err := Migrate(ctx, p, "demo", fstest.MapFS{
		"0001_init.sql":    {Data: []byte(init)},
		"0002_columna.sql": {Data: []byte(`ALTER TABLE cosas ADD COLUMN nombre text`)},
	}, log); err != nil {
		t.Fatalf("migración 2: %v", err)
	}

	if n := countMigrations(t, ctx, p, "demo"); n != 2 {
		t.Fatalf("filas en schema_migrations = %d, se esperaba 2", n)
	}
}
