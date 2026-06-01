package grpc

import (
	"errors"

	"github.com/anmol420/p2p-payment-ledger/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func toGRPCError(err error) error {
	switch {
	case errors.Is(err, service.ErrInsufficientFunds):
		return status.Errorf(codes.FailedPrecondition, "Insufficient funds: %v", err)
	case errors.Is(err, service.ErrAccountNotFound):
		return status.Errorf(codes.NotFound, "Account not found: %v", err)
	case errors.Is(err, service.ErrSameAccount):
		return status.Errorf(codes.InvalidArgument, "Cannot transfer funds to same account: %v", err)
	case errors.Is(err, service.ErrInvalidAmount):
		return status.Errorf(codes.InvalidArgument, "Invalid amount: %v", err)
	default:
		return status.Errorf(codes.Internal, "Internal error: %v", err)
	}
}
