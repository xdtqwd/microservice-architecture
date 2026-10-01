package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL           string
	Port                  string
	RedisAddr             string
	DBMaxConns            int32
	DBMinConns            int32
	DBAcquireTimeout      time.Duration
	RedisBreakerThreshold int
	RedisBreakerOpenFor   time.Duration
	ShutdownDrainDelay    time.Duration
	PayStuckAfter         time.Duration
	PayExpireAfter        time.Duration
	ReconcileEvery        time.Duration
	ReconcileWindow       time.Duration
}

func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}

	return &Config{
		DatabaseURL:           dbURL,
		Port:                  getEnv("PORT", ":8083"),
		RedisAddr:             getEnv("REDIS_ADDR", "localhost:6379"),
		DBMaxConns:            int32(getInt("DB_MAX_CONNS", 10)),
		DBMinConns:            int32(getInt("DB_MIN_CONNS", 2)),
		DBAcquireTimeout:      getDuration("DB_ACQUIRE_TIMEOUT", time.Second),
		RedisBreakerThreshold: getInt("REDIS_BREAKER_THRESHOLD", 5),
		RedisBreakerOpenFor:   getDuration("REDIS_BREAKER_OPEN_FOR", 5*time.Second),
		ShutdownDrainDelay:    getDuration("SHUTDOWN_DRAIN_DELAY", 5*time.Second),
		PayStuckAfter:         getDuration("PAY_STUCK_AFTER", 2*time.Minute),
		PayExpireAfter:        getDuration("PAY_EXPIRE_AFTER", 15*time.Minute),
		ReconcileEvery:        getDuration("RECONCILE_EVERY", time.Hour),
		ReconcileWindow:       getDuration("RECONCILE_WINDOW", 48*time.Hour),
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
