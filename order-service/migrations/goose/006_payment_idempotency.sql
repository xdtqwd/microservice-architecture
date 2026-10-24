-- +goose Up
-- Ключ идемпотентности на самом платеже: повтор после сбоя находит свой
-- платёж и не создаёт второй. NULL допускается многократно.
ALTER TABLE payments ADD COLUMN idempotency_key TEXT;
CREATE UNIQUE INDEX payments_idempotency_key ON payments (idempotency_key);

-- Запросы POST /pay по ключу: отпечаток тела и сохранённый ответ.
CREATE TABLE payment_requests (
    key           TEXT PRIMARY KEY,
    request_hash  TEXT NOT NULL,
    status        TEXT NOT NULL CHECK (status IN ('in_progress', 'completed')),
    response_code INT,
    response_body BYTEA,
    -- аренда: пока не истекла, ключ занят выполняющимся запросом
    locked_until  TIMESTAMPTZ NOT NULL,
    -- срок жизни ключа; после него ключ можно использовать заново
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX payment_requests_expires_at ON payment_requests (expires_at);

-- +goose Down
DROP TABLE payment_requests;
DROP INDEX payments_idempotency_key;
ALTER TABLE payments DROP COLUMN idempotency_key;
