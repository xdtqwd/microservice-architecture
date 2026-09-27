package txm

import (
	"context"
	"time"
)

// FinishTimeout — предел для Commit и Rollback.
const FinishTimeout = 2 * time.Second

// Detached возвращает контекст, не связанный с запросом, но с пределом.
//
// Commit и Rollback нельзя делать на контексте запроса: если он отменён,
// pgx не отправит команду, и транзакция повиснет до разрыва соединения,
// а коммит на последнем шаге может оборваться посередине.
// Но и context.Background() без предела нельзя — при мёртвой сети вызов
// повиснет навсегда вместе с соединением из пула.
func Detached() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), FinishTimeout)
}
