package db

import (
	"errors"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	maxRetries     = 5
	baseDelay      = 5 * time.Millisecond
	maxDelay       = 500 * time.Millisecond
	jitterFraction = 0.3
)

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case "40001":
			return true
		case "40P01":
			return true
		}
	}
	return false
}

func retryDelay(attempt int) time.Duration {
	base := float64(baseDelay) * float64(int(1)<<attempt)
	jitter := base * jitterFraction * (rand.Float64()*2 - 1)
	delay := time.Duration(base + jitter)
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}
