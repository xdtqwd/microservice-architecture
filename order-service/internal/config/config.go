package config

import "os"

type Config struct {
	DatabaseURL string
	Port        string
	RedisAddr   string
}

func Load() *Config {
	return &Config{
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:password@localhost:5436/orders_db"),
		Port:        getEnv("PORT", ":8083"),
		RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
