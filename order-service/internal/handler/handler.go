package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"order-service/internal/domain"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type OrderService interface {
	CreateOrder(ctx context.Context, items []domain.CreateOrderItem) (int, error)
	GetOrders(ctx context.Context, limit int, cursor *domain.OrderCursor) ([]domain.Order, *domain.OrderCursor, error)
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
	logger     *zap.Logger
}

type CreateOrderRequest struct {
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

func New(orderSvc OrderService, productSvc ProductService, logger *zap.Logger) *Handler {
	return &Handler{orderSvc: orderSvc, productSvc: productSvc, logger: logger}
}

func (h *Handler) GetProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.productSvc.GetProducts(r.Context())
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	responses := make([]ProductResponse, len(products))
	for i, p := range products {
		responses[i] = productToResponse(&p)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		writeError(w, h.logger, err)
	}
}

func (h *Handler) GetProductByID(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		writeError(w, h.logger, domain.ErrProductNotFound)
		return
	}
	product, err := h.productSvc.GetProductByID(ctx, id)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(productToResponse(product)); err != nil {
		writeError(w, h.logger, err)
	}
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var reqs []CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		writeError(w, h.logger, errors.New("invalid request body"))
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
		writeError(w, h.logger, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]int{"id": orderID}); err != nil {
		writeError(w, h.logger, err)
	}
}

func (h *Handler) GetOrders(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var cursor *domain.OrderCursor
	if afterID, _ := strconv.Atoi(r.URL.Query().Get("after_id")); afterID > 0 {
		cursor = &domain.OrderCursor{AfterID: afterID}
	}

	orders, nextCursor, err := h.orderSvc.GetOrders(r.Context(), limit, cursor)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	type response struct {
		Orders []OrderResponse `json:"orders"`
		NextAfterID *int `json:"next_after_id,omitempty"`
	}
	resp := response{Orders: make([]OrderResponse, len(orders))}
	for i, o := range orders {
		resp.Orders[i] = orderToResponse(&o)
	}
	if nextCursor != nil {
		resp.NextAfterID = &nextCursor.AfterID
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		writeError(w, h.logger, err)
	}
}

func (h *Handler) GetOrderByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		writeError(w, h.logger, domain.ErrOrderNotFound)
		return
	}
	order, err := h.orderSvc.GetOrderByID(r.Context(), id)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(orderToResponse(order)); err != nil {
		writeError(w, h.logger, err)
	}
}

func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		writeError(w, h.logger, domain.ErrOrderNotFound)
		return
	}
	cancelledID, err := h.orderSvc.CancelOrder(r.Context(), id)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]int{"cancelled_id": cancelledID}); err != nil {
		writeError(w, h.logger, err)
	}
}

func (h *Handler) InvalidateProductCache(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		writeError(w, h.logger, domain.ErrProductNotFound)
		return
	}
	if err = h.productSvc.InvalidateCache(r.Context(), id); err != nil {
		writeError(w, h.logger, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
		writeError(w, h.logger, err)
	}
}

// убедимся что service импортируется
