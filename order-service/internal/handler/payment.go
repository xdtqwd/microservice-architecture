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
	Pay(ctx context.Context, orderID int) (string, error)
}

type PaymentHandler struct {
	svc    PaymentService
	logger *zap.Logger
}

func NewPaymentHandler(svc PaymentService, logger *zap.Logger) *PaymentHandler {
	return &PaymentHandler{svc: svc, logger: logger}
}

func (h *PaymentHandler) Pay(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		writeError(w, h.logger, domain.ErrOrderNotFound)
		return
	}
	chargeID, err := h.svc.Pay(r.Context(), id)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"order_id": id, "status": "paid", "charge_id": chargeID})
}
