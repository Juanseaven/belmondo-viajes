// Package dbtest levanta bases de datos aisladas para los tests de integración.
//
// Existe por un fallo concreto: `go test ./...` corre los paquetes EN PARALELO.
// Con todos apuntando a la misma base y limpiando el esquema al empezar, dos
// paquetes se pisan y fallan con errores incomprensibles ("llave duplicada en
// pg_type_typname_nsp_index"), que además solo aparecen a veces.
//
// La solución es que cada paquete de test tenga su propia base. Aísla también las
// extensiones, que son objetos de base y no de esquema.
//
// No importa nada del proyecto —solo pgxpool— para que cualquier paquete pueda
// usarlo sin crear ciclos de imports, incluido el propio kernel/db.
package dbtest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnvVar es la variable con la URL de un Postgres donde crear y borrar bases.
const EnvVar = "TEST_DATABASE_URL"

// dbURL es la URL de la base exclusiva de este paquete de test. La rellena Main.
var dbURL string

// Main crea una base para el paquete, corre sus tests y la borra.
//
// Se usa desde TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(dbtest.Main(m, "mirror_postgres")) }
//
// `name` debe ser único por paquete: es lo que evita la colisión.
func Main(m *testing.M, name string) int {
	admin := os.Getenv(EnvVar)
	if admin == "" {
		// Sin base configurada no hay nada que preparar; cada test se salta solo
		// al llamar a Pool.
		return m.Run()
	}

	dbName := "viajes_crm_test_" + name
	if err := recreateDatabase(admin, dbName); err != nil {
		fmt.Fprintf(os.Stderr, "dbtest: %v\n", err)
		return 1
	}

	var err error
	if dbURL, err = withDatabase(admin, dbName); err != nil {
		fmt.Fprintf(os.Stderr, "dbtest: %v\n", err)
		return 1
	}

	code := m.Run()

	// El borrado es best-effort: que falle no puede cambiar el resultado de los
	// tests, y la base sobrante se recrea en la corrida siguiente.
	if err := dropDatabase(admin, dbName); err != nil {
		fmt.Fprintf(os.Stderr, "dbtest: no se pudo borrar %s: %v\n", dbName, err)
	}
	return code
}

// Pool devuelve un pool sobre la base del paquete, con el esquema `public` recién
// creado. Se salta el test si no hay base configurada.
//
// Cada test arranca de cero: dentro de un paquete los tests son secuenciales, así
// que limpiar el esquema acá es seguro.
func Pool(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	if dbURL == "" {
		t.Skipf("%s sin definir: se salta la integración con Postgres", EnvVar)
	}

	ctx := context.Background()
	pool, err := connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("conectando a la base de test: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("limpiando el esquema: %v", err)
	}
	return ctx, pool
}

func connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 4
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func recreateDatabase(adminURL, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := connect(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("conectando para crear %s: %w", name, err)
	}
	defer pool.Close()

	// WITH (FORCE) corta las conexiones que hubieran quedado de una corrida
	// anterior interrumpida; sin eso, el DROP se queda esperando.
	if _, err := pool.Exec(ctx, `DROP DATABASE IF EXISTS `+quote(name)+` WITH (FORCE)`); err != nil {
		return fmt.Errorf("borrando %s: %w", name, err)
	}
	if _, err := pool.Exec(ctx, `CREATE DATABASE `+quote(name)); err != nil {
		return fmt.Errorf("creando %s: %w", name, err)
	}
	return nil
}

func dropDatabase(adminURL, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := connect(ctx, adminURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, `DROP DATABASE IF EXISTS `+quote(name)+` WITH (FORCE)`)
	return err
}

// withDatabase cambia el nombre de base de una URL conservando el resto.
func withDatabase(raw, name string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s inválida: %w", EnvVar, err)
	}
	u.Path = "/" + name
	return u.String(), nil
}

// quote entrecomilla un identificador. Los nombres los fija el código y no la
// entrada del usuario, pero un identificador sin comillas se plegaría a
// minúsculas y rompería el borrado.
func quote(id string) string {
	return `"` + strings.ReplaceAll(id, `"`, `""`) + `"`
}
