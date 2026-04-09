package config

import (
	"os"
)

type Config struct {
	DatabaseURL  string
	OpenAIAPIKey string
	OpenAIModel  string
	LogDir       string
	ServerPort   string
}

func Load() *Config {
	return &Config{
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://hmys:hmys_dev@localhost:5432/howmuchyousay?sslmode=disable"),
		OpenAIAPIKey: getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:  getEnv("OPENAI_MODEL", "gpt-5-mini"),
		LogDir:       getEnv("LOG_DIR", "./logs"),
		ServerPort:   getEnv("SERVER_PORT", "8080"),
	}
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}
