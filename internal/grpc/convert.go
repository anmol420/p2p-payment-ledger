package grpc

import (
	"fmt"

	pb "github.com/anmol420/p2p-payment-ledger/gen/ledger/v1"
	"github.com/anmol420/p2p-payment-ledger/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func parseUUID(s string, fieldName string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid %s %q: must be a valid UUID", fieldName, s)
	}
	return id, nil
}

func accountToProto(a db.Account) *pb.Account {
	return &pb.Account{
		Id:        a.ID.String(),
		OwnerName: a.OwnerName,
		Balance:   a.Balance,
		CreatedAt: timestamppb.New(a.CreatedAt.Time),
		UpdatedAt: timestamppb.New(a.UpdatedAt.Time),
	}
}

func transactionToProto(t db.Transaction) *pb.Transaction {
	return &pb.Transaction{
		Id:             t.ID.String(),
		IdempotencyKey: t.IdempotencyKey,
		FromAccountId:  t.FromAccountID.String(),
		ToAccountId:    t.ToAccountID.String(),
		Amount:         t.Amount,
		Status:         string(t.Status),
		CreatedAt:      timestamppb.New(t.CreatedAt.Time),
	}
}

func auditEntryToProto(e db.AuditLog) *pb.AuditEntry {
	return &pb.AuditEntry{
		Id:            e.ID.String(),
		AccountId:     e.AccountID.String(),
		TransactionId: e.TransactionID.String(),
		EventType:     string(e.EventType),
		Amount:        e.Amount,
		BalanceBefore: e.BalanceBefore,
		BalanceAfter:  e.BalanceAfter,
		CreatedAt:     timestamppb.New(e.CreatedAt.Time),
	}
}
