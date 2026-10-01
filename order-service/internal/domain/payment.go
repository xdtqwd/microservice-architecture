package domain

import "errors"

type PaymentStatus string

const (
	PaymentPending   PaymentStatus = "pending"
	PaymentSucceeded PaymentStatus = "succeeded"
	PaymentFailed    PaymentStatus = "failed"
	PaymentExpired   PaymentStatus = "expired"
	PaymentRefunded  PaymentStatus = "refunded"
)

// pending ──> succeeded ──> refunded
//
//	│
//	├──> failed
//	└──> expired
var allowedPaymentTransitions = map[PaymentStatus][]PaymentStatus{
	PaymentPending:   {PaymentSucceeded, PaymentFailed, PaymentExpired},
	PaymentSucceeded: {PaymentRefunded},
	PaymentFailed:    {},
	PaymentExpired:   {},
	PaymentRefunded:  {},
}

var (
	ErrPaymentAlreadyActive = errors.New("order already has an active payment")
	ErrPaymentTransition    = errors.New("invalid payment state transition")
	ErrPaymentNotFound      = errors.New("payment not found")
)

func CanTransitionPayment(from, to PaymentStatus) bool {
	for _, s := range allowedPaymentTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// IsTerminal — из состояния нет выходов.
func (s PaymentStatus) IsTerminal() bool {
	next, ok := allowedPaymentTransitions[s]
	return ok && len(next) == 0
}
