package env

import (
	"os"

	"github.com/joho/godotenv"
)

func loadEnv(key string) string {
	if err := godotenv.Load(); err != nil {
		return ""
	}
	return os.Getenv(key)
}
