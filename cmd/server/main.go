package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	pb "github.com/anmol420/p2p-payment-ledger/gen/ledger/v1"
	"github.com/anmol420/p2p-payment-ledger/internal/db"
	"github.com/anmol420/p2p-payment-ledger/internal/env"
	internalgrpc "github.com/anmol420/p2p-payment-ledger/internal/grpc"
	"github.com/anmol420/p2p-payment-ledger/internal/observability"
	"github.com/anmol420/p2p-payment-ledger/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func logLevelFromEnv() slog.Level {
	LOG_LEVEL := env.StringGetEnv("LOG_LEVEL", slog.Default())
	switch LOG_LEVEL {
	case "debug", "DEBUG":
		return slog.LevelDebug
	case "warn", "WARN":
		return slog.LevelWarn
	case "error", "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevelFromEnv(),
	}))
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	DatabaseAddr := env.StringGetEnv("DATABASE_ADDR", slog.Default())
	Port := env.StringGetEnv("PORT", slog.Default())
	pool, err := db.DbConnect(ctx, DatabaseAddr)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("DB connection successful")
	repo := db.NewRepository(pool)
	transferSvc := service.NewTransferService(repo)
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(
			observability.ChainUnaryInterceptor(
				observability.UnaryRecoveryInterceptor(logger),
				observability.UnaryLoggerInterceptor(logger),
			),
		),
	)
	ledgerService := internalgrpc.NewServer(transferSvc, repo, logger)
	pb.RegisterLedgerServiceServer(grpcServer, ledgerService)
	reflection.Register(grpcServer)
	lis, err := net.Listen("tcp", Port)
	if err != nil {
		slog.Error("Failed to listen", "error", err)
		os.Exit(1)
	}
	go func() {
		slog.Info("Server listening on port", "port", Port)
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("gRPC server error", "error", err)
		}
	}()
	<-ctx.Done()
	slog.Info("Shutting down gRPC server")
	grpcServer.GracefulStop()
	slog.Info("Server shutting down")
}
