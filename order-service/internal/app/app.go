package app

import (
	"context"
	"log"
	"net/http"
	"order-service/internal/cache"
	"order-service/internal/config"
	"order-service/internal/handler"
	"order-service/internal/repository"
	"order-service/internal/service"

	"github.com/gorilla/mux"
)

type App struct {
	server *http.Server
}

func New(ctx context.Context) (*App, error) {
	cfg := config.Load()
	pool, err := repository.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	redisCache := cache.New(cfg.RedisAddr)
	repo := repository.New(pool)
	orderSvc := service.NewOrderService(repo)
	productSvc := service.NewProductService(repo, redisCache)
	h := handler.New(orderSvc, productSvc)
	r := setupRoutes(h)
	return &App{server: &http.Server{Addr: cfg.Port, Handler: r}}, nil
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
	log.Println("Order service started on :8083")
	return a.server.ListenAndServe()
}
