package main

import (
	"fmt"
	"math"

	"product-service/gen/productv1"

	"github.com/shopspring/decimal"
)

// moneyToProto — точная сумма в units + nanos, без float.
func moneyToProto(d decimal.Decimal, currency string) (*productv1.Money, error) {
	units := d.Truncate(0)
	if units.GreaterThan(decimal.NewFromInt(math.MaxInt64)) || units.LessThan(decimal.NewFromInt(math.MinInt64)) {
		return nil, fmt.Errorf("money %s out of int64 range", d)
	}
	nanos := d.Sub(units).Shift(9)
	if !nanos.Equal(nanos.Truncate(0)) {
		return nil, fmt.Errorf("money %s has more than 9 fractional digits", d)
	}
	return &productv1.Money{CurrencyCode: currency, Units: units.IntPart(), Nanos: int32(nanos.IntPart())}, nil
}
