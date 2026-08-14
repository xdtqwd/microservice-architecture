package handler

import (
	"encoding/json"
	"net/http"
	"order-service/internal/repository"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	conn *pgxpool.Pool
}

func New(conn *pgxpool.Pool) *Handler {
	return &Handler{conn: conn}
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var items []repository.OrderItem
	json.NewDecoder(r.Body).Decode(&items)

	orderID, err := repository.CreateOrder(h.conn, items)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int{"id": orderID})
}

func (h *Handler) GerProducts(w http.ResponseWriter, r *http.Request) {
	products, err := repository.GetProducts(h.conn)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-type", "application/json")
	json.NewEncoder(w).Encode(products)
}

func (h *Handler) GetProductByID(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	product, err := repository.GetProductByID(h.conn, id)
	if err != nil {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-type", "application/json")
	json.NewEncoder(w).Encode(product)
}

func (h *Handler) GetOrderByID(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	order, err := repository.GetOrderByID(h.conn, id)
	if err != nil {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-type", "application/json")
	json.NewEncoder(w).Encode(order)
}
func (h *Handler) GetOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := repository.GetOrders(h.conn)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-type", "application/json")
	json.NewEncoder(w).Encode(orders)
}

func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(mux.Vars(r)["id"])
	err := repository.CancelOrder(h.conn, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
