package repository

import (
	"context"
	"errors"
	"time"

	"order-service/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// Аренда ключа: дольше дедлайна запроса (10s), чтобы живой запрос
	// не перехватили, и не слишком долго — после падения сервиса повтор
	// подхватит работу через это время.
	idempotencyLease = 15 * time.Second

	// Срок жизни ключа. Клиенты повторяют в пределах минут или часов
	// (мобильная сеть, ретраи с бэкоффом, повторная отправка формы).
	// Сутки покрывают это с запасом и совпадают с практикой провайдеров.
	// Дольше хранить — расти таблице без пользы.
	idempotencyTTL = 24 * time.Hour
)

type IdempotencyRepo struct{ pool *pgxpool.Pool }

func NewIdempotencyRepo(pool *pgxpool.Pool) *IdempotencyRepo { return &IdempotencyRepo{pool: pool} }

// Claim атомарно занимает ключ или сообщает, что с ним уже было.
// INSERT ... ON CONFLICT DO NOTHING — одна операция, а не SELECT, затем INSERT.
func (r *IdempotencyRepo) Claim(ctx context.Context, key, hash string) (domain.IdempotencyClaim, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.IdempotencyClaim{}, err
	}
	defer rollbackDetached(tx)

	tag, err := tx.Exec(ctx, `
		INSERT INTO payment_requests (key, request_hash, status, locked_until, expires_at)
		VALUES ($1, $2, 'in_progress', NOW() + make_interval(secs => $3), NOW() + make_interval(secs => $4))
		ON CONFLICT (key) DO NOTHING`,
		key, hash, idempotencyLease.Seconds(), idempotencyTTL.Seconds())
	if err != nil {
		return domain.IdempotencyClaim{}, err
	}
	if tag.RowsAffected() == 1 {
		return domain.IdempotencyClaim{Owner: true}, commitDetached(tx)
	}

	var storedHash, status string
	var code *int
	var body []byte
	var leaseExpired, expired bool
	err = tx.QueryRow(ctx, `
		SELECT request_hash, status, response_code, response_body,
		       locked_until < NOW(), expires_at < NOW()
		FROM payment_requests WHERE key = $1 FOR UPDATE`, key).
		Scan(&storedHash, &status, &code, &body, &leaseExpired, &expired)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.IdempotencyClaim{Busy: true}, nil // удалили между запросами — пусть клиент повторит
	}
	if err != nil {
		return domain.IdempotencyClaim{}, err
	}

	switch {
	case expired:
		// срок ключа вышел — используем его заново как новый
		_, err = tx.Exec(ctx, `
			UPDATE payment_requests
			SET request_hash = $2, status = 'in_progress', response_code = NULL, response_body = NULL,
			    locked_until = NOW() + make_interval(secs => $3),
			    expires_at = NOW() + make_interval(secs => $4), created_at = NOW()
			WHERE key = $1`, key, hash, idempotencyLease.Seconds(), idempotencyTTL.Seconds())
		if err != nil {
			return domain.IdempotencyClaim{}, err
		}
		return domain.IdempotencyClaim{Owner: true}, commitDetached(tx)
	case storedHash != hash:
		return domain.IdempotencyClaim{Mismatch: true}, nil
	case status == "completed":
		c := 0
		if code != nil {
			c = *code
		}
		return domain.IdempotencyClaim{Code: c, Body: body}, nil
	case !leaseExpired:
		return domain.IdempotencyClaim{Busy: true}, nil
	}

	// аренда истекла, а ответа нет: предыдущий исполнитель упал — подхватываем
	if _, err := tx.Exec(ctx,
		"UPDATE payment_requests SET locked_until = NOW() + make_interval(secs => $2) WHERE key = $1",
		key, idempotencyLease.Seconds()); err != nil {
		return domain.IdempotencyClaim{}, err
	}
	return domain.IdempotencyClaim{Owner: true}, commitDetached(tx)
}

// Complete сохраняет окончательный ответ.
func (r *IdempotencyRepo) Complete(ctx context.Context, key string, code int, body []byte) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE payment_requests SET status = 'completed', response_code = $2, response_body = $3
		WHERE key = $1`, key, code, body)
	return err
}

// Release отпускает аренду без сохранения ответа: исход неизвестен,
// повтор с тем же ключом выполнится заново и сразу.
func (r *IdempotencyRepo) Release(ctx context.Context, key string) error {
	_, err := r.pool.Exec(ctx,
		"UPDATE payment_requests SET locked_until = NOW() WHERE key = $1 AND status = 'in_progress'", key)
	return err
}
