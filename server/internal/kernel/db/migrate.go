package db

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate aplica las migraciones pendientes de un módulo.
//
// El nombre del módulo forma parte de la clave, así que cada uno numera su 0001
// sin coordinarse con nadie: es lo que permite agregar un módulo sin editar el
// SQL de otro.
//
// Se llama una vez por módulo, en orden, y el kernel va primero porque los demás
// tienen claves foráneas contra sus tablas.
func Migrate(ctx context.Context, pool *pgxpool.Pool, module string, files fs.FS, log *slog.Logger) error {
	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return err
	}

	applied, err := appliedVersions(ctx, pool, module)
	if err != nil {
		return err
	}

	versions, err := availableVersions(files)
	if err != nil {
		return fmt.Errorf("módulo %s: %w", module, err)
	}

	for _, version := range versions {
		if applied[version] {
			continue
		}
		sqlText, err := fs.ReadFile(files, version)
		if err != nil {
			return fmt.Errorf("módulo %s: leyendo %s: %w", module, version, err)
		}
		if err := applyOne(ctx, pool, module, version, string(sqlText)); err != nil {
			return err
		}
		log.Info("migración aplicada", "module", module, "version", version)
	}
	return nil
}

// ensureMigrationsTable crea la tabla de control fuera del sistema de
// migraciones. No puede ser una migración de sí misma: es justamente lo que
// decide qué migraciones faltan.
func ensureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	const q = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
		    module     text        NOT NULL,
		    version    text        NOT NULL,
		    applied_at timestamptz NOT NULL DEFAULT now(),
		    PRIMARY KEY (module, version)
		)`
	if _, err := pool.Exec(ctx, q); err != nil {
		return fmt.Errorf("creando schema_migrations: %w", err)
	}
	return nil
}

func appliedVersions(ctx context.Context, pool *pgxpool.Pool, module string) (map[string]bool, error) {
	rows, err := pool.Query(ctx,
		`SELECT version FROM schema_migrations WHERE module = $1`, module)
	if err != nil {
		return nil, fmt.Errorf("leyendo migraciones aplicadas de %s: %w", module, err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// availableVersions lista los .sql del FS en orden lexicográfico, que con el
// prefijo numérico (0001_, 0002_) es el orden cronológico.
func availableVersions(files fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("listando migraciones: %w", err)
	}
	var versions []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			versions = append(versions, e.Name())
		}
	}
	sort.Strings(versions)
	return versions, nil
}

// applyOne corre una migración y registra su versión en la MISMA transacción.
//
// Si se registrara aparte, un corte entre ambas cosas dejaría la base en un
// estado que el runner no sabe interpretar: o reaplica un DDL que ya corrió, o da
// por aplicado algo que no lo está.
func applyOne(ctx context.Context, pool *pgxpool.Pool, module, version, sqlText string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abriendo transacción para %s/%s: %w", module, version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, sqlText); err != nil {
		return fmt.Errorf("aplicando %s/%s: %w", module, version, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (module, version) VALUES ($1, $2)`,
		module, version,
	); err != nil {
		return fmt.Errorf("registrando %s/%s: %w", module, version, err)
	}
	return tx.Commit(ctx)
}
