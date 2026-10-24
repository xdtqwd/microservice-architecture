package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type pinger struct{ err error }

func (p pinger) Ping(context.Context) error { return p.err }

func status(h http.HandlerFunc) int {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	return rec.Code
}

var errDown = errors.New("down")

func TestHealth_PostgresDown(t *testing.T) {
	h := NewHealthHandler(pinger{err: errDown}, pinger{})
	assert.Equal(t, http.StatusOK, status(h.Liveness), "liveness не зависит от базы")
	assert.Equal(t, http.StatusServiceUnavailable, status(h.Readiness))
}

func TestHealth_RedisDown_StillReady(t *testing.T) {
	h := NewHealthHandler(pinger{}, pinger{err: errDown})
	assert.Equal(t, http.StatusOK, status(h.Liveness))
	assert.Equal(t, http.StatusOK, status(h.Readiness), "без Redis сервис работает — трафик принимаем")
}

func TestHealth_ShuttingDown_NotReadyButAlive(t *testing.T) {
	h := NewHealthHandler(pinger{}, pinger{})
	h.SetShuttingDown()
	assert.Equal(t, http.StatusServiceUnavailable, status(h.Readiness))
	assert.Equal(t, http.StatusOK, status(h.Liveness), "при остановке процесс жив, перезапускать нечего")
}
