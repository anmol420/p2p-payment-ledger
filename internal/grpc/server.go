package grpc

import (
	"context"
	"errors"
	"log/slog"

	pb "github.com/anmol420/p2p-payment-ledger/gen/ledger/v1"
	"github.com/anmol420/p2p-payment-ledger/internal/db"
	"github.com/anmol420/p2p-payment-ledger/internal/service"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	pb.UnimplementedLedgerServiceServer
	transferSvc *service.TransferService
	repo        db.Repository
	logger      *slog.Logger
}

func NewServer(
	transferSvc *service.TransferService,
	repo db.Repository,
	logger *slog.Logger,
) *Server {
	return &Server{
		transferSvc: transferSvc,
		repo:        repo,
		logger:      logger,
	}
}

func (s *Server) CreateAccount(
	ctx context.Context,
	req *pb.CreateAccountRequest,
) (*pb.CreateAccountResponse, error) {
	if req.OwnerName == "" {
		return nil, status.Error(codes.InvalidArgument, "owner name is required")
	}
	if len(req.OwnerName) > 255 {
		return nil, status.Error(codes.InvalidArgument, "owner name is too long")
	}
	if req.InitialBalance < 0 {
		return nil, status.Error(codes.InvalidArgument, "initial_balance cannot be negative")
	}
	acc, err := s.repo.CreateAccount(ctx, db.CreateAccountParams{
		OwnerName: req.OwnerName,
		Balance:   req.InitialBalance,
	})
	if err != nil {
		s.logger.Error("create account failed", "error", err)
		return nil, status.Error(codes.Internal, "failed to create account")
	}
	return &pb.CreateAccountResponse{
		Account: accountToProto(acc),
	}, nil
}

func (s *Server) GetBalance(
	ctx context.Context,
	req *pb.GetBalanceRequest,
) (*pb.GetBalanceResponse, error) {
	if req.AccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "account id is required")
	}
	accountId, err := parseUUID(req.AccountId, "account_id")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	acc, err := s.repo.GetAccount(ctx, accountId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "account not found")
		}
		s.logger.Error("get account failed", "account_id", accountId, "error", err)
		return nil, status.Error(codes.Internal, "failed to get account")
	}
	return &pb.GetBalanceResponse{
		AccountId: acc.ID.String(),
		Balance:   acc.Balance,
		OwnerName: acc.OwnerName,
		AsOf:      timestamppb.Now(),
	}, nil
}

func (s *Server) Transfer(
	ctx context.Context,
	req *pb.TransferRequest,
) (*pb.TransferResponse, error) {
	if req.FromAccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "from_account_id is required")
	}
	if req.ToAccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "to_account_id is required")
	}
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	if req.Amount < 0 {
		return nil, status.Error(codes.InvalidArgument, "amount cannot be negative")
	}
	if len(req.IdempotencyKey) > 128 {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is too long")
	}
	fromId, err := parseUUID(req.FromAccountId, "from_account_id")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	toId, err := parseUUID(req.ToAccountId, "to_account_id")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	result, err := s.transferSvc.ExecuteTransfer(
		ctx,
		fromId,
		toId,
		req.Amount,
		req.IdempotencyKey,
	)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &pb.TransferResponse{
		Transaction: transactionToProto(result.Transaction),
		FromBalance: result.FromAccount.Balance,
		ToBalance:   result.ToAccount.Balance,
	}, nil
}

func (s *Server) ListTransactions(
	ctx context.Context,
	req *pb.ListTransactionsRequest,
) (*pb.ListTransactionsResponse, error) {
	if req.AccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	accountId, err := parseUUID(req.AccountId, "account_id")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	pageSize := req.PageSize
	if pageSize == 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := req.PageOffset
	if offset < 0 {
		offset = 0
	}
	txns, err := s.repo.ListTransactionsByAccount(ctx, db.ListTransactionsByAccountParams{
		FromAccountID: accountId,
		Limit:         pageSize,
		Offset:        offset,
	})
	if err != nil {
		s.logger.Error("list transactions failed", "account_id", accountId, "error", err)
		return nil, status.Error(codes.Internal, "list transactions failed")
	}
	protoTxns := make([]*pb.Transaction, len(txns))
	for i, txn := range txns {
		protoTxns[i] = transactionToProto(txn)
	}
	return &pb.ListTransactionsResponse{
		Transactions: protoTxns,
		TotalCount:   int32(len(protoTxns)),
	}, nil
}
