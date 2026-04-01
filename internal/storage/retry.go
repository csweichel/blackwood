package storage

import (
	"context"
	"errors"
	"log/slog"
	"time"

	sqlite3 "modernc.org/sqlite/lib"

	"modernc.org/sqlite"
)

const (
	retryMaxAttempts = 5
	retryBaseDelay   = 50 * time.Millisecond
)

// isBusy returns true if the error is a SQLITE_BUSY or SQLITE_LOCKED error.
func isBusy(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		code := sqliteErr.Code()
		return code == sqlite3.SQLITE_BUSY ||
			code == sqlite3.SQLITE_BUSY_RECOVERY ||
			code == sqlite3.SQLITE_BUSY_SNAPSHOT ||
			code == sqlite3.SQLITE_LOCKED
	}
	return false
}

// retryOnBusy retries fn on SQLITE_BUSY/SQLITE_LOCKED with exponential backoff.
// It respects context cancellation. Exported for use by other packages sharing
// the same database (e.g., the index package).
func RetryOnBusy(ctx context.Context, fn func() error) error {
	var err error
	for attempt := range retryMaxAttempts {
		err = fn()
		if err == nil || !isBusy(err) {
			return err
		}

		delay := retryBaseDelay << attempt // 50, 100, 200, 400, 800ms
		slog.Debug("retrying after SQLITE_BUSY", "attempt", attempt+1, "delay", delay)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}
