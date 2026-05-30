package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/anmol420/p2p-payment-ledger/internal/db"
	"github.com/anmol420/p2p-payment-ledger/internal/env"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	DATABASE_ADDR := env.StringGetEnv("DATABASE_ADDR", slog.Default())

	pool, err := db.DbConnect(ctx, DATABASE_ADDR)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("DB connection successful")

	// smoke test
	repo := db.NewRepository(pool)
	acc, err := repo.CreateAccount(ctx, db.CreateAccountParams{
		OwnerName: "Anmol",
		Balance: 2000,
	})
	if err != nil {
		slog.Error("create account failed", "error", err)
		os.Exit(1)
	}
	slog.Info("account created", "id", acc.ID, "owner", acc.OwnerName, "balance", acc.Balance)
	fetched, err := repo.GetAccount(ctx, acc.ID)
	if err != nil {
		slog.Error("get account failed", "error", err)
		os.Exit(1)
	}
	slog.Info("account fetched", "id", fetched.ID, "balance", fetched.Balance)
	// test done
	
	<-ctx.Done()
	slog.Info("Server shutting down")
}