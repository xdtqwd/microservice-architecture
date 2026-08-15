package app

import (
	"context"
	"log"
	"net/http"
	"order-service/internal/handler"
	"order-service/internal/repository"

	"github.com/gorilla/mux"
)

type App struct {
	server *http.Server
}

func New(ctx context.Context) (*App, error) {
	pool, err := repository.Connect(ctx)
	if err != nil {
		return nil, err
	}
	repo := repository.New(pool)
	h := handler.New(repo)
	r := setupRoutes(h)
	return &App{server: &http.Server{Addr: ":8083", Handler: r}}, nil
}
func setupRoutes(h *handler.Handler) http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/products", h.GetProducts).Methods("GET")
	r.HandleFunc("/products/{id}", h.GetProductByID).Methods("GET")
	r.HandleFunc("/orders", h.CreateOrder).Methods("POST")
	r.HandleFunc("/orders", h.GetOrders).Methods("GET")
	r.HandleFunc("/orders/{id}", h.GetOrderByID).Methods("GET")
	r.HandleFunc("/orders/{id}/cancel", h.CancelOrder).Methods("POST")
	return r
}

func (a *App) Run() error {
	log.Println("Order service started on :8083")
	return a.server.ListenAndServe()
}
