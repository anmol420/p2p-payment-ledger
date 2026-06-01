package grpc

import (
	"context"
	"log/slog"

	pb "github.com/anmol420/p2p-payment-ledger/gen/ledger/v1"
	"github.com/anmol420/p2p-payment-ledger/internal/db"
	"github.com/anmol420/p2p-payment-ledger/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *Server) GetBalance(
	ctx context.Context,
	req *pb.GetBalanceRequest,
) (*pb.GetBalanceResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *Server) Transfer(
	ctx context.Context,
	req *pb.TransferRequest,
) (*pb.TransferResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *Server) ListTransactions(
	ctx context.Context,
	req *pb.ListTransactionsRequest,
) (*pb.ListTransactionsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
