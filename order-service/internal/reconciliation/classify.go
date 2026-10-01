// Package reconciliation — сверка наших платежей с выгрузкой провайдера.
package reconciliation

import (
	"fmt"

	"order-service/internal/domain"

	"github.com/shopspring/decimal"
)

type Kind string

const (
	Matched           Kind = "matched"
	MissingAtProvider Kind = "missing_at_provider" // у нас деньги получены, у провайдера записи нет
	MissingAtOurs     Kind = "missing_at_ours"     // у провайдера списание, у нас платежа нет
	AmountMismatch    Kind = "amount_mismatch"
	StatusMismatch    Kind = "status_mismatch"
)

type Action string

const (
	NoAction Action = ""
	// Безопасные: двигаем наше состояние вперёд по разрешённому переходу к тому,
	// что провайдер уже сделал. Денег никто не двигает, решений не отменяем.
	FixPaymentSucceeded Action = "payment_succeeded"
	FixPaymentFailed    Action = "payment_failed"
	FixRefundSucceeded  Action = "refund_succeeded"
	// Всё остальное — человеку.
	Manual Action = "manual"
)

type Ours struct {
	PaymentID    int64
	OrderID      int64
	Key          string
	Amount       decimal.Decimal
	Status       domain.PaymentStatus
	ChargeID     string
	RefundID     int64
	RefundStatus string // "" — возврата нет
}

type Theirs struct {
	Key      string
	ChargeID string
	Amount   decimal.Decimal
	Status   string // charged | declined | refunded
}

type Finding struct {
	Kind   Kind
	Action Action
	Note   string
}

// Classify сравнивает нашу запись и запись провайдера по одному ключу.
// Любой из аргументов может быть nil — записи нет.
func Classify(ours *Ours, theirs *Theirs) Finding {
	switch {
	case ours == nil && theirs == nil:
		return Finding{Kind: Matched}

	case ours == nil:
		if theirs.Status == "declined" {
			return Finding{Kind: Matched, Note: "declined at provider, no money moved"}
		}
		return Finding{Kind: MissingAtOurs, Action: Manual,
			Note: "provider holds money we have no payment for: find the order, then refund or attach"}

	case theirs == nil:
		switch ours.Status {
		case domain.PaymentFailed, domain.PaymentExpired, domain.PaymentPending:
			// денег не было, или платёж ещё в работе (зависшими занимается PAY-05)
			return Finding{Kind: Matched}
		}
		return Finding{Kind: MissingAtProvider, Action: Manual,
			Note: fmt.Sprintf("we consider payment %s, provider has no charge: order may be shipped unpaid", ours.Status)}
	}

	moneyMoved := theirs.Status == "charged" || theirs.Status == "refunded"
	if moneyMoved && !ours.Amount.Equal(theirs.Amount) {
		return Finding{Kind: AmountMismatch, Action: Manual,
			Note: fmt.Sprintf("we expect %s, provider charged %s", ours.Amount, theirs.Amount)}
	}

	switch {
	case ours.Status == domain.PaymentSucceeded && theirs.Status == "charged",
		ours.Status == domain.PaymentFailed && theirs.Status == "declined",
		ours.Status == domain.PaymentExpired && theirs.Status == "declined",
		ours.Status == domain.PaymentRefunded && theirs.Status == "refunded" && ours.RefundStatus == "succeeded":
		return Finding{Kind: Matched}

	// безопасно: провайдер уже решил, мы просто узнаём об этом
	case ours.Status == domain.PaymentPending && theirs.Status == "charged":
		return Finding{Kind: StatusMismatch, Action: FixPaymentSucceeded, Note: "pending here, charged at provider"}
	case ours.Status == domain.PaymentPending && theirs.Status == "declined":
		return Finding{Kind: StatusMismatch, Action: FixPaymentFailed, Note: "pending here, declined at provider"}
	case theirs.Status == "refunded" && ours.RefundID != 0 &&
		(ours.RefundStatus == "pending" || ours.RefundStatus == "manual_review") &&
		(ours.Status == domain.PaymentSucceeded || ours.Status == domain.PaymentRefunded):
		return Finding{Kind: StatusMismatch, Action: FixRefundSucceeded, Note: "refund done at provider, still pending here"}
	}

	return Finding{Kind: StatusMismatch, Action: Manual,
		Note: fmt.Sprintf("ours %s (refund %q), provider %s", ours.Status, ours.RefundStatus, theirs.Status)}
}
