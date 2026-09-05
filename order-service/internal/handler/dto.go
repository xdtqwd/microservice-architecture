package handler

import (
	"order-service/internal/domain"
	"time"
)

type OrderResponse struct {
	ID        int                 `json:"id"`
	Status    string              `json:"status"`
	CreatedAt time.Time           `json:"created_at"`
	Items     []OrderItemResponse `json:"items"`
}

type OrderItemResponse struct {
	ProductID int    `json:"product_id"`
	Quantity  int    `json:"quantity"`
	Price     string `json:"price"`
}

type ProductResponse struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Price string `json:"price"`
	Stock int    `json:"stock"`
}

func orderToResponse(o *domain.Order) OrderResponse {
	items := make([]OrderItemResponse, len(o.Items))
	for i, item := range o.Items {
		items[i] = OrderItemResponse{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Price:     item.Price.String(),
		}
	}
	return OrderResponse{
		ID:        o.ID,
		Status:    o.Status,
		CreatedAt: o.CreatedAt,
		Items:     items,
	}
}

func productToResponse(p *domain.Product) ProductResponse {
	return ProductResponse{
		ID:    p.ID,
		Name:  p.Name,
		Price: p.Price.String(),
		Stock: p.Stock,
	}
}
