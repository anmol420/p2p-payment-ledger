package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/anmol420/p2p-payment-ledger/internal/db"
	"github.com/anmol420/p2p-payment-ledger/internal/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInsufficientFunds    = errors.New("insufficient funds")
	ErrAccountNotFound      = errors.New("account not found")
	ErrSameAccount          = errors.New("cannot transfer to same account")
	ErrInvalidAmount        = errors.New("amount must be greater than zero")
	ErrDuplicateIdempotency = errors.New("duplicate idempotency key")
)

type TransferService struct {
	repo db.Repository
}

func NewTransferService(repo db.Repository) *TransferService {
	return &TransferService{
		repo: repo,
	}
}

type TransferResult struct {
	Transaction db.Transaction
	FromAccount db.Account
	ToAccount   db.Account
}

func (s *TransferService) ExecuteTransfer(
	ctx context.Context,
	fromID pgtype.UUID,
	toID pgtype.UUID,
	amount int64,
	idempotencyKey string,
) (TransferResult, error) {
	if amount <= 0 {
		return TransferResult{}, ErrInvalidAmount
	}
	if fromID == toID {
		return TransferResult{}, ErrSameAccount
	}
	if idempotencyKey == "" {
		return TransferResult{}, fmt.Errorf("idempotency key is required")
	}
	logger := observability.LoggerFromContext(ctx)
	existing, err := s.repo.GetTransactionByIdempotencyKey(ctx, idempotencyKey)
	if err == nil {
		logger.Info("idempotent request — returning existing transaction",
			"idempotency_key", idempotencyKey,
			"existing_transaction_id", existing.ID,
		)
		return s.buildResultFromTransaction(ctx, existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return TransferResult{}, fmt.Errorf("check idempotency key: %w", err)
	}
	var result TransferResult
	err = s.repo.ExecTx(ctx, func(q db.Querier) error {
		var txErr error
		result, txErr = executeTransferTx(ctx, q, fromID, toID, amount, idempotencyKey)
		return txErr
	})
	if err != nil {
		return TransferResult{}, err
	}
	return result, nil
}

func executeTransferTx(
	ctx context.Context,
	q db.Querier,
	fromID pgtype.UUID,
	toID pgtype.UUID,
	amount int64,
	idempotencyKey string,
) (TransferResult, error) {
	lockIDA := uuidToInt64(fromID)
	lockIDB := uuidToInt64(toID)
	if lockIDA > lockIDB {
		lockIDA, lockIDB = lockIDB, lockIDA
	}
	if err := q.AcquireAdvisoryLock(ctx, lockIDA); err != nil {
		return TransferResult{}, fmt.Errorf("acquire lock A: %w", err)
	}
	if err := q.AcquireAdvisoryLock(ctx, lockIDB); err != nil {
		return TransferResult{}, fmt.Errorf("acquire lock B: %w", err)
	}
	fromAccount, err := q.GetAccountForUpdate(ctx, fromID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TransferResult{}, fmt.Errorf("%w: sender %s", ErrAccountNotFound, fromID)
		}
		return TransferResult{}, fmt.Errorf("fetch sender: %w", err)
	}
	toAccount, err := q.GetAccountForUpdate(ctx, toID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TransferResult{}, fmt.Errorf("%w: receiver %s", ErrAccountNotFound, toID)
		}
		return TransferResult{}, fmt.Errorf("fetch receiver: %w", err)
	}
	logger := observability.LoggerFromContext(ctx)
	logger.Debug("advisory locks acquired — proceeding with transfer",
		"from_account", fromAccount,
		"to_account", toAccount,
		"amount", amount,
	)
	logger.Debug("balance check passed",
		"available", fromAccount.Balance,
		"required", amount,
	)
	if fromAccount.Balance < amount {
		return TransferResult{}, ErrInsufficientFunds
	}
	updatedFrom, err := q.UpdateAccountBalance(ctx, db.UpdateAccountBalanceParams{
		ID:      fromID,
		Balance: fromAccount.Balance - amount,
	})
	if err != nil {
		return TransferResult{}, fmt.Errorf("debit sender: %w", err)
	}
	updatedTo, err := q.UpdateAccountBalance(ctx, db.UpdateAccountBalanceParams{
		ID:      toID,
		Balance: toAccount.Balance + amount,
	})
	if err != nil {
		return TransferResult{}, fmt.Errorf("credit receiver: %w", err)
	}
	txn, err := q.CreateTransaction(ctx, db.CreateTransactionParams{
		IdempotencyKey: idempotencyKey,
		FromAccountID:  fromID,
		ToAccountID:    toID,
		Amount:         amount,
		Status:         db.TransactionStatusCompleted,
	})
	if err != nil {
		return TransferResult{}, fmt.Errorf("record transaction: %w", err)
	}
	return TransferResult{
		Transaction: txn,
		FromAccount: updatedFrom,
		ToAccount:   updatedTo,
	}, nil
}

func (s *TransferService) buildResultFromTransaction(
	ctx context.Context,
	txn db.Transaction,
) (TransferResult, error) {
	fromAccount, err := s.repo.GetAccount(ctx, txn.FromAccountID)
	if err != nil {
		return TransferResult{}, fmt.Errorf("fetch from account: %w", err)
	}
	toAccount, err := s.repo.GetAccount(ctx, txn.ToAccountID)
	if err != nil {
		return TransferResult{}, fmt.Errorf("fetch to account: %w", err)
	}
	return TransferResult{
		Transaction: txn,
		FromAccount: fromAccount,
		ToAccount:   toAccount,
	}, nil
}
