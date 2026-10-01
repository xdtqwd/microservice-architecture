-- +goose Up
CREATE TABLE payments (
    id                  BIGSERIAL PRIMARY KEY,
    order_id            BIGINT NOT NULL REFERENCES orders(id),
    amount              NUMERIC(12,2) NOT NULL CHECK (amount > 0),
    currency            CHAR(3) NOT NULL DEFAULT 'RUB',
    status              TEXT NOT NULL
        CHECK (status IN ('pending', 'succeeded', 'failed', 'expired', 'refunded')),
    provider_payment_id TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Один активный платёж на заказ — правило схемы, а не кода.
-- Активный: ещё идёт (pending) или прошёл (succeeded). После failed, expired
-- и refunded строка выпадает из индекса, и можно начать новый платёж.
CREATE UNIQUE INDEX payments_one_active_per_order
    ON payments (order_id) WHERE status IN ('pending', 'succeeded');

-- История переходов: отдельная таблица, только дописывается.
CREATE TABLE payment_events (
    id          BIGSERIAL PRIMARY KEY,
    payment_id  BIGINT NOT NULL REFERENCES payments(id),
    from_status TEXT,
    to_status   TEXT NOT NULL,
    reason      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX payment_events_payment_id ON payment_events (payment_id);

-- +goose Down
DROP TABLE payment_events;
DROP TABLE payments;
