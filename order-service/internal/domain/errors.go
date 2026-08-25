package domain

import "errors"

var (
	ErrOrderNotFound           = errors.New("order not found")
	ErrProductNotFound         = errors.New("product not found")
	ErrInsufficientStock       = errors.New("insufficient stock")
	ErrInvalidStatusTransition = errors.New("invalid status transition")
)

var ErrOrderAlreadyCancelled = errors.New("order already cancelled")
