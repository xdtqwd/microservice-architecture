package reconciliation

import (
	"testing"

	"order-service/internal/domain"

	"github.com/shopspring/decimal"
)

func TestClassify(t *testing.T) {
	d := decimal.NewFromInt
	ours := func(s domain.PaymentStatus, amount int64, refund string) *Ours {
		o := &Ours{PaymentID: 1, Key: "k", Amount: d(amount), Status: s, RefundStatus: refund}
		if refund != "" {
			o.RefundID = 7
		}
		return o
	}
	theirs := func(s string, amount int64) *Theirs {
		return &Theirs{Key: "k", ChargeID: "ch_1", Amount: d(amount), Status: s}
	}

	cases := []struct {
		name   string
		o      *Ours
		th     *Theirs
		kind   Kind
		action Action
	}{
		{"совпало: оплачен и списан", ours(domain.PaymentSucceeded, 100, ""), theirs("charged", 100), Matched, NoAction},
		{"совпало: отказ с обеих сторон", ours(domain.PaymentFailed, 100, ""), theirs("declined", 100), Matched, NoAction},
		{"совпало: возврат с обеих сторон", ours(domain.PaymentRefunded, 100, "succeeded"), theirs("refunded", 100), Matched, NoAction},
		{"совпало: у нас failed, у него ничего", ours(domain.PaymentFailed, 100, ""), nil, Matched, NoAction},
		{"совпало: у него отказ, у нас ничего", nil, theirs("declined", 100), Matched, NoAction},

		{"у нас paid, у него нет", ours(domain.PaymentSucceeded, 100, ""), nil, MissingAtProvider, Manual},
		{"у нас refunded, у него нет", ours(domain.PaymentRefunded, 100, "succeeded"), nil, MissingAtProvider, Manual},
		{"у него списание, у нас нет", nil, theirs("charged", 100), MissingAtOurs, Manual},
		{"суммы разошлись", ours(domain.PaymentSucceeded, 100, ""), theirs("charged", 99), AmountMismatch, Manual},

		{"безопасно: pending, у него списано", ours(domain.PaymentPending, 100, ""), theirs("charged", 100), StatusMismatch, FixPaymentSucceeded},
		{"безопасно: pending, у него отказ", ours(domain.PaymentPending, 100, ""), theirs("declined", 100), StatusMismatch, FixPaymentFailed},
		{"безопасно: возврат ждёт, у него проведён", ours(domain.PaymentSucceeded, 100, "pending"), theirs("refunded", 100), StatusMismatch, FixRefundSucceeded},
		{"безопасно: возврат на ручном разборе, у него проведён", ours(domain.PaymentSucceeded, 100, "manual_review"), theirs("refunded", 100), StatusMismatch, FixRefundSucceeded},

		{"опасно: у нас failed, у него списано", ours(domain.PaymentFailed, 100, ""), theirs("charged", 100), StatusMismatch, Manual},
		{"опасно: у нас expired, у него списано", ours(domain.PaymentExpired, 100, ""), theirs("charged", 100), StatusMismatch, Manual},
		{"опасно: у нас refunded, у него просто списано", ours(domain.PaymentRefunded, 100, "succeeded"), theirs("charged", 100), StatusMismatch, Manual},
		{"опасно: у нас paid, у него возврат без нашего запроса", ours(domain.PaymentSucceeded, 100, ""), theirs("refunded", 100), StatusMismatch, Manual},
		{"опасно: у нас paid, у него отказ", ours(domain.PaymentSucceeded, 100, ""), theirs("declined", 100), StatusMismatch, Manual},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := Classify(c.o, c.th)
			if f.Kind != c.kind || f.Action != c.action {
				t.Fatalf("got %s/%q, want %s/%q (note: %s)", f.Kind, f.Action, c.kind, c.action, f.Note)
			}
			if f.Kind != Matched && f.Note == "" {
				t.Fatal("у расхождения должно быть пояснение для человека")
			}
		})
	}
}
