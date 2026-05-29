package env

import (
	"os"

	"github.com/joho/godotenv"
)

func getEnv(key string) string {
	err := godotenv.Load()
	if err != nil {
		return ""
	}
	return os.Getenv(key)
}
