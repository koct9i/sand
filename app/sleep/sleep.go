package sleep

import (
	"context"
	"time"

	"github.com/go-logr/logr"
)

func Main(ctx context.Context, t time.Duration) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info("Sleep begin", "duration", t)
	select {
	case <-ctx.Done():
	case <-time.After(t):
	}
	logger.Info("Sleep finished", "duration", t)
	return nil
}
