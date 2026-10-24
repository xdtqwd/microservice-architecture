package repository

import (
	"order-service/internal/txm"

	"github.com/jackc/pgx/v5"
)

// commitDetached коммитит на контексте с пределом, не связанном с запросом.
func commitDetached(tx pgx.Tx) error {
	ctx, cancel := txm.Detached()
	defer cancel()
	return tx.Commit(ctx)
}
