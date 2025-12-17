package sleep

import (
	"context"
	"time"
)

func Main(ctx context.Context, t time.Duration) error {
	select {
	case <-ctx.Done():
	case <-time.After(t):
	}
	return nil
}
