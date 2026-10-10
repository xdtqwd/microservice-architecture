package handler

import (
	"fmt"
	"net/http"
	"testing"

	"order-service/internal/domain"

	"github.com/stretchr/testify/assert"
)

// FIX-01 / BUG-03: нечисловой id — ошибка разбора (400), а не «не найдено» (404).
func TestNonNumericID_400(t *testing.T) {
	h := newH(nil, nil)
	for name, rec := range map[string]interface{ Result() *http.Response }{
		"GET /orders/abc":         do(h.GetOrderByID, req{target: "/orders/abc", vars: map[string]string{"id": "abc"}}),
		"POST /orders/abc/cancel": do(h.CancelOrder, req{method: http.MethodPost, target: "/orders/abc/cancel", vars: map[string]string{"id": "abc"}}),
		"GET /products/abc":       do(h.GetProductByID, req{target: "/products/abc", vars: map[string]string{"id": "abc"}}),
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, http.StatusBadRequest, rec.Result().StatusCode)
		})
	}
}

// Наружу — только публичный текст, без цепочки обёрток с именами функций.
func TestErrorBody_NoInternalNames(t *testing.T) {
	wrapped := fmt.Errorf("CreateOrder get price: %w", domain.ErrProductNotFound)
	rec := postOrder(newH(&fakeOrders{err: wrapped}, nil), `[{"product_id":1,"quantity":1}]`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.JSONEq(t, `{"error":"product not found"}`, rec.Body.String())
}

// Пояснение к невалидному вводу клиенту полезно — его оставляем.
func TestValidationError_KeepsReason(t *testing.T) {
	rec := postOrder(newH(&fakeOrders{err: domain.Invalid("quantity must be greater than 0")}, nil),
		`[{"product_id":1,"quantity":1}]`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.JSONEq(t, `{"error":"invalid request: quantity must be greater than 0"}`, rec.Body.String())
}
