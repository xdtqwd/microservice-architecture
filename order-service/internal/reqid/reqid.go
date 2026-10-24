// Package reqid — идентификатор запроса, который проходит через HTTP,
// gRPC и логи всех сервисов.
package reqid

import (
	"context"
	"regexp"

	"github.com/google/uuid"
)

const (
	Header      = "X-Request-Id"
	MetadataKey = "x-request-id" // ключи gRPC-метаданных — в нижнем регистре
)

type ctxKey struct{}

// безопасный id: без пробелов и переводов строк, чтобы не подделать строки лога
var valid = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func From(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// OrNew возвращает пришедший id, если он безопасный, иначе новый.
func OrNew(incoming string) string {
	if valid.MatchString(incoming) {
		return incoming
	}
	return uuid.New().String()
}
