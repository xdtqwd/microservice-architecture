package handler

import (
	"context"
	"net/http"
	"strconv"

	"order-service/internal/domain"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type PaymentService interface {
	Pay(ctx context.Context, orderID int, key string) (string, error)
}

type PaymentHandler struct {
	svc    PaymentService
	idem   IdempotencyStore
	logger *zap.Logger
}

func NewPaymentHandler(svc PaymentService, idem IdempotencyStore, logger *zap.Logger) *PaymentHandler {
	return &PaymentHandler{svc: svc, idem: idem, logger: logger}
}

// Handler — POST /orders/{id}/pay, обёрнутый в идемпотентность.
func (h *PaymentHandler) Handler() http.Handler {
	return Idempotent(h.idem, h.logger, http.HandlerFunc(h.Pay))
}

func (h *PaymentHandler) Pay(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		writeError(w, h.logger, domain.ErrOrderNotFound)
		return
	}
	chargeID, err := h.svc.Pay(r.Context(), id, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"order_id": id, "status": "paid", "charge_id": chargeID})
}
