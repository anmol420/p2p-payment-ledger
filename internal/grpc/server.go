package grpc

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	pb "github.com/anmol420/p2p-payment-ledger/gen/ledger/v1"
	"github.com/anmol420/p2p-payment-ledger/internal/db"
	"github.com/anmol420/p2p-payment-ledger/internal/observability"
	"github.com/anmol420/p2p-payment-ledger/internal/service"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	pb.UnimplementedLedgerServiceServer
	transferSvc *service.TransferService
	txnSvc      *service.TransactionService
	repo        db.Repository
	logger      *slog.Logger
	metrics     *observability.Metrics
}

func NewServer(
	transferSvc *service.TransferService,
	txnSvc *service.TransactionService,
	repo db.Repository,
	logger *slog.Logger,
	metrics *observability.Metrics,
) *Server {
	return &Server{
		transferSvc: transferSvc,
		txnSvc:      txnSvc,
		repo:        repo,
		logger:      logger,
		metrics:     metrics,
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
	logger := observability.LoggerFromContext(ctx)
	logger.Info("create account",
		"account", req.OwnerName,
		"initial_balance", req.InitialBalance,
	)
	acc, err := s.repo.CreateAccount(ctx, db.CreateAccountParams{
		OwnerName: req.OwnerName,
		Balance:   req.InitialBalance,
	})
	if err != nil {
		s.logger.Error("create account failed", "error", err)
		return nil, status.Error(codes.Internal, "failed to create account")
	}
	s.metrics.ActiveAccountsTotal.Inc()
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
	logger := observability.LoggerFromContext(ctx)
	logger.Info("get account balance",
		"account_id", req.AccountId,
	)
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
	logger := observability.LoggerFromContext(ctx)
	logger.Info("executing transfer",
		"from_account_id", req.FromAccountId,
		"to_account_id", req.ToAccountId,
		"amount", req.Amount,
		"idempotency_key", req.IdempotencyKey,
	)
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
	logger := observability.LoggerFromContext(ctx)
	if req.AccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	accountID, err := parseUUID(req.AccountId, "account_id")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	result, err := s.txnSvc.ListTransactions(ctx, accountID, req.PageSize, req.PageToken)
	if err != nil {
		if strings.Contains(err.Error(), "invalid page_token") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		logger.Error("list transactions failed", "error", err)
		return nil, status.Error(codes.Internal, "failed to list transactions")
	}
	protoTxns := make([]*pb.Transaction, len(result.Transactions))
	for i, t := range result.Transactions {
		protoTxns[i] = transactionToProto(t)
	}
	return &pb.ListTransactionsResponse{
		Transactions:  protoTxns,
		NextPageToken: result.NextCursor,
	}, nil
}
func (s *Server) GetAuditLog(
	ctx context.Context,
	req *pb.GetAuditLogRequest,
) (*pb.GetAuditLogResponse, error) {
	logger := observability.LoggerFromContext(ctx)
	if req.AccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	accountID, err := parseUUID(req.AccountId, "account_id")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	result, err := s.txnSvc.ListAuditLog(ctx, accountID, req.PageSize, req.PageToken)
	if err != nil {
		if strings.Contains(err.Error(), "invalid page_token") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		logger.Error("get audit log failed", "error", err)
		return nil, status.Error(codes.Internal, "failed to get audit log")
	}
	protoEntries := make([]*pb.AuditEntry, len(result.AuditLogs))
	for i, e := range result.AuditLogs {
		protoEntries[i] = auditEntryToProto(e)
	}

	return &pb.GetAuditLogResponse{
		Entries:       protoEntries,
		NextPageToken: result.NextCursor,
	}, nil
}
