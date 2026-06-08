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
	repo    db.Repository
	metrics *observability.Metrics
}

func NewTransferService(repo db.Repository, metrics *observability.Metrics) *TransferService {
	return &TransferService{
		repo:    repo,
		metrics: metrics,
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
		s.metrics.TransferTotal.WithLabelValues("invalid_input").Inc()
		return TransferResult{}, ErrInvalidAmount
	}
	if fromID == toID {
		s.metrics.TransferTotal.WithLabelValues("invalid_input").Inc()
		return TransferResult{}, ErrSameAccount
	}
	if idempotencyKey == "" {
		s.metrics.TransferTotal.WithLabelValues("invalid_input").Inc()
		return TransferResult{}, fmt.Errorf("idempotency key is required")
	}
	logger := observability.LoggerFromContext(ctx)
	existing, err := s.repo.GetTransactionByIdempotencyKey(ctx, idempotencyKey)
	if err == nil {
		logger.Info("idempotent request — returning existing transaction",
			"idempotency_key", idempotencyKey,
			"existing_transaction_id", existing.ID,
		)
		s.metrics.IdempotentRequestTotal.Inc()
		s.metrics.TransferTotal.WithLabelValues("success").Inc()
		return s.buildResultFromTransaction(ctx, existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		s.metrics.TransferTotal.WithLabelValues("internal_error").Inc()
		return TransferResult{}, fmt.Errorf("check idempotency key: %w", err)
	}
	var result TransferResult
	err = s.repo.ExecTx(ctx, func(q db.Querier) error {
		var txErr error
		result, txErr = executeTransferTx(ctx, q, fromID, toID, amount, idempotencyKey)
		return txErr
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInsufficientFunds):
			s.metrics.TransferTotal.WithLabelValues("insufficient_funds").Inc()
		case errors.Is(err, ErrAccountNotFound):
			s.metrics.TransferTotal.WithLabelValues("account_not_found").Inc()
		default:
			s.metrics.TransferTotal.WithLabelValues("internal_error").Inc()
		}
		return TransferResult{}, err
	}
	s.metrics.TransferTotal.WithLabelValues("success").Inc()
	s.metrics.TransferAmount.Observe(float64(amount))
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
	_, err = q.CreateAuditEntry(ctx, db.CreateAuditEntryParams{
		AccountID:     fromID,
		TransactionID: txn.ID,
		EventType:     db.AuditEventTypeDebit,
		Amount:        amount,
		BalanceBefore: fromAccount.Balance,
		BalanceAfter:  updatedFrom.Balance,
	})
	if err != nil {
		return TransferResult{}, fmt.Errorf("write debit audit entry: %w", err)
	}
	_, err = q.CreateAuditEntry(ctx, db.CreateAuditEntryParams{
		AccountID:     toID,
		TransactionID: txn.ID,
		EventType:     db.AuditEventTypeCredit,
		Amount:        amount,
		BalanceBefore: toAccount.Balance,
		BalanceAfter:  updatedTo.Balance,
	})
	if err != nil {
		return TransferResult{}, fmt.Errorf("write credit audit entry: %w", err)
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
