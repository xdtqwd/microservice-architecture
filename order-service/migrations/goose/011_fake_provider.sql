-- +goose Up
-- Таблицы фейкового платёжного провайдера. Раньше их создавал сервис
-- на старте (CREATE TABLE / ALTER TABLE / CREATE INDEX IF NOT EXISTS).
-- При одновременном старте реплик они гонялись: IF NOT EXISTS не атомарен,
-- второй падал с duplicate key ... pg_class_relname_nsp_index.
-- IF NOT EXISTS здесь оставлен для баз, где таблицы уже создал старый код.
CREATE TABLE IF NOT EXISTS fake_provider_charges (
    id         BIGSERIAL PRIMARY KEY,
    order_id   INT NOT NULL,
    amount     NUMERIC(12,2) NOT NULL,
    status     TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- провайдер сам дедуплицирует списания по ключу идемпотентности
ALTER TABLE fake_provider_charges ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS fake_provider_charges_key
    ON fake_provider_charges (idempotency_key);

CREATE TABLE IF NOT EXISTS fake_provider_refunds (
    id              BIGSERIAL PRIMARY KEY,
    idempotency_key TEXT UNIQUE NOT NULL,
    charge_id       TEXT NOT NULL,
    amount          NUMERIC(12,2) NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- переключатель доступности, можно менять на лету:
-- UPDATE fake_provider_settings SET down = true
CREATE TABLE IF NOT EXISTS fake_provider_settings (
    id   INT PRIMARY KEY,
    down BOOLEAN NOT NULL DEFAULT false
);
INSERT INTO fake_provider_settings (id) VALUES (1) ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS fake_provider_settings;
DROP TABLE IF EXISTS fake_provider_refunds;
DROP TABLE IF EXISTS fake_provider_charges;
