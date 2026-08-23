package domain

import "time"

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
	Price     float64
}

type Product struct {
	ID    int
	Name  string
	Price float64
	Stock int
}
