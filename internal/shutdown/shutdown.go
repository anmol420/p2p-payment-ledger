package shutdown

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Hook struct {
	Name string
	Fn   func(ctx context.Context) error
}

type Manager struct {
	timeout time.Duration
	hooks   []Hook
	logger  *slog.Logger
}

func NewManager(timeout time.Duration, logger *slog.Logger) *Manager {
	return &Manager{
		timeout: timeout,
		logger:  logger,
	}
}

func (m *Manager) Register(name string, fn func(ctx context.Context) error) {
	m.hooks = append(m.hooks, Hook{name, fn})
}

func (m *Manager) Wait() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	m.logger.Info("shutdown signal received",
		"signal", sig.String(),
		"timeout", m.timeout.String(),
	)
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()
	for i := len(m.hooks) - 1; i >= 0; i-- {
		hook := m.hooks[i]
		m.logger.Info("running shutdown hook", "name", hook.Name)
		if err := hook.Fn(ctx); err != nil {
			m.logger.Warn("shutdown hook failed", "name", hook.Name, "err", err)
		} else {
			m.logger.Info("shutdown hook succeeded", "name", hook.Name)
		}
	}
	m.logger.Info("shutdown completed")
}
