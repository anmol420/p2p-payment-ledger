package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/anmol420/p2p-payment-ledger/internal/db"
	"github.com/anmol420/p2p-payment-ledger/internal/observability"
	"github.com/anmol420/p2p-payment-ledger/internal/service"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("ledger_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})
	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	runMigrations(t, connStr)
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool
}

func runMigrations(t *testing.T, connStr string) {
	t.Helper()
	m, err := migrate.New(
		"file://../../cmd/migrate/migrations",
		connStr,
	)
	require.NoError(t, err, "failed to create migrator")
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("failed to run migrations: %v", err)
	}
}

func TestExecuteTransfer_Integration(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := db.NewRepository(pool)
	metrics := observability.NewMetrics()
	svc := service.NewTransferService(repo, metrics)
	anmol, err := repo.CreateAccount(ctx, db.CreateAccountParams{
		OwnerName: "Anmol",
		Balance:   100000,
	})
	require.NoError(t, err)
	mrx, err := repo.CreateAccount(ctx, db.CreateAccountParams{
		OwnerName: "mrx",
		Balance:   50000,
	})
	require.NoError(t, err)
	t.Run("successful transfer updates both balances", func(t *testing.T) {
		result, err := svc.ExecuteTransfer(ctx,
			anmol.ID, mrx.ID, 20000, "integ-key-001")
		require.NoError(t, err)
		assert.Equal(t, int64(80000), result.FromAccount.Balance)
		assert.Equal(t, int64(70000), result.ToAccount.Balance)
		dbAnmol, err := repo.GetAccount(ctx, anmol.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(80000), dbAnmol.Balance)
		dbmrx, err := repo.GetAccount(ctx, mrx.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(70000), dbmrx.Balance)
	})
	t.Run("idempotency — same key returns same result without double charge", func(t *testing.T) {
		result1, err := svc.ExecuteTransfer(ctx,
			anmol.ID, mrx.ID, 5000, "integ-key-002")
		require.NoError(t, err)
		result2, err := svc.ExecuteTransfer(ctx,
			anmol.ID, mrx.ID, 5000, "integ-key-002")
		require.NoError(t, err)
		assert.Equal(t, result1.Transaction.ID, result2.Transaction.ID,
			"idempotent call must return the same transaction ID")
		var count int
		err = pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM transactions WHERE idempotency_key = $1",
			"integ-key-002",
		).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "must have exactly one transaction row")
	})
	t.Run("insufficient funds — no balance change", func(t *testing.T) {
		before, err := repo.GetAccount(ctx, anmol.ID)
		require.NoError(t, err)
		_, err = svc.ExecuteTransfer(ctx,
			anmol.ID, mrx.ID, 999999999, "integ-key-003")
		require.Error(t, err)
		require.True(t, errors.Is(err, service.ErrInsufficientFunds))
		after, err := repo.GetAccount(ctx, anmol.ID)
		require.NoError(t, err)
		assert.Equal(t, before.Balance, after.Balance,
			"failed transfer must not change any balance")
	})
	t.Run("transaction is recorded in DB after successful transfer", func(t *testing.T) {
		result, err := svc.ExecuteTransfer(ctx,
			anmol.ID, mrx.ID, 1000, "integ-key-004")
		require.NoError(t, err)
		txn, err := repo.GetTransaction(ctx, result.Transaction.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(1000), txn.Amount)
		assert.Equal(t, "integ-key-004", txn.IdempotencyKey)
		assert.Equal(t, db.TransactionStatusCompleted, txn.Status)
	})
}

func TestExecuteTransfer_Concurrent(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := db.NewRepository(pool)
	metrics := observability.NewMetrics()
	svc := service.NewTransferService(repo, metrics)
	const (
		numTransfers   = 100
		amountEach     = int64(100)
		initialBalance = int64(numTransfers * int(amountEach))
	)
	sender, err := repo.CreateAccount(ctx, db.CreateAccountParams{
		OwnerName: "Sender",
		Balance:   initialBalance,
	})
	require.NoError(t, err)
	receiver, err := repo.CreateAccount(ctx, db.CreateAccountParams{
		OwnerName: "Receiver",
		Balance:   0,
	})
	require.NoError(t, err)
	type result struct {
		err error
	}
	results := make(chan result, numTransfers)
	for i := 0; i < numTransfers; i++ {
		i := i
		go func() {
			_, err := svc.ExecuteTransfer(ctx,
				sender.ID,
				receiver.ID,
				amountEach,
				fmt.Sprintf("concurrent-key-%d", i),
			)
			results <- result{err: err}
		}()
	}
	var succeeded, failed int
	for i := 0; i < numTransfers; i++ {
		r := <-results
		if r.err != nil {
			t.Logf("transfer %d failed: %v", i, r.err)
			failed++
		} else {
			succeeded++
		}
	}
	t.Logf("succeeded: %d, failed: %d", succeeded, failed)
	assert.Equal(t, numTransfers, succeeded,
		"all transfers must succeed when there is sufficient balance")
	dbSender, err := repo.GetAccount(ctx, sender.ID)
	require.NoError(t, err)
	dbReceiver, err := repo.GetAccount(ctx, receiver.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), dbSender.Balance,
		"sender must have zero balance after all transfers")
	assert.Equal(t, initialBalance, dbReceiver.Balance,
		"receiver balance must equal exactly what was sent")
	totalBalance := dbSender.Balance + dbReceiver.Balance
	assert.Equal(t, initialBalance, totalBalance,
		"total money in system must be conserved (double-entry invariant)")
	var txnCount int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM transactions WHERE from_account_id = $1",
		sender.ID,
	).Scan(&txnCount)
	require.NoError(t, err)
	assert.Equal(t, numTransfers, txnCount,
		"must have exactly one transaction row per transfer")
}
