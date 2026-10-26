package postgres

import (
	"testing"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/webhook"
)

func TestPendingHistorySyncReturnsOnlyUnfinishedHistoryEvents(t *testing.T) {
	ctx, pool := testDB(t)
	events := NewEvents(pool)

	pendingID, err := events.Insert(ctx, "line-recovery", webhook.EventHistorySync, "history-recovery-pending", []byte(`{"Data":{"progress":40}}`))
	if err != nil {
		t.Fatalf("insert pending: %v", err)
	}
	failedID, err := events.Insert(ctx, "line-recovery", webhook.EventHistorySync, "history-recovery-failed", []byte(`{"Data":{"progress":60}}`))
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	if err := events.MarkFailed(ctx, failedID, "timeout"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	processedID, err := events.Insert(ctx, "line-recovery", webhook.EventHistorySync, "history-recovery-processed", []byte(`{}`))
	if err != nil {
		t.Fatalf("insert processed: %v", err)
	}
	if err := events.MarkProcessed(ctx, processedID); err != nil {
		t.Fatalf("mark processed: %v", err)
	}
	if _, err := events.Insert(ctx, "line-recovery", webhook.EventMessage, "history-recovery-message", []byte(`{}`)); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	jobs, err := events.PendingHistorySync(ctx, 10)
	if err != nil {
		t.Fatalf("PendingHistorySync: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %+v; se esperaban pending y failed", jobs)
	}
	if jobs[0].EventID != pendingID || jobs[1].EventID != failedID {
		t.Fatalf("ids = %d, %d; se esperaban %d, %d", jobs[0].EventID, jobs[1].EventID, pendingID, failedID)
	}
	for _, job := range jobs {
		if job.Event != webhook.EventHistorySync || job.Instance != "line-recovery" || job.PartitionKey != "line-recovery|" || job.ReceivedAt.IsZero() {
			t.Fatalf("job incompleto: %+v", job)
		}
	}
}
