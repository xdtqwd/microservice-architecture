-- +goose Up
CREATE TABLE refunds (
    id                 BIGSERIAL PRIMARY KEY,
    payment_id         BIGINT NOT NULL REFERENCES payments(id),
    amount             NUMERIC(12,2) NOT NULL CHECK (amount > 0),
    status             TEXT NOT NULL CHECK (status IN ('pending', 'succeeded', 'manual_review')),
    attempts           INT NOT NULL DEFAULT 0,
    next_attempt_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error         TEXT,
    provider_refund_id TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- полный возврат: не больше одного на платёж, повторная отмена не создаст второй
CREATE UNIQUE INDEX refunds_one_per_payment ON refunds (payment_id);
-- очередь воркера: только ожидающие, по времени следующей попытки
CREATE INDEX refunds_due ON refunds (next_attempt_at) WHERE status = 'pending';

-- +goose Down
DROP TABLE refunds;
