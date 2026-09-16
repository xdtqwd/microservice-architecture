package config

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL string
	Port        string
	RedisAddr   string
	DBMaxConns  int32
	DBMinConns  int32
}

func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}

	return &Config{
		DatabaseURL: dbURL,
		Port:        getEnv("PORT", ":8083"),
		RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),
		DBMaxConns:  25,
		DBMinConns:  5,
	}, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
