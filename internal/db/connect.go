package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PoolConfig struct {
	DSN          string
	MaxOpenConns int32
	MinOpenConns int32
	ConnTimeout  time.Duration
}

func DbConnect(ctx context.Context, cfg PoolConfig) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse DB config: %w", err)
	}
	config.MaxConns = cfg.MaxOpenConns
	config.MinConns = cfg.MinOpenConns
	config.ConnConfig.ConnectTimeout = cfg.ConnTimeout
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping DB: %w", err)
	}
	return pool, nil
}
