package observability_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/anmol420/p2p-payment-ledger/internal/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryLoggingInterceptor_InjectsTraceID(t *testing.T) {
	logger := slog.Default()
	interceptor := observability.UnaryLoggerInterceptor(logger)
	info := &grpc.UnaryServerInfo{FullMethod: "/ledger.v1.LedgerService/Transfer"}
	var capturedCtx context.Context
	handler := func(ctx context.Context, req any) (any, error) {
		capturedCtx = ctx
		return "ok", nil
	}
	_, err := interceptor(context.Background(), nil, info, handler)
	require.NoError(t, err)
	traceID := observability.TraceIDFromContext(capturedCtx)
	assert.NotEmpty(t, traceID, "trace_id must be injected into context")
	assert.Len(t, traceID, 36, "trace_id must be a UUID (36 chars)")
	logger2 := observability.LoggerFromContext(capturedCtx)
	assert.NotNil(t, logger2, "logger must be injected into context")
}

func TestUnaryLoggingInterceptor_PropagatesErrors(t *testing.T) {
	logger := slog.Default()
	interceptor := observability.UnaryLoggerInterceptor(logger)
	info := &grpc.UnaryServerInfo{FullMethod: "/ledger.v1.LedgerService/GetBalance"}
	expectedErr := status.Error(codes.NotFound, "account not found")
	handler := func(ctx context.Context, req any) (any, error) {
		return nil, expectedErr
	}
	_, err := interceptor(context.Background(), nil, info, handler)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestUnaryRecoveryInterceptor_CatchesPanic(t *testing.T) {
	logger := slog.Default()
	interceptor := observability.UnaryRecoveryInterceptor(logger)
	info := &grpc.UnaryServerInfo{FullMethod: "/ledger.v1.LedgerService/Transfer"}
	handler := func(ctx context.Context, req any) (any, error) {
		panic("something went terribly wrong")
	}
	resp, err := interceptor(context.Background(), nil, info, handler)
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err),
		"panic must be converted to Internal gRPC error")
}

func TestLoggerFromContext_FallsBackToDefault(t *testing.T) {
	ctx := context.Background()
	logger := observability.LoggerFromContext(ctx)
	assert.NotNil(t, logger, "must never return nil logger")
}
