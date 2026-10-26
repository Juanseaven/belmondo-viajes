package mirror

import (
	"context"
	"fmt"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/worker"
)

type pendingHistorySource interface {
	PendingHistorySync(ctx context.Context, limit int) ([]worker.Job, error)
}

type historyJobSubmitter interface {
	Submit(job worker.Job) error
}

func requeuePendingHistorySync(
	ctx context.Context,
	source pendingHistorySource,
	destination historyJobSubmitter,
	limit int,
) (int, error) {
	jobs, err := source.PendingHistorySync(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("listando HistorySync pendientes: %w", err)
	}
	for i, job := range jobs {
		if err := destination.Submit(job); err != nil {
			return i, fmt.Errorf("reencolando HistorySync %d: %w", job.EventID, err)
		}
	}
	return len(jobs), nil
}
