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

// IdempotencyClaim — результат попытки занять ключ идемпотентности.
type IdempotencyClaim struct {
	Owner    bool // ключ наш — выполняем запрос
	Mismatch bool // ключ уже использован с другим телом запроса
	Busy     bool // запрос с этим ключом выполняется прямо сейчас
	Code     int  // сохранённый ответ первой попытки
	Body     []byte
}

// ErrPaymentInProgress — у заказа есть платёж с неизвестным исходом.
// Отменять нельзя: не знаем, списаны ли деньги и нужен ли возврат.
var ErrPaymentInProgress = errors.New("order has a payment in progress, retry later")
