package productclient

import (
	"testing"

	"order-service/internal/gen/productv1"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestMoney_RoundTripExact(t *testing.T) {
	for _, s := range []string{"150000", "150000.50", "0.10", "0.01", "-12.34", "0", "999999999999.999999999"} {
		t.Run(s, func(t *testing.T) {
			want := decimal.RequireFromString(s)
			m, err := MoneyToProto(want, "RUB")
			require.NoError(t, err)

			// через провод: сериализация и разбор, как между сервисами
			wire, err := proto.Marshal(m)
			require.NoError(t, err)
			var back productv1.Money
			require.NoError(t, proto.Unmarshal(wire, &back))

			got, err := MoneyFromProto(&back)
			require.NoError(t, err)
			assert.True(t, want.Equal(got), "want %s, got %s", want, got)
		})
	}
}

func TestMoney_Layout(t *testing.T) {
	m, err := MoneyToProto(decimal.RequireFromString("150000.50"), "RUB")
	require.NoError(t, err)
	assert.Equal(t, int64(150000), m.Units)
	assert.Equal(t, int32(500_000_000), m.Nanos)

	m, err = MoneyToProto(decimal.RequireFromString("-12.34"), "RUB")
	require.NoError(t, err)
	assert.Equal(t, int64(-12), m.Units, "отрицательные: к нулю, знаки совпадают")
	assert.Equal(t, int32(-340_000_000), m.Nanos)
}

// То, ради чего не double: сумма копеек остаётся точной.
func TestMoney_NoFloatDrift(t *testing.T) {
	sum := decimal.Zero
	for i := 0; i < 10; i++ {
		m, err := MoneyToProto(decimal.RequireFromString("0.1"), "RUB")
		require.NoError(t, err)
		v, err := MoneyFromProto(m)
		require.NoError(t, err)
		sum = sum.Add(v)
	}
	assert.Equal(t, "1", sum.String())

	f := 0.0
	for i := 0; i < 10; i++ {
		f += 0.1
	}
	assert.NotEqual(t, 1.0, f, "с double так не выйдет: %v", f)
}

func TestMoney_Invalid(t *testing.T) {
	_, err := MoneyToProto(decimal.RequireFromString("0.0000000001"), "RUB")
	assert.ErrorIs(t, err, ErrInvalidMoney, "10 знаков — не округляем молча")

	for name, m := range map[string]*productv1.Money{
		"nil":             nil,
		"nanos too big":   {Units: 1, Nanos: 1_000_000_000},
		"different signs": {Units: 1, Nanos: -1},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := MoneyFromProto(m)
			assert.ErrorIs(t, err, ErrInvalidMoney)
		})
	}
}
