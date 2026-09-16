package app

import (
	"context"
	"fmt"
	"net/http"
	"order-service/internal/cache"
	"order-service/internal/config"
	"order-service/internal/handler"

	"order-service/internal/kafka"
	"order-service/internal/metrics"
	"order-service/internal/repository"
	"order-service/internal/service"
	"order-service/internal/txm"
	"order-service/internal/worker"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

type App struct {
	server *http.Server
	logger *zap.Logger
	ctx    context.Context
	pool   *pgxpool.Pool
	cache  *cache.RedisCache
	relay  *worker.OutboxRelay
}

func newRepositories(pool *pgxpool.Pool, c *cache.RedisCache, logger *zap.Logger) (*repository.OrderRepo, repository.ProductStorage) {
	productRepo := repository.NewProductRepo(pool)
	cachedProductRepo := repository.NewCachedProductRepo(productRepo, c, logger)
	return repository.NewOrderRepo(pool), cachedProductRepo
}

func newServices(
	orderRepo *repository.OrderRepo,
	productRepo repository.ProductStorage,
	pool *pgxpool.Pool,
	logger *zap.Logger,
) (*service.OrderService, *service.ProductService) {
	txManager := txm.New(pool)
	producer := kafka.NewProducer([]string{"kafka:9092"})
	outboxRepo := repository.NewOutboxRepo(pool)
	return service.NewOrderService(orderRepo, txManager, logger, producer, outboxRepo),
		service.NewProductService(productRepo, logger)
}

func newHandler(
	orderSvc *service.OrderService,
	productSvc *service.ProductService,
	logger *zap.Logger,
) *handler.Handler {
	return handler.New(orderSvc, productSvc, logger)
}

func setupRoutes(h *handler.Handler, health *handler.HealthHandler, logger *zap.Logger) http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/products", h.GetProducts).Methods("GET")
	r.HandleFunc("/products/{id}", h.GetProductByID).Methods("GET")
	r.HandleFunc("/orders", h.CreateOrder).Methods("POST")
	r.HandleFunc("/orders", h.GetOrders).Methods("GET")
	r.HandleFunc("/orders/{id}", h.GetOrderByID).Methods("GET")
	r.HandleFunc("/orders/{id}/cancel", h.CancelOrder).Methods("POST")
	r.HandleFunc("/products/{id}/cache", h.InvalidateProductCache).Methods("DELETE")
	r.HandleFunc("/healthz", health.Liveness)
	r.HandleFunc("/readyz", health.Readiness)
	r.Handle("/metrics", promhttp.Handler())

	chain := handler.RequestID(
		handler.Logger(logger)(
			handler.Recover(logger)(
				handler.Timeout(10 * time.Second)(r),
			),
		),
	)
	return chain
}

func New(ctx context.Context, logger *zap.Logger) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	pool, err := repository.Connect(ctx, cfg.DatabaseURL, logger, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("postgres unavailable: %w", err)
	}
	logger.Info("postgres connected!")

	redisCache := cache.New(cfg.RedisAddr)
	if err := redisCache.Ping(ctx); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}
	logger.Info("Redis connected!")

	orderRepo, productRepo := newRepositories(pool, redisCache, logger)
	orderSvc, productSvc := newServices(orderRepo, productRepo, pool, logger)
	h := newHandler(orderSvc, productSvc, logger)

	relay := worker.NewOutboxRelay(pool, []string{"kafka:9092"}, logger)
	return &App{
		server: &http.Server{
			Addr:              cfg.Port,
			Handler:           setupRoutes(h, handler.NewHealthHandler(pool, redisCache), logger),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		logger: logger,
		ctx:    ctx,
		pool:   pool,
		cache:  redisCache,
		relay:  relay,
	}, nil
}

func (a *App) Run() error {
	a.logger.Info("Order service started", zap.String("port", a.server.Addr))

	relayCtx, relayCancel := context.WithCancel(a.ctx)
	defer relayCancel()
	go a.relay.Run(relayCtx)

	// метрики пула соединений
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-relayCtx.Done():
				return
			case <-ticker.C:
				stat := a.pool.Stat()
				metrics.DBPoolAcquired.Set(float64(stat.AcquiredConns()))
				metrics.DBPoolIdle.Set(float64(stat.IdleConns()))
			}
		}
	}()

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
