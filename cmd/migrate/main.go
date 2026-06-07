package main

import (
	"errors"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	DATABASE_URL := os.Getenv("DATABASE_URL")
	if DATABASE_URL == "" {
		logger.Error("DATABASE_URL environment variable not set")
		os.Exit(1)
	}
	m, err := migrate.New("file:///migrations", DATABASE_URL)
	if err != nil {
		logger.Error("failed to create migrator", "error", err)
		os.Exit(1)
	}
	defer func(m *migrate.Migrate) {
		err, _ := m.Close()
		if err != nil {
			logger.Error("failed to close migrator", "error", err)
		}
	}(m)
	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("no migration found")
			os.Exit(0)
		}
		logger.Error("failed to run migration", "error", err)
		os.Exit(1)
	}
	version, dirty, err := m.Version()
	if err != nil {
		logger.Error("failed to get migration version", "error", err)
		os.Exit(1)
	}
	logger.Info("migration applied successfully", "version", version, "dirty", dirty)
}
