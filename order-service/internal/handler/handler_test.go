package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"order-service/internal/domain"

	"github.com/gorilla/mux"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------- фейки ----------

type fakeOrders struct {
	err       error
	createID  int
	exists    bool
	gotLimit  int
	gotCursor *domain.OrderCursor
	orders    []domain.Order
	next      *domain.OrderCursor
	order     *domain.Order
}

func (f *fakeOrders) CreateOrder(_ context.Context, _ []domain.CreateOrderItem, _ string) (int, bool, error) {
	return f.createID, f.exists, f.err
}
func (f *fakeOrders) GetOrders(_ context.Context, limit int, c *domain.OrderCursor) ([]domain.Order, *domain.OrderCursor, error) {
	f.gotLimit, f.gotCursor = limit, c
	return f.orders, f.next, f.err
}
func (f *fakeOrders) GetOrderByID(_ context.Context, _ int) (*domain.Order, error) {
	return f.order, f.err
}
func (f *fakeOrders) CancelOrder(_ context.Context, id int) (int, error) {
	return id, f.err
}

type fakeProducts struct {
	err      error
	products []domain.Product
}

func (f *fakeProducts) GetProducts(context.Context) ([]domain.Product, error) {
	return f.products, f.err
}
func (f *fakeProducts) GetProductByID(_ context.Context, id int) (*domain.Product, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &domain.Product{ID: id, Name: "A", Price: decimal.NewFromInt(100), Stock: 5}, nil
}

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func newH(o *fakeOrders, p *fakeProducts) *Handler {
	if o == nil {
		o = &fakeOrders{}
	}
	if p == nil {
		p = &fakeProducts{}
	}
	return New(o, p, zap.NewNop())
}

type req struct {
	method, target, body, contentType string
	vars                              map[string]string
}

func do(h http.HandlerFunc, r req) *httptest.ResponseRecorder {
	if r.method == "" {
		r.method = http.MethodGet
	}
	hr := httptest.NewRequest(r.method, r.target, strings.NewReader(r.body))
	if r.contentType != "" {
		hr.Header.Set("Content-Type", r.contentType)
	}
	if r.vars != nil {
		hr = mux.SetURLVars(hr, r.vars)
	}
	rec := httptest.NewRecorder()
	h(rec, hr)
	return rec
}

func postOrder(h *Handler, body string) *httptest.ResponseRecorder {
	return do(h.CreateOrder, req{method: http.MethodPost, target: "/orders", body: body, contentType: "application/json"})
}

// ---------- карта ошибок ----------

func TestWriteError_EveryDomainErrorHasItsStatus(t *testing.T) {
	for domainErr, want := range errToStatus {
		t.Run(domainErr.Error(), func(t *testing.T) {
			rec := httptest.NewRecorder()
			// обёрнутая ошибка — так она приходит из сервиса
			writeError(rec, zap.NewNop(), fmt.Errorf("repo: %w", domainErr))
			assert.Equal(t, want, rec.Code)
			assert.Contains(t, rec.Body.String(), domainErr.Error())
		})
	}
}

func TestWriteError_UnknownIs500AndDoesNotLeak(t *testing.T) {
	rec := httptest.NewRecorder()
	leak := errors.New(`ERROR: relation "orders" does not exist, host=10.0.3.7 user=postgres`)
	writeError(rec, zap.NewNop(), leak)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "Internal Server Error")
	assert.NotContains(t, rec.Body.String(), "relation")
	assert.NotContains(t, rec.Body.String(), "10.0.3.7")
}

// ---------- CreateOrder ----------

func TestCreateOrder_New201_Repeat200(t *testing.T) {
	rec := postOrder(newH(&fakeOrders{createID: 7}, nil), `[{"product_id":1,"quantity":2}]`)
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.JSONEq(t, `{"id":7}`, rec.Body.String())

	rec = postOrder(newH(&fakeOrders{createID: 7, exists: true}, nil), `[{"product_id":1,"quantity":2}]`)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCreateOrder_BadInput_400(t *testing.T) {
	many := "[" + strings.Repeat(`{"product_id":1,"quantity":1},`, 100) + `{"product_id":1,"quantity":1}]`
	cases := map[string]string{
		"кривой JSON":        `[{"product_id":`,
		"не массив":          `{"product_id":1}`,
		"пустой список":      `[]`,
		"больше 100 позиций": many,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := postOrder(newH(nil, nil), body)
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		})
	}
}

func TestCreateOrder_BodyTooLarge_413(t *testing.T) {
	big := `[{"product_id":1,"quantity":1,"pad":"` + strings.Repeat("x", 1<<17) + `"}]`
	rec := postOrder(newH(nil, nil), big)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestCreateOrder_WrongContentType_415(t *testing.T) {
	rec := do(newH(nil, nil).CreateOrder, req{
		method: http.MethodPost, target: "/orders",
		body: `[{"product_id":1,"quantity":1}]`, contentType: "text/plain",
	})
	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}

func TestCreateOrder_ServiceErrorMapped(t *testing.T) {
	rec := postOrder(newH(&fakeOrders{err: domain.ErrInsufficientStock}, nil), `[{"product_id":1,"quantity":2}]`)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// ---------- GetOrders ----------

func TestGetOrders_OK_WithNextCursor(t *testing.T) {
	f := &fakeOrders{
		orders: []domain.Order{{ID: 5, Status: "pending"}},
		next:   &domain.OrderCursor{AfterID: 5},
	}
	rec := do(newH(f, nil).GetOrders, req{target: "/orders?limit=1&after_id=10"})

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"next_after_id":5`)
	assert.Equal(t, 1, f.gotLimit)
	require.NotNil(t, f.gotCursor)
	assert.Equal(t, 10, f.gotCursor.AfterID)
}

func TestGetOrders_NoLimit_UsesServiceDefault(t *testing.T) {
	f := &fakeOrders{}
	rec := do(newH(f, nil).GetOrders, req{target: "/orders"})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, f.gotLimit, "без limit сервис подставит значение по умолчанию")
}

func TestGetOrders_BadQuery_400(t *testing.T) {
	for _, q := range []string{
		"limit=abc", "limit=-1", "limit=0", "limit=101",
		"after_id=abc", "after_id=0", "offset=10",
	} {
		t.Run(q, func(t *testing.T) {
			f := &fakeOrders{}
			rec := do(newH(f, nil).GetOrders, req{target: "/orders?" + q})
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		})
	}
}

// ---------- GetOrderByID / CancelOrder ----------

func TestGetOrderByID(t *testing.T) {
	h := newH(&fakeOrders{order: &domain.Order{ID: 3, Status: "pending"}}, nil)
	rec := do(h.GetOrderByID, req{target: "/orders/3", vars: map[string]string{"id": "3"}})
	assert.Equal(t, http.StatusOK, rec.Code)

	h = newH(&fakeOrders{err: domain.ErrOrderNotFound}, nil)
	rec = do(h.GetOrderByID, req{target: "/orders/9", vars: map[string]string{"id": "9"}})
	assert.Equal(t, http.StatusNotFound, rec.Code)

	rec = do(h.GetOrderByID, req{target: "/orders/x", vars: map[string]string{"id": "x"}})
	assert.Less(t, rec.Code, 500, "нечисловой id — ошибка клиента, не сервера")
}

func TestCancelOrder(t *testing.T) {
	rec := do(newH(nil, nil).CancelOrder, req{method: http.MethodPost, vars: map[string]string{"id": "3"}, target: "/orders/3/cancel"})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(newH(&fakeOrders{err: domain.ErrOrderAlreadyCancelled}, nil).CancelOrder,
		req{method: http.MethodPost, vars: map[string]string{"id": "3"}, target: "/orders/3/cancel"})
	assert.Equal(t, http.StatusConflict, rec.Code)

	rec = do(newH(nil, nil).CancelOrder, req{method: http.MethodPost, vars: map[string]string{"id": "x"}, target: "/orders/x/cancel"})
	assert.Less(t, rec.Code, 500)
}

// ---------- Products ----------

func TestProducts(t *testing.T) {
	p := &fakeProducts{products: []domain.Product{{ID: 1, Name: "A", Price: decimal.NewFromInt(100)}}}
	rec := do(newH(nil, p).GetProducts, req{target: "/products"})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"A"`)

	rec = do(newH(nil, &fakeProducts{err: errors.New("db down")}).GetProducts, req{target: "/products"})
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	rec = do(newH(nil, nil).GetProductByID, req{target: "/products/1", vars: map[string]string{"id": "1"}})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(newH(nil, &fakeProducts{err: domain.ErrProductNotFound}).GetProductByID,
		req{target: "/products/9", vars: map[string]string{"id": "9"}})
	assert.Equal(t, http.StatusNotFound, rec.Code)

	rec = do(newH(nil, nil).GetProductByID, req{target: "/products/x", vars: map[string]string{"id": "x"}})
	assert.Less(t, rec.Code, 500)
}

// ---------- Health ----------

func TestHealth(t *testing.T) {
	ok := NewHealthHandler(fakePinger{}, fakePinger{})
	down := NewHealthHandler(fakePinger{}, fakePinger{err: errors.New("redis down")})

	assert.Equal(t, http.StatusOK, do(ok.Liveness, req{target: "/healthz"}).Code)
	assert.Equal(t, http.StatusOK, do(down.Liveness, req{target: "/healthz"}).Code, "liveness не ходит в зависимости")
	assert.Equal(t, http.StatusOK, do(ok.Readiness, req{target: "/readyz"}).Code)
	assert.Equal(t, http.StatusServiceUnavailable, do(down.Readiness, req{target: "/readyz"}).Code)
}

// ---------- Middleware ----------

func TestRequestID_HeaderPresentAndUnique(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ids := map[string]bool{}
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		id := rec.Header().Get("X-Request-Id")
		assert.NotEmpty(t, id)
		assert.False(t, ids[id], "request id должен быть уникальным")
		ids[id] = true
	}
}

// Настоящий сервер, а не recorder: без Recover net/http оборвёт соединение,
// и клиент получит ошибку вместо ответа.
func TestChain_PanicGives500AndServerStaysAlive(t *testing.T) {
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			panic("boom")
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(Chain(zap.NewNop(), next))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	require.NoError(t, err, "паника не должна рвать соединение")
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.NotContains(t, string(body), "boom", "текст паники не уходит клиенту")
	assert.NotEmpty(t, resp.Header.Get("X-Request-Id"))

	resp, err = http.Get(srv.URL)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "процесс жив и обслуживает следующий запрос")
}

func TestTimeout_SetsDeadline(t *testing.T) {
	var deadline time.Time
	var has bool
	h := Timeout(time.Second)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline, has = r.Context().Deadline()
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	assert.True(t, has)
	assert.WithinDuration(t, time.Now().Add(time.Second), deadline, 200*time.Millisecond)
}

func TestLogger_PassesStatusThrough(t *testing.T) {
	h := Logger(zap.NewNop())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("ok"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(nil)))
	assert.Equal(t, http.StatusTeapot, rec.Code)
}
