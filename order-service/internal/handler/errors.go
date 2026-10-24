package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"order-service/internal/domain"

	"go.uber.org/zap"
)

type errorResponse struct {
	Error string `json:"error"`
}

var errToStatus = map[error]int{
	domain.ErrOrderNotFound:           http.StatusNotFound,
	domain.ErrProductNotFound:         http.StatusNotFound,
	domain.ErrInsufficientStock:       http.StatusConflict,
	domain.ErrInvalidStatusTransition: http.StatusConflict,
	domain.ErrOrderAlreadyCancelled:   http.StatusConflict,
	domain.ErrInvalidCursor:           http.StatusBadRequest,
	domain.ErrOffsetNotSupported:      http.StatusBadRequest,
	domain.ErrInvalidRequest:          http.StatusBadRequest,
	domain.ErrUnsupportedMediaType:    http.StatusUnsupportedMediaType,
	domain.ErrPaymentDeclined:         http.StatusPaymentRequired,
	domain.ErrPaymentAlreadyActive:    http.StatusConflict,
	domain.ErrPaymentTransition:       http.StatusConflict,
	domain.ErrPaymentNotFound:         http.StatusNotFound,
	domain.ErrPaymentInProgress:       http.StatusConflict,
	// не дождались соединения из пула или общий дедлайн запроса:
	// сервис перегружен, но жив — клиенту стоит повторить позже
	context.DeadlineExceeded: http.StatusServiceUnavailable,
}

func writeError(w http.ResponseWriter, logger *zap.Logger, err error) {
	status := http.StatusInternalServerError
	var matched error

	for domainErr, code := range errToStatus {
		if errors.Is(err, domainErr) {
			status = code
			matched = domainErr
			break
		}
	}

	if status == http.StatusInternalServerError {
		logger.Error("internal error", zap.Error(err))
	}

	w.Header().Set("Content-Type", "application/json")
	if status == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", "1")
	}
	w.WriteHeader(status)

	// Наружу — только публичный текст: текст доменной ошибки или пояснение
	// к невалидному вводу. Цепочка обёрток с именами функций — только в лог:
	// это деталь реализации, а не часть API.
	msg := http.StatusText(status)
	if status < http.StatusInternalServerError && matched != nil {
		msg = matched.Error()
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			msg = ve.Error()
		}
	}
	_ = json.NewEncoder(w).Encode(errorResponse{Error: msg})
}

// respond отправляет успешный ответ. Тело кодируется в буфер ДО заголовков:
// ошибка кодирования ещё может стать честным 500. Если же упала запись,
// заголовки уже ушли — исправить ответ нельзя, только записать в лог (FIX-03).
func respond(w http.ResponseWriter, logger *zap.Logger, status int, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		writeError(w, logger, fmt.Errorf("encode response: %w", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(buf.Bytes()); err != nil {
		logger.Warn("write response failed, client likely gone", zap.Error(err))
	}
}
