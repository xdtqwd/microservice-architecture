package handler

import "time"

type OrderResponse struct {
	ID        int                 `json:"id"`
	Status    string              `json:"status"`
	CreatedAt time.Time           `json:"created_at"`
	Items     []OrderItemResponse `json:"items"`
}

type OrderItemResponse struct {
	ProductID int     `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type ProductResponse struct {
	ID    int     `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Stock int     `json:"stock"`
}
