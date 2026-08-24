package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"order-service/internal/apperrors"
	"order-service/internal/domain"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

type OrderService interface {
	CreateOrder(ctx context.Context, items []domain.CreateOrderItem) (int, error)
	GetOrders(ctx context.Context, limit, offset int) ([]domain.Order, error)
	GetOrderByID(ctx context.Context, id int) (*domain.Order, error)
	CancelOrder(ctx context.Context, id int) (int, error)
}

type ProductService interface {
	GetProducts(ctx context.Context) ([]domain.Product, error)
	GetProductByID(ctx context.Context, id int) (*domain.Product, error)
	InvalidateCache(ctx context.Context, id int) error
}

type Handler struct {
	orderSvc   OrderService
	productSvc ProductService
}

func New(orderSvc OrderService, productSvc ProductService) *Handler {
	return &Handler{orderSvc: orderSvc, productSvc: productSvc}
}

type CreateOrderRequest struct {
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

func (h *Handler) GetProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.productSvc.GetProducts(r.Context())
	if err != nil {
		if err.Error() == "order already cancelled" {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(products); err != nil {
		if err.Error() == "order already cancelled" {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) GetProductByID(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	product, err := h.productSvc.GetProductByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			http.Error(w, "product not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(productToResponse(product)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var reqs []CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	items := make([]domain.CreateOrderItem, len(reqs))
	for i, req := range reqs {
		items[i] = domain.CreateOrderItem{
			ProductID: req.ProductID,
			Quantity:  req.Quantity,
		}
	}

	orderID, err := h.orderSvc.CreateOrder(r.Context(), items)
	if err != nil {
		if errors.Is(err, domain.ErrInsufficientStock) {
			http.Error(w, "insufficient stock", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]int{"id": orderID}); err != nil {
		if err.Error() == "order already cancelled" {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) GetOrders(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	orders, err := h.orderSvc.GetOrders(r.Context(), limit, offset)
	if err != nil {
		if err.Error() == "order already cancelled" {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	responses := make([]OrderResponse, len(orders))
	for i, o := range orders {
		responses[i] = orderToResponse(&o)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(orders); err != nil {
		if err.Error() == "order already cancelled" {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) GetOrderByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	order, err := h.orderSvc.GetOrderByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			http.Error(w, "order not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(orderToResponse(order)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	cancelledID, err := h.orderSvc.CancelOrder(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			http.Error(w, "order not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, apperrors.ErrOrderAlreadyCancelled) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]int{"cancelled_id": cancelledID}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
func (h *Handler) InvalidateProductCache(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	err = h.productSvc.InvalidateCache(r.Context(), id)
	if err != nil {
		if err.Error() == "order already cancelled" {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("cache invalidated")); err != nil {
		if err.Error() == "order already cancelled" {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
