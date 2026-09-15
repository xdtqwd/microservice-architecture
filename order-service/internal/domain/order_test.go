package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanTransition(t *testing.T) {
	tests := []struct {
		from string
		to   string
		ok   bool
	}{
		{"pending", "paid", true},
		{"pending", "cancelled", true},
		{"paid", "shipped", true},
		{"shipped", "delivered", true},
		{"delivered", "cancelled", false},
		{"cancelled", "paid", false},
		{"pending", "delivered", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.ok, CanTransition(tt.from, tt.to), "%s→%s", tt.from, tt.to)
	}
}
