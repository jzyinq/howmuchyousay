package config

import (
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL         string
	OpenAIAPIKey        string
	OpenAIModel         string
	LogDir              string
	ServerPort          string
	FirecrawlAPIKey     string
	FirecrawlAPIURL     string
	FirecrawlMaxScrapes int
	CrawlTimeout        int
}

func Load() *Config {
	return &Config{
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://hmys:hmys_dev@localhost:5432/howmuchyousay?sslmode=disable"),
		OpenAIAPIKey:        getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:         getEnv("OPENAI_MODEL", "gpt-5-mini"),
		LogDir:              getEnv("LOG_DIR", "./logs"),
		ServerPort:          getEnv("SERVER_PORT", "8080"),
		FirecrawlAPIKey:     getEnv("FIRECRAWL_API_KEY", ""),
		FirecrawlAPIURL:     getEnv("FIRECRAWL_API_URL", "https://api.firecrawl.dev"),
		FirecrawlMaxScrapes: getEnvInt("FIRECRAWL_MAX_SCRAPES", 50),
		CrawlTimeout:        getEnvInt("CRAWL_TIMEOUT", 300),
	}
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	val, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return n
}
