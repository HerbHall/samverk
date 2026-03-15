package logstore

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// StartPruner runs a background goroutine that prunes log entries older
// than the given retention duration. It prunes once immediately, then
// every 24 hours. The goroutine exits when ctx is cancelled.
func StartPruner(ctx context.Context, store *LogStore, retention time.Duration, logger *zap.Logger) {
	prune := func() {
		deleted, err := store.Prune(ctx, retention)
		if err != nil {
			logger.Warn("log pruner failed", zap.Error(err))
			return
		}
		if deleted > 0 {
			logger.Info("log pruner completed", zap.Int64("deleted", deleted), zap.Duration("retention", retention))
		}
	}

	// Prune immediately on start.
	prune()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prune()
		}
	}
}
