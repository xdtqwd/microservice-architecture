package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"order-service/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rounds = 30

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// runTogether запускает fn в n горутинах одновременно и возвращает их ошибки.
func runTogether(n int, fn func(i int) error) []error {
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = fn(i)
		}(i)
	}
	close(start)
	wg.Wait()
	return errs
}

func TestConcurrency_OppositeOrder_NoDeadlock(t *testing.T) {
	resetDB(t)
	a := seedProduct(t, "A", 100, 100000)
	b := seedProduct(t, "B", 100, 100000)
	repo := newOrderRepo()

	forward := []domain.OrderItem{{ProductID: a, Quantity: 1}, {ProductID: b, Quantity: 1}}
	reverse := []domain.OrderItem{{ProductID: b, Quantity: 1}, {ProductID: a, Quantity: 1}}

	deadlocks := 0
	for r := 0; r < rounds; r++ {
		errs := runTogether(4, func(i int) error {
			items := forward
			if i%2 == 1 {
				items = reverse
			}
			// копия: CreateOrder сортирует слайс на месте
			cp := append([]domain.OrderItem(nil), items...)
			_, _, err := createInTx(repo, cp, "")
			return err
		})
		for _, err := range errs {
			if pgCode(err) == "40P01" {
				deadlocks++
			} else {
				require.NoError(t, err)
			}
		}
	}
	assert.Zero(t, deadlocks, "дедлоков 40P01: %d из %d транзакций", deadlocks, rounds*4)
}

func TestConcurrency_LastItems_NoLostUpdate(t *testing.T) {
	resetDB(t)
	const stock, extra = 20, 10
	pid := seedProduct(t, "Hot", 100, stock)
	repo := newOrderRepo()

	errs := runTogether(stock+extra, func(int) error {
		_, _, err := createInTx(repo, []domain.OrderItem{{ProductID: pid, Quantity: 1}}, "")
		return err
	})

	ok, insufficient := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrInsufficientStock):
			insufficient++
		default:
			t.Errorf("неожиданная ошибка: %v", err)
		}
	}
	assert.Equal(t, stock, ok, "успешных заказов ровно столько, сколько было товара")
	assert.Equal(t, extra, insufficient)
	assert.Equal(t, 0, stockOf(t, pid), "остаток ровно ноль")
	assert.Equal(t, stock, countRows(t, "orders"))
}

func TestConcurrency_DoubleCancel_StockReturnedOnce(t *testing.T) {
	repo := newOrderRepo()
	for r := 0; r < rounds; r++ {
		resetDB(t)
		pid := seedProduct(t, "A", 100, 10)
		id := createOrder(t, repo, domain.OrderItem{ProductID: pid, Quantity: 3})

		errs := runTogether(2, func(int) error {
			_, err := repo.CancelOrder(context.Background(), id)
			return err
		})

		ok, already := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, domain.ErrOrderAlreadyCancelled):
				already++
			default:
				t.Fatalf("раунд %d: неожиданная ошибка: %v", r, err)
			}
		}
		require.Equal(t, 1, ok, "раунд %d: успешная отмена ровно одна", r)
		require.Equal(t, 1, already, "раунд %d", r)
		require.Equal(t, 10, stockOf(t, pid), "раунд %d: остаток вернулся один раз", r)
	}
}
