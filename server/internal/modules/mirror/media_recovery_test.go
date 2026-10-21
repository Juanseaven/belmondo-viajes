package mirror

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestMediaRecoveryLoopRunsPeriodicallyAndStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var calls atomic.Int32
	go func() {
		runMediaRecoveryLoop(ctx, time.Millisecond, func(context.Context) error {
			calls.Add(1)
			return nil
		})
		close(done)
	}()

	deadline := time.After(time.Second)
	for calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("periodic media recovery did not run twice")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("periodic media recovery did not stop")
	}
}
