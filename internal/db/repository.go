//go:generate mockgen -source=repository.go -destination=mocks/mock_repository.go -package=mocks

package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Querier
	ExecTx(context.Context, func(Querier) error) error
}

type repository struct {
	*Queries
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{
		Queries: New(pool),
		pool:    pool,
	}
}

func (r *repository) ExecTx(ctx context.Context, fn func(Querier) error) error {
	var attempt int
	for {
		err := r.execTxOnce(ctx, fn)
		if err == nil {
			if attempt > 0 {
				slog.Debug("transaction succeeded after retries", "attempts", attempt+1)
			}
			return nil
		}
		if !isRetryable(err) {
			return err
		}
		attempt++
		if attempt >= maxRetries {
			return fmt.Errorf("transaction failed after %d attempts: %w", maxRetries, err)
		}
		delay := retryDelay(attempt - 1)
		slog.Debug("retrying transaction",
			"attempt", attempt,
			"max", maxRetries,
			"delay_ms", delay.Milliseconds(),
			"error", err,
		)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		}
	}
}

func (r *repository) execTxOnce(ctx context.Context, fn func(Querier) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	qtx := r.Queries.WithTx(tx)
	if err := fn(qtx); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return fmt.Errorf("tx error: %w, rollback error: %v", err, rollbackErr)
		}
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}
