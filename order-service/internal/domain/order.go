package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

type Order struct {
	ID        int
	Status    string
	CreatedAt time.Time
	Items     []OrderItem
}

type OrderItem struct {
	ID        int
	OrderID   int
	ProductID int
	Quantity  int
	Price     decimal.Decimal
}

type Product struct {
	ID    int
	Name  string
	Price decimal.Decimal
	Stock int
}

type CreateOrderItem struct {
	ProductID int
	Quantity  int
}

type OrderCursor struct {
	AfterID int
}
