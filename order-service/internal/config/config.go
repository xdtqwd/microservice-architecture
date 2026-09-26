package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL      string
	Port             string
	RedisAddr        string
	DBMaxConns       int32
	DBMinConns       int32
	DBAcquireTimeout time.Duration
}

func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}

	return &Config{
		DatabaseURL:      dbURL,
		Port:             getEnv("PORT", ":8083"),
		RedisAddr:        getEnv("REDIS_ADDR", "localhost:6379"),
		DBMaxConns:       int32(getInt("DB_MAX_CONNS", 10)),
		DBMinConns:       int32(getInt("DB_MIN_CONNS", 2)),
		DBAcquireTimeout: getDuration("DB_ACQUIRE_TIMEOUT", time.Second),
	}, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return fallback
}
