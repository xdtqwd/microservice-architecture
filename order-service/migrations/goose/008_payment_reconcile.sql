-- +goose Up
ALTER TABLE payments
    ADD COLUMN reconcile_attempts   INT NOT NULL DEFAULT 0,
    ADD COLUMN next_reconcile_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- заполнено — воркер больше не трогает, нужен человек
    ADD COLUMN manual_review_reason TEXT;

-- очередь сверки: только зависшие и не отданные человеку
CREATE INDEX payments_reconcile_queue ON payments (next_reconcile_at)
    WHERE status = 'pending' AND manual_review_reason IS NULL;

-- +goose Down
DROP INDEX payments_reconcile_queue;
ALTER TABLE payments
    DROP COLUMN manual_review_reason,
    DROP COLUMN next_reconcile_at,
    DROP COLUMN reconcile_attempts;
