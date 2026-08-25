package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"order-service/internal/domain"

	"go.uber.org/zap"
)

type errorResponse struct {
	Error string `json:"error"`
}

var errToStatus = map[error]int{
	domain.ErrOrderNotFound:            http.StatusNotFound,
	domain.ErrProductNotFound:          http.StatusNotFound,
	domain.ErrInsufficientStock:        http.StatusConflict,
	domain.ErrInvalidStatusTransition:  http.StatusConflict,
	domain.ErrOrderAlreadyCancelled: http.StatusConflict,
}

func writeError(w http.ResponseWriter, logger *zap.Logger, err error) {
	status := http.StatusInternalServerError

	for domainErr, code := range errToStatus {
		if errors.Is(err, domainErr) {
			status = code
			break
		}
	}

	if status == http.StatusInternalServerError {
		logger.Error("internal error", zap.Error(err))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	msg := http.StatusText(status)
	if status != http.StatusInternalServerError {
		msg = err.Error()
	}
	_ = json.NewEncoder(w).Encode(errorResponse{Error: msg})
}
