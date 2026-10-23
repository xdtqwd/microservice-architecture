//go:build integration

package repository_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"order-service/internal/handler"
	"order-service/internal/payment"
	"order-service/internal/repository"
	"order-service/internal/service"

	"github.com/gorilla/mux"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// countingProvider считает вызовы. results[i] — исход i-го вызова (nil — успех).
type countingProvider struct {
	calls   atomic.Int64
	delay   time.Duration
	results []error
}

func (p *countingProvider) Charge(_ context.Context, key string, _ int, _ decimal.Decimal) (string, error) {
	n := int(p.calls.Add(1))
	time.Sleep(p.delay)
	if n <= len(p.results) && p.results[n-1] != nil {
		return "", p.results[n-1]
	}
	return "ch_" + key, nil
}

func newPayServer(t *testing.T, prov service.PaymentProvider) *httptest.Server {
	t.Helper()
	svc := service.NewPaymentService(repository.NewPaymentRepo(testPool, zap.NewNop()), prov, zap.NewNop())
	ph := handler.NewPaymentHandler(svc, repository.NewIdempotencyRepo(testPool), zap.NewNop())
	r := mux.NewRouter()
	r.Handle("/orders/{id}/pay", ph.Handler()).Methods("POST")
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func payHTTP(t *testing.T, srv *httptest.Server, orderID int, key, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/orders/"+strconv.Itoa(orderID)+"/pay", strings.NewReader(body))
	require.NoError(t, err)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// Критерий задачи: 50 одновременных запросов с одним ключом.
func TestPayIdempotency_50Goroutines_OneCharge_SameResponse(t *testing.T) {
	resetDB(t)
	orderID := newPendingOrder(t)
	prov := &countingProvider{delay: 200 * time.Millisecond}
	srv := newPayServer(t, prov)

	const n = 50
	codes := make([]int, n)
	bodies := make([]string, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			codes[i], bodies[i] = payHTTP(t, srv, orderID, "key-50", "")
		}(i)
	}
	close(start)
	wg.Wait()

	assert.EqualValues(t, 1, prov.calls.Load(), "провайдер вызван один раз")
	assert.Equal(t, 1, countRows(t, "payments"), "в базе один платёж")
	assert.Equal(t, "paid", orderStatus(t, orderID))
	for i := 1; i < n; i++ {
		assert.Equal(t, codes[0], codes[i], "код ответа %d", i)
		assert.Equal(t, bodies[0], bodies[i], "тело ответа %d", i)
	}
	assert.Equal(t, http.StatusOK, codes[0])
}

func TestPayIdempotency_MissingKey_400(t *testing.T) {
	resetDB(t)
	orderID := newPendingOrder(t)
	prov := &countingProvider{}
	srv := newPayServer(t, prov)

	code, _ := payHTTP(t, srv, orderID, "", "")

	assert.Equal(t, http.StatusBadRequest, code)
	assert.Zero(t, prov.calls.Load())
	assert.Equal(t, 0, countRows(t, "payments"))
}

func TestPayIdempotency_SameKeyDifferentRequest_422(t *testing.T) {
	resetDB(t)
	orderA := newPendingOrder(t)
	orderB := newPendingOrder(t)
	prov := &countingProvider{}
	srv := newPayServer(t, prov)

	code, _ := payHTTP(t, srv, orderA, "key-reused", "")
	require.Equal(t, http.StatusOK, code)

	code, body := payHTTP(t, srv, orderB, "key-reused", "")
	assert.Equal(t, http.StatusUnprocessableEntity, code, body)
	assert.EqualValues(t, 1, prov.calls.Load(), "второй заказ не оплачивался")
	assert.Equal(t, "pending", orderStatus(t, orderB))
}

func TestPayIdempotency_ReplayAfterCompletion_NoProviderCall(t *testing.T) {
	resetDB(t)
	orderID := newPendingOrder(t)
	prov := &countingProvider{}
	srv := newPayServer(t, prov)

	code1, body1 := payHTTP(t, srv, orderID, "key-replay", "")
	code2, body2 := payHTTP(t, srv, orderID, "key-replay", "")

	assert.Equal(t, http.StatusOK, code1)
	assert.Equal(t, code1, code2)
	assert.Equal(t, body1, body2, "повтор получает тот же ответ, а не 409 «уже оплачен»")
	assert.EqualValues(t, 1, prov.calls.Load())
}

func TestPayIdempotency_DeclineIsRemembered(t *testing.T) {
	resetDB(t)
	orderID := newPendingOrder(t)
	prov := &countingProvider{results: []error{payment.ErrDeclined}}
	srv := newPayServer(t, prov)

	code1, body1 := payHTTP(t, srv, orderID, "key-decline", "")
	code2, body2 := payHTTP(t, srv, orderID, "key-decline", "")

	assert.Equal(t, http.StatusPaymentRequired, code1)
	assert.Equal(t, code1, code2)
	assert.Equal(t, body1, body2)
	assert.EqualValues(t, 1, prov.calls.Load(), "отказ не переспрашиваем — ответ сохранён")
}

// Исход неизвестен (провайдер не ответил): ответ не сохраняется, повтор
// выполняется заново, находит тот же платёж по ключу и доводит его.
func TestPayIdempotency_UnknownOutcome_RetryResumesSamePayment(t *testing.T) {
	resetDB(t)
	orderID := newPendingOrder(t)
	prov := &countingProvider{results: []error{errors.New("provider timeout")}}
	srv := newPayServer(t, prov)

	code1, _ := payHTTP(t, srv, orderID, "key-unknown", "")
	require.GreaterOrEqual(t, code1, 500)
	require.Equal(t, "pending", orderStatus(t, orderID))

	code2, _ := payHTTP(t, srv, orderID, "key-unknown", "")

	assert.Equal(t, http.StatusOK, code2)
	assert.EqualValues(t, 2, prov.calls.Load(), "провайдера спросили снова с тем же ключом")
	assert.Equal(t, 1, countRows(t, "payments"), "платёж тот же, а не второй")
	assert.Equal(t, "paid", orderStatus(t, orderID))
}
