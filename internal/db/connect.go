package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func DbConnect(ctx context.Context, db string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(db)
	if err != nil {
		return nil, fmt.Errorf("Parse DB config: %w", err)
	}
	config.MaxConns = 25
	config.MinConns = 5
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("Create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("Ping DB: %w", err)
	}
	return pool, nil
}
