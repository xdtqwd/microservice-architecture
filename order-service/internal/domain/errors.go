package domain

import "errors"

var (
	ErrOrderNotFound           = errors.New("order not found")
	ErrProductNotFound         = errors.New("product not found")
	ErrInsufficientStock       = errors.New("insufficient stock")
	ErrInvalidStatusTransition = errors.New("invalid status transition")
)

var ErrOrderAlreadyCancelled = errors.New("order already cancelled")

var (
	ErrInvalidCursor      = errors.New("invalid cursor")
	ErrOffsetNotSupported = errors.New("offset is not supported, use after_id for pagination")
)

var (
	// ErrInvalidRequest — некорректные данные от клиента: кривой JSON,
	// пустой заказ, неверные параметры запроса.
	ErrInvalidRequest = errors.New("invalid request")
	// ErrUnsupportedMediaType — тело не в application/json.
	ErrUnsupportedMediaType = errors.New("content type must be application/json")
)
