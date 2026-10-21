package mirror

import (
	"context"
	"time"
)

const mediaRecoveryInterval = time.Minute

func runMediaRecoveryLoop(
	ctx context.Context,
	interval time.Duration,
	recoverPending func(context.Context) error,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = recoverPending(ctx)
		}
	}
}
