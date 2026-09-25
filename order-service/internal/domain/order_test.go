package domain

import "testing"

// Мутант :55 (s == to -> s != to): у CanTransition не было ни одного теста.
func TestCanTransition(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{"pending", "paid", true},
		{"pending", "cancelled", true},
		{"paid", "shipped", true},
		{"paid", "cancelled", true},
		{"shipped", "delivered", true},
		{"pending", "shipped", false},
		{"pending", "delivered", false},
		{"shipped", "cancelled", false},
		{"delivered", "cancelled", false},
		{"cancelled", "paid", false},
		{"cancelled", "cancelled", false},
		{"unknown", "paid", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := CanTransition(c.from, c.to); got != c.want {
			t.Errorf("CanTransition(%q, %q) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}
