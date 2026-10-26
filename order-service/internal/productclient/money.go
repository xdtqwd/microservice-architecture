package productclient

import (
	"errors"
	"fmt"
	"math"

	"order-service/internal/gen/productv1"

	"github.com/shopspring/decimal"
)

const nanosPerUnit = 1_000_000_000

var ErrInvalidMoney = errors.New("invalid money")

// MoneyToProto переводит точную сумму в units + nanos без потери точности.
// Больше девяти знаков после запятой Money не вмещает — это ошибка, а не округление.
func MoneyToProto(d decimal.Decimal, currency string) (*productv1.Money, error) {
	units := d.Truncate(0) // к нулю: знаки units и nanos совпадают
	if units.GreaterThan(decimal.NewFromInt(math.MaxInt64)) || units.LessThan(decimal.NewFromInt(math.MinInt64)) {
		return nil, fmt.Errorf("%w: %s out of int64 range", ErrInvalidMoney, d)
	}
	nanos := d.Sub(units).Shift(9)
	if !nanos.Equal(nanos.Truncate(0)) {
		return nil, fmt.Errorf("%w: %s has more than 9 fractional digits", ErrInvalidMoney, d)
	}
	return &productv1.Money{CurrencyCode: currency, Units: units.IntPart(), Nanos: int32(nanos.IntPart())}, nil
}

// MoneyFromProto проверяет инварианты Money и собирает точную сумму.
func MoneyFromProto(m *productv1.Money) (decimal.Decimal, error) {
	if m == nil {
		return decimal.Zero, fmt.Errorf("%w: nil", ErrInvalidMoney)
	}
	if m.Nanos <= -nanosPerUnit || m.Nanos >= nanosPerUnit {
		return decimal.Zero, fmt.Errorf("%w: nanos %d out of range", ErrInvalidMoney, m.Nanos)
	}
	if (m.Units > 0 && m.Nanos < 0) || (m.Units < 0 && m.Nanos > 0) {
		return decimal.Zero, fmt.Errorf("%w: units %d and nanos %d have different signs", ErrInvalidMoney, m.Units, m.Nanos)
	}
	return decimal.New(m.Units, 0).Add(decimal.New(int64(m.Nanos), -9)), nil
}
