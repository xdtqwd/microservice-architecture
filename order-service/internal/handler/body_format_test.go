package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"order-service/internal/domain"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var v T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &v), "тело — валидный JSON: %s", rec.Body.String())
	return v
}

func TestCreateOrder_EmptyBody_400(t *testing.T) {
	rec := postOrder(newH(nil, nil), "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.NotEmpty(t, decode[map[string]string](t, rec)["error"])
}

// Ошибки: всегда {"error": "..."} — клиенты разбирают их одинаково.
func TestErrorBody_Format(t *testing.T) {
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"400": postOrder(newH(nil, nil), `[{"product_id":`),
		"404": do(newH(&fakeOrders{err: domain.ErrOrderNotFound}, nil).GetOrderByID, req{target: "/orders/9", vars: map[string]string{"id": "9"}}),
		"409": postOrder(newH(&fakeOrders{err: domain.ErrInsufficientStock}, nil), `[{"product_id":1,"quantity":1}]`),
		"500": do(newH(nil, &fakeProducts{err: errors.New("db down")}).GetProducts, req{target: "/products"}),
	} {
		t.Run(name, func(t *testing.T) {
			body := decode[map[string]string](t, rec)
			assert.NotEmpty(t, body["error"])
			assert.Len(t, body, 1, "ничего лишнего в теле ошибки")
		})
	}
}

// Маленькие фиксированные ответы — тело целиком: в них нечему меняться.
func TestBody_SmallResponsesExact(t *testing.T) {
	rec := postOrder(newH(&fakeOrders{createID: 7}, nil), `[{"product_id":1,"quantity":1}]`)
	assert.JSONEq(t, `{"id":7}`, rec.Body.String())

	rec = do(newH(nil, nil).CancelOrder, req{method: http.MethodPost, target: "/orders/3/cancel", vars: map[string]string{"id": "3"}})
	assert.JSONEq(t, `{"cancelled_id":3}`, rec.Body.String())
}

// Сущности — ключевые поля: добавление нового поля тест не ломает.
func TestBody_ProductFields(t *testing.T) {
	p := &fakeProducts{products: []domain.Product{{ID: 1, Name: "A", Price: decimal.RequireFromString("150000.50"), Stock: 5}}}
	list := decode[[]map[string]any](t, do(newH(nil, p).GetProducts, req{target: "/products"}))
	require.Len(t, list, 1)
	assert.EqualValues(t, 1, list[0]["id"])
	assert.Equal(t, "A", list[0]["name"])
	assert.EqualValues(t, 5, list[0]["stock"])
	assert.Contains(t, list[0], "price")

	one := decode[map[string]any](t, do(newH(nil, nil).GetProductByID, req{target: "/products/1", vars: map[string]string{"id": "1"}}))
	assert.EqualValues(t, 1, one["id"])
}

func TestBody_OrdersFields(t *testing.T) {
	f := &fakeOrders{orders: []domain.Order{{ID: 5, Status: "pending"}}, next: &domain.OrderCursor{AfterID: 5}}
	page := decode[map[string]any](t, do(newH(f, nil).GetOrders, req{target: "/orders"}))
	orders := page["orders"].([]any)
	require.Len(t, orders, 1)
	assert.EqualValues(t, 5, orders[0].(map[string]any)["id"])
	assert.Equal(t, "pending", orders[0].(map[string]any)["status"])
	assert.EqualValues(t, 5, page["next_after_id"])

	last := decode[map[string]any](t, do(newH(&fakeOrders{}, nil).GetOrders, req{target: "/orders"}))
	assert.NotContains(t, last, "next_after_id", "на последней странице курсора нет")

	one := decode[map[string]any](t, do(newH(&fakeOrders{order: &domain.Order{ID: 3, Status: "paid"}}, nil).GetOrderByID,
		req{target: "/orders/3", vars: map[string]string{"id": "3"}}))
	assert.EqualValues(t, 3, one["id"])
	assert.Equal(t, "paid", one["status"])
}
