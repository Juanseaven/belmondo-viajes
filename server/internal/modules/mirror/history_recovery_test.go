package mirror

import (
	"context"
	"errors"
	"testing"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/worker"
)

type historySourceStub struct {
	jobs []worker.Job
	err  error
}

func (s historySourceStub) PendingHistorySync(context.Context, int) ([]worker.Job, error) {
	return s.jobs, s.err
}

type historySubmitterStub struct {
	jobs []worker.Job
	err  error
}

func (s *historySubmitterStub) Submit(job worker.Job) error {
	if s.err != nil {
		return s.err
	}
	s.jobs = append(s.jobs, job)
	return nil
}

func TestRequeuePendingHistorySyncSubmitsRecoveredJobs(t *testing.T) {
	source := historySourceStub{jobs: []worker.Job{{EventID: 1}, {EventID: 2}}}
	destination := &historySubmitterStub{}

	count, err := requeuePendingHistorySync(context.Background(), source, destination, 200)
	if err != nil {
		t.Fatalf("requeuePendingHistorySync: %v", err)
	}
	if count != 2 || len(destination.jobs) != 2 {
		t.Fatalf("reencolados = %d / %+v", count, destination.jobs)
	}
}

func TestRequeuePendingHistorySyncReportsQueueFailure(t *testing.T) {
	destination := &historySubmitterStub{err: errors.New("queue full")}
	count, err := requeuePendingHistorySync(context.Background(), historySourceStub{jobs: []worker.Job{{EventID: 1}}}, destination, 200)
	if err == nil || count != 0 {
		t.Fatalf("count = %d, err = %v", count, err)
	}
}
