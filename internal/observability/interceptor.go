package observability

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func UnaryLoggerInterceptor(base *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler) (any, error) {
		traceID := uuid.New().String()
		logger := base.With(
			"trace_id", traceID,
			"method", info.FullMethod,
		)
		ctx = WithTraceID(ctx, traceID)
		ctx = WithLogger(ctx, logger)
		start := time.Now()
		logger.Info("request started")
		resp, err := handler(ctx, req)
		duration := time.Since(start)
		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}
		logAttributes := []any{
			"duration", duration.Milliseconds(),
			"status", code.String(),
		}
		if err != nil {
			logAttributes = append(logAttributes, "error", err.Error())
			logger.Error("request failed", logAttributes...)
		} else {
			logger.Info("request completed", logAttributes...)
		}
		return resp, err
	}
}

func UnaryRecoveryInterceptor(base *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				traceID := uuid.New().String()
				base.Error("handler panic recovered",
					"trace_id", traceID,
					"method", info.FullMethod,
					"panic", r,
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

func ChainUnaryInterceptor(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler) (any, error) {
		chained := handler
		for i := len(interceptors) - 1; i >= 0; i-- {
			_, interceptor := i, interceptors[i]
			next := chained
			chained = func(ctx context.Context, req any) (any, error) {
				return interceptor(ctx, req, info, next)
			}
		}
		return chained(ctx, req)
	}
}
