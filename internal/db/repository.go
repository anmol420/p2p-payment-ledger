package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Querier
	ExecTx(context.Context, func(Querier) error) error
}

type repository struct {
	*Queries
	pool		*pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{
		Queries: New(pool),
		pool: pool,
	}
}

func (r *repository) ExecTx(ctx context.Context, fn func(Querier) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
	if err != nil {
		return fmt.Errorf("Begin tx: %w", err)
	}
	qtx := r.Queries.WithTx(tx)
	if err := fn(qtx); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return fmt.Errorf("tx error: %w, rollback error: %v", err, rollbackErr)
		}
		return err
	}
	return tx.Commit(ctx)
}
