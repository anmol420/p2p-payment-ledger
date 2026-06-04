package main

import (
	"context"
	"log/slog"
	"net"
	"os"

	pb "github.com/anmol420/p2p-payment-ledger/gen/ledger/v1"
	"github.com/anmol420/p2p-payment-ledger/internal/db"
	"github.com/anmol420/p2p-payment-ledger/internal/env"
	internalgrpc "github.com/anmol420/p2p-payment-ledger/internal/grpc"
	"github.com/anmol420/p2p-payment-ledger/internal/observability"
	"github.com/anmol420/p2p-payment-ledger/internal/service"
	"github.com/anmol420/p2p-payment-ledger/internal/shutdown"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	cfg, err := env.Load()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error(
			"config validation failed",
			"error", err,
		)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.SlogLevel(),
	}))
	slog.SetDefault(logger)
	logger.Info("starting server",
		"grpc_port", cfg.GRPC_PORT,
		"log_level", cfg.LOG_LEVEL,
		"db_max_conn", cfg.DATABASE_MAX_CONNS,
	)
	shutdownMgr := shutdown.NewManager(cfg.SHUTDOWN_TIMEOUT, logger)
	pool, err := db.DbConnect(context.Background(), db.PoolConfig{
		DSN:          cfg.DATABASE_URL,
		MaxOpenConns: cfg.DATABASE_MAX_CONNS,
		MinOpenConns: cfg.DATABASE_MIN_CONNS,
		ConnTimeout:  cfg.DATABASE_TIMEOUT,
	})
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	logger.Info("database connected",
		"max_conns", cfg.DATABASE_MAX_CONNS,
		"min_conns", cfg.DATABASE_MIN_CONNS,
	)
	shutdownMgr.Register("database", func(ctx context.Context) error {
		pool.Close()
		return nil
	})
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
	ledgerServer := internalgrpc.NewServer(transferSvc, repo, logger)
	pb.RegisterLedgerServiceServer(grpcServer, ledgerServer)
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("ledger.v1.LedgerService", grpc_health_v1.HealthCheckResponse_SERVING)
	reflection.Register(grpcServer)
	shutdownMgr.Register("grpcServer", func(ctx context.Context) error {
		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
			return nil
		case <-ctx.Done():
			logger.Warn("graceful stop timed out — forcing stop")
			grpcServer.Stop()
			return ctx.Err()
		}
	})
	lis, err := net.Listen("tcp", cfg.GRPCAddr())
	if err != nil {
		logger.Error("failed to listen", "error", err, "addr", cfg.GRPCAddr(), "port", cfg.GRPC_PORT)
		os.Exit(1)
	}
	go func() {
		logger.Info("gRPC server listening", "addr", cfg.GRPCAddr())
		if err := grpcServer.Serve(lis); err != nil {
			logger.Info("gRPC server stopped", "reason", err.Error())
		}
	}()
	shutdownMgr.Wait()
}
