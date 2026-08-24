package apperrors

import "errors"

var (
	ErrInsufficientStock     = errors.New("insufficient stock")
	ErrOrderAlreadyCancelled = errors.New("order already cancelled")
)
