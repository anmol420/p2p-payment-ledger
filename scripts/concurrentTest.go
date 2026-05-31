//go:build ignore

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/anmol420/p2p-payment-ledger/internal/db"
	"github.com/anmol420/p2p-payment-ledger/internal/env"
	"github.com/anmol420/p2p-payment-ledger/internal/service"
)

func main() {
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
	repo := db.NewRepository(pool)
	svc := service.NewTransferService(repo)
	anmol, _ := repo.CreateAccount(ctx, db.CreateAccountParams{
		OwnerName: "Anmol",
		Balance:   100000,
	})
	mrx, _ := repo.CreateAccount(ctx, db.CreateAccountParams{
		OwnerName: "Mr.X",
		Balance:   50000,
	})
	const numGoroutines = 10
	const transferAmount = int64(200)
	var wg sync.WaitGroup
	errors := make([]error, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.ExecuteTransfer(
				context.Background(),
				anmol.ID,
				mrx.ID,
				transferAmount,
				fmt.Sprintf("concurrent key: %d", i),
			)
			errors[i] = err
		}(i)
	}
	wg.Wait()
	failed := 0
	for _, err := range errors {
		if err != nil {
			fmt.Println("Transfer error:", err)
			failed++
		}
	}
	fmt.Printf("%d succeeded, %d failed\n", numGoroutines-failed, failed)
}
