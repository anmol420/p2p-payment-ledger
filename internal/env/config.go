package env

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DATABASE_URL       string
	DATABASE_MAX_CONNS int32
	DATABASE_MIN_CONNS int32
	DATABASE_TIMEOUT   time.Duration

	GRPC_PORT    string
	METRICS_PORT string

	LOG_LEVEL string

	SHUTDOWN_TIMEOUT time.Duration
}

func Load() (*Config, error) {
	errL := godotenv.Load()
	if errL != nil {
		return nil, errL
	}

	cfg := &Config{}
	var errs []string
	var err error

	cfg.DATABASE_URL = loadEnv("DATABASE_URL")
	if cfg.DATABASE_URL == "" {
		errs = append(errs, "DATABASE_URL must be set")
	}

	cfg.GRPC_PORT = getDefaultEnv("GRPC_PORT", "50051")
	cfg.METRICS_PORT = getDefaultEnv("METRICS_PORT", "8000")
	cfg.LOG_LEVEL = getDefaultEnv("LOG_LEVEL", "INFO")

	cfg.DATABASE_MAX_CONNS, err = getNumberEnv("DATABASE_MAX_CONNS", 25)
	if err != nil {
		errs = append(errs, fmt.Sprintf("DATABASE_MAX_CONNS: %v", err))
	}
	cfg.DATABASE_MIN_CONNS, err = getNumberEnv("DATABASE_MIN_CONNS", 5)
	if err != nil {
		errs = append(errs, fmt.Sprintf("DATABASE_MIN_CONNS: %v", err))
	}
	cfg.DATABASE_TIMEOUT, err = getDurationEnv("DATABASE_TIMEOUT", 5*time.Second)
	if err != nil {
		errs = append(errs, fmt.Sprintf("DATABASE_TIMEOUT: %v", err))
	}
	cfg.SHUTDOWN_TIMEOUT, err = getDurationEnv("SHUTDOWN_TIMEOUT", 30*time.Second)
	if err != nil {
		errs = append(errs, fmt.Sprintf("SHUTDOWN_TIMEOUT: %v", err))
	}

	if cfg.DATABASE_MIN_CONNS > cfg.DATABASE_MAX_CONNS {
		errs = append(errs, "DATABASE_MIN_CONNS > DATABASE_MAX_CONNS")
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("config validation failed:\n  - %s",
			strings.Join(errs, "\n  - "))
	}
	return cfg, nil
}

func (c *Config) SlogLevel() slog.Level {
	switch strings.ToLower(c.LOG_LEVEL) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (c *Config) GRPCAddr() string {
	return ":" + c.GRPC_PORT
}

func (c *Config) MetricsAddr() string {
	return ":" + c.METRICS_PORT
}

func getDefaultEnv(key string, defaultVal string) string {
	val := loadEnv(key)
	if val == "" {
		val = defaultVal
	}
	return val
}

func getNumberEnv(key string, defaultVal int32) (int32, error) {
	val := loadEnv(key)
	if val == "" {
		return defaultVal, nil
	}
	intVal, err := strconv.ParseInt(val, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%q is not a valid integer: %w", val, err)
	}
	return int32(intVal), nil
}

func getDurationEnv(key string, defaultVal time.Duration) (time.Duration, error) {
	val := loadEnv(key)
	if val == "" {
		return defaultVal, nil
	}
	duration, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal, fmt.Errorf("%q is not a valid duration (use e.g. 30s, 1m): %w", val, err)
	}
	return duration, nil
}
