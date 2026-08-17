package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"order-service/internal/repository"
	"order-service/internal/service"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

type Handler struct {
	orderSvc   *service.OrderService
	productSvc *service.ProductService
}

func New(orderSvc *service.OrderService, productSvc *service.ProductService) *Handler {
	return &Handler{orderSvc: orderSvc, productSvc: productSvc}
}

func (h *Handler) GetProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.productSvc.GetProducts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(products)
}

func (h *Handler) GetProductByID(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	product, err := h.productSvc.GetProductByID(ctx, id)
	if err != nil {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(product)
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var items []repository.OrderItem
	json.NewDecoder(r.Body).Decode(&items)

	orderID, err := h.orderSvc.CreateOrder(r.Context(), items)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int{"id": orderID})
}

func (h *Handler) GetOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := h.orderSvc.GetOrders(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}

func (h *Handler) GetOrderByID(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	order, err := h.orderSvc.GetOrderByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	cancelledID, err := h.orderSvc.CancelOrder(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"cancelled_id": cancelledID})
}

func (h *Handler) InvalidateProductCache(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	err := h.productSvc.InvalidateCache(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("cache invalidated"))

}
