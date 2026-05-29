package env

import (
	"log/slog"
	"os"
)

func StringGetEnv(key string, logger *slog.Logger) string {
	value := getEnv(key)
	if value == "" {
		logger.Error("Failed to fetch value for key", "key", key)
		os.Exit(1)
	}
	return value
}
