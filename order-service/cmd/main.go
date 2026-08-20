package main

import (
	"context"
	"log"
	"order-service/internal/app"
	"order-service/internal/logger"

	"go.uber.org/zap"
)

func main() {
	ctx := context.Background()

	logger, err := logger.New()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	app, err := app.New(ctx, logger)
	if err != nil {
		logger.Fatal("failed to start", zap.Error(err))
	}
	if err := app.Run(); err != nil {
		logger.Fatal("server stopped", zap.Error(err))
	}
	logger.Info("server stopped gracefully")
}
