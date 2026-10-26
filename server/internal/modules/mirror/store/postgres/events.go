package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/store"
	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/worker"
)

type Events struct{ db *pgxpool.Pool }

func NewEvents(db *pgxpool.Pool) *Events { return &Events{db: db} }

// Insert registra el webhook crudo.
//
// La idempotencia se delega en el índice único de dedupe_key, no en un SELECT
// previo: dos reintentos concurrentes de Evolution pasarían ambos la
// comprobación y solo uno chocaría contra el índice. Dejar que la base decida
// elimina la carrera.
func (e *Events) Insert(ctx context.Context, instance, event, dedupeKey string, payload []byte) (int64, error) {
	const q = `
		INSERT INTO webhook_events (instance_name, event, dedupe_key, payload)
		VALUES ($1, $2, $3, $4)
		RETURNING id`

	var id int64
	err := e.db.QueryRow(ctx, q, instance, event, dedupeKey, payload).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, store.ErrDuplicate
		}
		return 0, err
	}
	return id, nil
}

func (e *Events) MarkProcessed(ctx context.Context, id int64) error {
	const q = `
		UPDATE webhook_events
		   SET status = 'processed', processed_at = now(),
		       payload = payload #- '{data,Message,base64}' #- '{data,Message,mediaUrl}'
		 WHERE id = $1`
	_, err := e.db.Exec(ctx, q, id)
	return err
}

func (e *Events) MarkFailed(ctx context.Context, id int64, cause string) error {
	const q = `
		UPDATE webhook_events
		   SET status = 'failed', attempts = attempts + 1, last_error = $2
		 WHERE id = $1`
	_, err := e.db.Exec(ctx, q, id, cause)
	return err
}

// PendingHistorySync recupera lotes de historial que un reinicio o timeout dejó
// sin terminar. Persistir mensajes es idempotente, por lo que repetir la parte
// ya importada es seguro.
func (e *Events) PendingHistorySync(ctx context.Context, limit int) ([]worker.Job, error) {
	const q = `
		SELECT id, event, instance_name, payload, received_at
		  FROM webhook_events
		 WHERE event = 'HistorySync'
		   AND status IN ('pending', 'failed')
		 ORDER BY received_at, id
		 LIMIT $1`
	rows, err := e.db.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]worker.Job, 0)
	for rows.Next() {
		var job worker.Job
		if err := rows.Scan(&job.EventID, &job.Event, &job.Instance, &job.Payload, &job.ReceivedAt); err != nil {
			return nil, err
		}
		job.PartitionKey = job.Instance + "|"
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}
