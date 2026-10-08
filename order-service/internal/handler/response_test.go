package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"order-service/internal/domain"

	"github.com/gorilla/mux"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// brokenWriter — клиент отвалился посреди ответа: первая запись тела падает.
type brokenWriter struct {
	*httptest.ResponseRecorder
	headerCalls int
	writes      int
}

func (b *brokenWriter) WriteHeader(code int) {
	b.headerCalls++
	b.ResponseRecorder.WriteHeader(code)
}

func (b *brokenWriter) Write(p []byte) (int, error) {
	b.writes++
	if b.writes == 1 {
		return 0, errors.New("client gone")
	}
	return b.ResponseRecorder.Write(p)
}

// FIX-03: после отправки заголовков ответ уже не исправить. Раньше хендлер
// на ошибке записи звал writeError: второй WriteHeader и JSON ошибки,
// дописанный в хвост начатого ответа.
func TestFix03_NoWritesAfterFailedResponse(t *testing.T) {
	product := domain.Product{ID: 1, Name: "A", Price: decimal.NewFromInt(100), Stock: 5}
	order := &domain.Order{ID: 3, Status: "pending"}

	cases := map[string]struct {
		h      http.HandlerFunc
		method string
		target string
		body   string
		ctype  string
		vars   map[string]string
	}{
		"GetProducts":    {h: newH(nil, &fakeProducts{products: []domain.Product{product}}).GetProducts, target: "/products"},
		"GetProductByID": {h: newH(nil, nil).GetProductByID, target: "/products/1", vars: map[string]string{"id": "1"}},
		"CreateOrder": {h: newH(&fakeOrders{createID: 7}, nil).CreateOrder, method: http.MethodPost, target: "/orders",
			body: `[{"product_id":1,"quantity":1}]`, ctype: "application/json"},
		"GetOrders":    {h: newH(&fakeOrders{orders: []domain.Order{*order}}, nil).GetOrders, target: "/orders"},
		"GetOrderByID": {h: newH(&fakeOrders{order: order}, nil).GetOrderByID, target: "/orders/3", vars: map[string]string{"id": "3"}},
		"CancelOrder":  {h: newH(nil, nil).CancelOrder, method: http.MethodPost, target: "/orders/3/cancel", vars: map[string]string{"id": "3"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			method := c.method
			if method == "" {
				method = http.MethodGet
			}
			r := httptest.NewRequest(method, c.target, strings.NewReader(c.body))
			if c.ctype != "" {
				r.Header.Set("Content-Type", c.ctype)
			}
			if c.vars != nil {
				r = mux.SetURLVars(r, c.vars)
			}
			w := &brokenWriter{ResponseRecorder: httptest.NewRecorder()}

			c.h(w, r)

			assert.LessOrEqual(t, w.headerCalls, 1, "заголовки отправляются один раз")
			assert.Equal(t, 1, w.writes, "после неудачной записи больше ничего не пишем")
		})
	}
}
