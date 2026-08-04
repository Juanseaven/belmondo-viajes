// Package db es el acceso a Postgres del kernel: abre el pool y aplica las
// migraciones de cada módulo.
//
// Los módulos reciben el *pgxpool.Pool ya abierto y escriben sus propias
// consultas; el kernel no intenta ser un ORM ni conocer sus tablas.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect abre el pool. maxConns debe quedar por debajo del max_connections de
// Postgres dividido entre las réplicas del backend.
func Connect(ctx context.Context, url string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL inválida: %w", err)
	}
	cfg.MaxConns = maxConns
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creando el pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres inalcanzable: %w", err)
	}
	return pool, nil
}
