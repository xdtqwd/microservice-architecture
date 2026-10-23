-- +goose Up
CREATE TABLE reconciliation_runs (
    id           BIGSERIAL PRIMARY KEY,
    period_from  TIMESTAMPTZ NOT NULL,
    period_to    TIMESTAMPTZ NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at  TIMESTAMPTZ,
    checked      INT NOT NULL DEFAULT 0,
    matched      INT NOT NULL DEFAULT 0,
    auto_fixed   INT NOT NULL DEFAULT 0,
    manual       INT NOT NULL DEFAULT 0
);

-- одна строка — одно найденное расхождение со всем, что нужно для разбора
CREATE TABLE reconciliation_items (
    id                 BIGSERIAL PRIMARY KEY,
    run_id             BIGINT NOT NULL REFERENCES reconciliation_runs(id),
    kind               TEXT NOT NULL,
    action             TEXT NOT NULL CHECK (action IN ('auto_fixed', 'manual')),
    idempotency_key    TEXT,
    payment_id         BIGINT,
    order_id           BIGINT,
    our_status         TEXT,
    our_amount         NUMERIC(12,2),
    provider_status    TEXT,
    provider_amount    NUMERIC(12,2),
    provider_charge_id TEXT,
    note               TEXT NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX reconciliation_items_run ON reconciliation_items (run_id);

-- +goose Down
DROP TABLE reconciliation_items;
DROP TABLE reconciliation_runs;
