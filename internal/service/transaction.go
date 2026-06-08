package service

import (
	"context"
	"fmt"

	"github.com/anmol420/p2p-payment-ledger/internal/db"
	"github.com/anmol420/p2p-payment-ledger/internal/observability"
	"github.com/jackc/pgx/v5/pgtype"
)

type TransactionService struct {
	repo db.Repository
}

func NewTransactionService(repo db.Repository) *TransactionService {
	return &TransactionService{repo: repo}
}

type ListTransactionsResult struct {
	Transactions []db.Transaction
	NextCursor   string
}

func (s *TransactionService) ListTransactions(
	ctx context.Context,
	accountID pgtype.UUID,
	pageSize int32,
	pageToken string,
) (ListTransactionsResult, error) {
	logger := observability.LoggerFromContext(ctx)
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	params := db.ListTransactionsByAccountCursorParams{
		AccountID:  accountID,
		LimitCount: pageSize,
	}
	if pageToken == "" {
		params.UseCursor = false
	} else {
		cursorTime, cursorID, err := decodeCursor(pageToken)
		if err != nil {
			return ListTransactionsResult{}, fmt.Errorf("invalid page_token: %w", err)
		}
		params.UseCursor = true
		params.CursorID = cursorID
		params.CursorTime = cursorTime
	}
	txns, err := s.repo.ListTransactionsByAccountCursor(ctx, params)
	if err != nil {
		return ListTransactionsResult{}, fmt.Errorf("list transactions: %w", err)
	}
	logger.Debug("listed transactions",
		"account_id", accountID,
		"page_size", pageSize,
		"returned", len(txns),
		"has_cursor", params.UseCursor,
	)
	var nextCursor string
	if int32(len(txns)) == pageSize {
		last := txns[len(txns)-1]
		var err error
		nextCursor, err = encodeCursor(last.CreatedAt, last.ID)
		if err != nil {
			logger.Error("failed to encode next cursor", "error", err)
		}
	}
	return ListTransactionsResult{
		Transactions: txns,
		NextCursor:   nextCursor,
	}, err
}

type ListAuditLogResult struct {
	AuditLogs  []db.AuditLog
	NextCursor string
}

func (s *TransactionService) ListAuditLog(
	ctx context.Context,
	accountID pgtype.UUID,
	pageSize int32,
	pageToken string,
) (ListAuditLogResult, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	params := db.ListAuditLogByAccountParams{
		AccountID:  accountID,
		LimitCount: pageSize,
	}
	if pageToken != "" {
		cursorTime, cursorID, err := decodeCursor(pageToken)
		if err != nil {
			return ListAuditLogResult{}, fmt.Errorf("invalid page_token: %w", err)
		}
		params.UseCursor = true
		params.CursorTime = cursorTime
		params.CursorID = cursorID
	}
	entries, err := s.repo.ListAuditLogByAccount(ctx, params)
	if err != nil {
		return ListAuditLogResult{}, fmt.Errorf("list audit log: %w", err)
	}
	var nextCursor string
	if int32(len(entries)) == pageSize {
		last := entries[len(entries)-1]
		var err error
		nextCursor, err = encodeCursor(last.CreatedAt, last.ID)
		if err != nil {
			observability.LoggerFromContext(ctx).Error(
				"failed to encode audit cursor", "error", err)
		}
	}
	return ListAuditLogResult{
		AuditLogs:  entries,
		NextCursor: nextCursor,
	}, nil
}
