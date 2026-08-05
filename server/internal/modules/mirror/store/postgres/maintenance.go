package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Maintenance son las tareas de fondo del módulo.
type Maintenance struct{ db *pgxpool.Pool }

func NewMaintenance(db *pgxpool.Pool) *Maintenance { return &Maintenance{db: db} }

// PurgeEvents borra los webhooks ya resueltos anteriores a `before`.
//
// webhook_events guarda el payload JSONB completo de cada evento, incluidos los
// ACK de entrega y lectura, que multiplican el volumen de mensajes. Sin purga la
// tabla crece del orden de millones de filas al mes: el problema no es que
// consulte lento, es que llena el disco.
//
// Solo se borran los eventos en estado 'processed' o 'skipped'. Los 'pending' y
// 'failed' son justamente los que un reprocesador tendría que recoger, así que
// borrarlos por antigüedad perdería datos en silencio.
//
// El borrado va por lotes acotados con un subselect: un DELETE único sobre
// millones de filas mantendría una transacción abierta minutos, bloqueando la
// ingesta y dejando la tabla hinchada.
func (m *Maintenance) PurgeEvents(ctx context.Context, before time.Time, batch int) (int64, error) {
	if batch < 1 {
		batch = 1000
	}

	const q = `
		DELETE FROM webhook_events
		 WHERE id IN (
		     SELECT id FROM webhook_events
		      WHERE received_at < $1
		        AND status IN ('processed', 'skipped')
		      ORDER BY id
		      LIMIT $2
		 )`

	var total int64
	for {
		tag, err := m.db.Exec(ctx, q, before, batch)
		if err != nil {
			return total, fmt.Errorf("purgando webhook_events: %w", err)
		}
		total += tag.RowsAffected()

		// Un lote incompleto significa que ya no queda nada más viejo que `before`.
		if tag.RowsAffected() < int64(batch) {
			return total, nil
		}

		// Respiro entre lotes: la purga es trabajo de fondo y no tiene ninguna
		// prisa por delante de la ingesta.
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// isTimeout distingue una consulta cortada por statement_timeout de un fallo
// real. 57014 es query_canceled.
func isTimeout(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "57014"
}
