package app

import (
	"context"
	"fmt"
	"net/http"
	"order-service/internal/cache"
	"order-service/internal/config"
	"order-service/internal/handler"
	"order-service/internal/repository"
	"order-service/internal/service"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type App struct {
	server *http.Server
	logger *zap.Logger
	ctx    context.Context
	pool   *pgxpool.Pool
	cache  *cache.RedisCache
}

func New(ctx context.Context, logger *zap.Logger) (*App, error) {
	cfg := config.Load()
	pool, err := repository.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	redisCache := cache.New(cfg.RedisAddr)
	if err := redisCache.Ping(ctx); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}
	logger.Info("Redis connected!")
	orderRepo := repository.NewOrderRepo(pool)
	productRepo := repository.NewProductRepo(pool)
	orderSvc := service.NewOrderService(orderRepo)
	productSvc := service.NewProductService(productRepo, redisCache, logger)
	h := handler.New(orderSvc, productSvc, logger)
	r := setupRoutes(h)
	return &App{
		server: &http.Server{Addr: cfg.Port, Handler: r},
		logger: logger,
		ctx:    ctx,
		pool:   pool,
		cache:  redisCache,
	}, nil
}

func setupRoutes(h *handler.Handler) http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/products", h.GetProducts).Methods("GET")
	r.HandleFunc("/products/{id}", h.GetProductByID).Methods("GET")
	r.HandleFunc("/orders", h.CreateOrder).Methods("POST")
	r.HandleFunc("/orders", h.GetOrders).Methods("GET")
	r.HandleFunc("/orders/{id}", h.GetOrderByID).Methods("GET")
	r.HandleFunc("/orders/{id}/cancel", h.CancelOrder).Methods("POST")
	r.HandleFunc("/products/{id}/cache", h.InvalidateProductCache).Methods("DELETE")
	return r
}

func (a *App) Run() error {
	a.logger.Info("Order service started", zap.String("port", a.server.Addr))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Fatal("server error", zap.Error(err))
		}
	}()

	<-quit
	a.logger.Info("Shutting down...")

	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()

	if err := a.server.Shutdown(ctx); err != nil {
		a.logger.Error("server shutdown error", zap.Error(err))
		return err
	}
	a.logger.Info("HTTP server stopped")

	if err := a.cache.Close(); err != nil {
		a.logger.Error("redis close error", zap.Error(err))
	}
	a.logger.Info("Redis closed")

	a.pool.Close()
	a.logger.Info("DB pool closed")

	return nil
}
