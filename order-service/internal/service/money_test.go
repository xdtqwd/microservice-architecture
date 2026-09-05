package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestFloat64MoneyPrecision(t *testing.T) {
	// float64 — проблема
	prices := []float64{0.1, 0.1, 0.1}
	var total float64
	for _, p := range prices {
		total += p
	}
	assert.NotEqual(t, 0.3, total) // 0.30000000000000004 — баг

	// decimal — решение
	d1 := decimal.NewFromFloat(0.1)
	d2 := decimal.NewFromFloat(0.1)
	d3 := decimal.NewFromFloat(0.1)
	totalDecimal := d1.Add(d2).Add(d3)
	assert.Equal(t, "0.3", totalDecimal.String()) // точно
}
