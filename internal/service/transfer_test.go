package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/anmol420/p2p-payment-ledger/internal/db"
	"github.com/anmol420/p2p-payment-ledger/internal/db/mocks"
	"github.com/anmol420/p2p-payment-ledger/internal/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func makeUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	require.NoError(t, id.Scan(s), "invalid UUID in test: %s", s)
	return id
}

func setupMockRepo(t *testing.T) (*gomock.Controller, *mocks.MockRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockRepository(ctrl)
	return ctrl, repo
}

func TestExecuteTransfer(t *testing.T) {
	ctx := context.Background()
	anmolID := makeUUID(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	mrxID := makeUUID(t, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	txnID := makeUUID(t, "cccccccc-cccc-cccc-cccc-cccccccccccc")
	anmolAccount := db.Account{ID: anmolID, OwnerName: "Anmol", Balance: 100000}
	mrxAccount := db.Account{ID: mrxID, OwnerName: "mrx", Balance: 50000}
	tests := []struct {
		name           string
		fromID         pgtype.UUID
		toID           pgtype.UUID
		amount         int64
		idempotencyKey string
		setupMocks     func(repo *mocks.MockRepository)
		wantErr        error
		wantFromBal    int64
		wantToBal      int64
	}{
		{
			name:           "happy path — valid transfer",
			fromID:         anmolID,
			toID:           mrxID,
			amount:         20000,
			idempotencyKey: "key-001",
			setupMocks: func(repo *mocks.MockRepository) {
				repo.EXPECT().
					GetTransactionByIdempotencyKey(ctx, "key-001").
					Return(db.Transaction{}, pgx.ErrNoRows)
				repo.EXPECT().
					ExecTx(ctx, gomock.Any()).
					DoAndReturn(func(ctx context.Context, fn func(db.Querier) error) error {
						return fn(repo)
					})
				repo.EXPECT().AcquireAdvisoryLock(ctx, gomock.Any()).Return(nil).Times(2)
				repo.EXPECT().
					GetAccountForUpdate(ctx, anmolID).
					Return(anmolAccount, nil)
				repo.EXPECT().
					GetAccountForUpdate(ctx, mrxID).
					Return(mrxAccount, nil)
				repo.EXPECT().
					UpdateAccountBalance(ctx, db.UpdateAccountBalanceParams{
						ID:      anmolID,
						Balance: 80000,
					}).
					Return(db.Account{ID: anmolID, Balance: 80000}, nil)
				repo.EXPECT().
					UpdateAccountBalance(ctx, db.UpdateAccountBalanceParams{
						ID:      mrxID,
						Balance: 70000,
					}).
					Return(db.Account{ID: mrxID, Balance: 70000}, nil)
				repo.EXPECT().
					CreateTransaction(ctx, gomock.Any()).
					Return(db.Transaction{ID: txnID, Amount: 20000}, nil)
			},
			wantErr:     nil,
			wantFromBal: 80000,
			wantToBal:   70000,
		},
		{
			name:           "insufficient funds",
			fromID:         anmolID,
			toID:           mrxID,
			amount:         999999999,
			idempotencyKey: "key-002",
			setupMocks: func(repo *mocks.MockRepository) {
				repo.EXPECT().
					GetTransactionByIdempotencyKey(ctx, "key-002").
					Return(db.Transaction{}, pgx.ErrNoRows)
				repo.EXPECT().
					ExecTx(ctx, gomock.Any()).
					DoAndReturn(func(ctx context.Context, fn func(db.Querier) error) error {
						return fn(repo)
					})
				repo.EXPECT().AcquireAdvisoryLock(ctx, gomock.Any()).Return(nil).Times(2)
				repo.EXPECT().
					GetAccountForUpdate(ctx, anmolID).
					Return(anmolAccount, nil)
				repo.EXPECT().
					GetAccountForUpdate(ctx, mrxID).
					Return(mrxAccount, nil)
			},
			wantErr: service.ErrInsufficientFunds,
		},
		{
			name:           "duplicate idempotency key — returns existing transaction",
			fromID:         anmolID,
			toID:           mrxID,
			amount:         20000,
			idempotencyKey: "key-003",
			setupMocks: func(repo *mocks.MockRepository) {
				existingTxn := db.Transaction{
					ID:             txnID,
					IdempotencyKey: "key-003",
					FromAccountID:  anmolID,
					ToAccountID:    mrxID,
					Amount:         20000,
				}
				repo.EXPECT().
					GetTransactionByIdempotencyKey(ctx, "key-003").
					Return(existingTxn, nil)
				repo.EXPECT().
					GetAccount(ctx, anmolID).
					Return(db.Account{ID: anmolID, Balance: 80000}, nil)
				repo.EXPECT().
					GetAccount(ctx, mrxID).
					Return(db.Account{ID: mrxID, Balance: 70000}, nil)
			},
			wantErr:     nil,
			wantFromBal: 80000,
			wantToBal:   70000,
		},
		{
			name:           "invalid amount — zero",
			fromID:         anmolID,
			toID:           mrxID,
			amount:         0,
			idempotencyKey: "key-004",
			setupMocks: func(repo *mocks.MockRepository) {
			},
			wantErr: service.ErrInvalidAmount,
		},
		{
			name:           "invalid amount — negative",
			fromID:         anmolID,
			toID:           mrxID,
			amount:         -500,
			idempotencyKey: "key-005",
			setupMocks:     func(repo *mocks.MockRepository) {},
			wantErr:        service.ErrInvalidAmount,
		},
		{
			name:           "same account — cannot transfer to self",
			fromID:         anmolID,
			toID:           anmolID,
			amount:         1000,
			idempotencyKey: "key-006",
			setupMocks:     func(repo *mocks.MockRepository) {},
			wantErr:        service.ErrSameAccount,
		},
		{
			name:           "sender account not found",
			fromID:         anmolID,
			toID:           mrxID,
			amount:         1000,
			idempotencyKey: "key-007",
			setupMocks: func(repo *mocks.MockRepository) {
				repo.EXPECT().
					GetTransactionByIdempotencyKey(ctx, "key-007").
					Return(db.Transaction{}, pgx.ErrNoRows)
				repo.EXPECT().
					ExecTx(ctx, gomock.Any()).
					DoAndReturn(func(ctx context.Context, fn func(db.Querier) error) error {
						return fn(repo)
					})
				repo.EXPECT().AcquireAdvisoryLock(ctx, gomock.Any()).Return(nil).Times(2)
				repo.EXPECT().
					GetAccountForUpdate(ctx, anmolID).
					Return(db.Account{}, pgx.ErrNoRows)
			},
			wantErr: service.ErrAccountNotFound,
		},
		{
			name:           "db failure during transfer — returns internal error",
			fromID:         anmolID,
			toID:           mrxID,
			amount:         1000,
			idempotencyKey: "key-008",
			setupMocks: func(repo *mocks.MockRepository) {
				repo.EXPECT().
					GetTransactionByIdempotencyKey(ctx, "key-008").
					Return(db.Transaction{}, pgx.ErrNoRows)
				repo.EXPECT().
					ExecTx(ctx, gomock.Any()).
					DoAndReturn(func(ctx context.Context, fn func(db.Querier) error) error {
						return fn(repo)
					})
				repo.EXPECT().AcquireAdvisoryLock(ctx, gomock.Any()).Return(nil).Times(2)
				repo.EXPECT().
					GetAccountForUpdate(ctx, anmolID).
					Return(anmolAccount, nil)
				repo.EXPECT().
					GetAccountForUpdate(ctx, mrxID).
					Return(mrxAccount, nil)
				repo.EXPECT().
					UpdateAccountBalance(ctx, gomock.Any()).
					Return(db.Account{}, errors.New("connection reset by peer"))
			},
			wantErr: errors.New("connection reset by peer"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl, mockRepo := setupMockRepo(t)
			defer ctrl.Finish()
			tc.setupMocks(mockRepo)
			svc := service.NewTransferService(mockRepo)
			result, err := svc.ExecuteTransfer(ctx, tc.fromID, tc.toID, tc.amount, tc.idempotencyKey)
			if tc.wantErr != nil {
				require.Error(t, err)
				if errors.Is(tc.wantErr, service.ErrInsufficientFunds) ||
					errors.Is(tc.wantErr, service.ErrAccountNotFound) ||
					errors.Is(tc.wantErr, service.ErrInvalidAmount) ||
					errors.Is(tc.wantErr, service.ErrSameAccount) {
					assert.True(t, errors.Is(err, tc.wantErr),
						"expected %v, got %v", tc.wantErr, err)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantFromBal, result.FromAccount.Balance)
			assert.Equal(t, tc.wantToBal, result.ToAccount.Balance)
		})
	}
}
