package domain

import "testing"

func TestCanTransitionPayment_AllPairs(t *testing.T) {
	all := []PaymentStatus{PaymentPending, PaymentSucceeded, PaymentFailed, PaymentExpired, PaymentRefunded}
	allowed := map[[2]PaymentStatus]bool{
		{PaymentPending, PaymentSucceeded}:  true,
		{PaymentPending, PaymentFailed}:     true,
		{PaymentPending, PaymentExpired}:    true,
		{PaymentSucceeded, PaymentRefunded}: true,
	}
	for _, from := range all {
		for _, to := range all {
			want := allowed[[2]PaymentStatus{from, to}]
			if got := CanTransitionPayment(from, to); got != want {
				t.Errorf("%s -> %s: got %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestPaymentStatus_IsTerminal(t *testing.T) {
	for s, want := range map[PaymentStatus]bool{
		PaymentPending: false, PaymentSucceeded: false,
		PaymentFailed: true, PaymentExpired: true, PaymentRefunded: true,
	} {
		if got := s.IsTerminal(); got != want {
			t.Errorf("%s.IsTerminal() = %v, want %v", s, got, want)
		}
	}
}
