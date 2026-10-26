package worker

import (
	"context"
	"log/slog"
	"time"
)

// UserCounter counts stored users. port.UserRepository satisfies it.
type UserCounter interface {
	Count(ctx context.Context) (int64, error)
}

// RunUserCount logs the total number of users on every tick (req. 6). It
// doesn't own a ticker: main passes ticker.C, and tests pass a channel they
// control (D16). A failed count is logged and the next tick tries again.
// It returns when ctx is cancelled or ticks is closed.
func RunUserCount(ctx context.Context, ticks <-chan time.Time, counter UserCounter, logger *slog.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok {
				return
			}
			n, err := counter.Count(ctx)
			if err != nil {
				logger.ErrorContext(ctx, "user count failed", "error", err)
				continue
			}
			logger.InfoContext(ctx, "user count", "total_users", n)
		}
	}
}
