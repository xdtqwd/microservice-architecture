-- +goose Up
-- 001 создала колонку idempotency_key, а код и init.sql используют key.
-- На базах из init.sql колонка уже называется key — там ничего не делаем.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'idempotency_keys' AND column_name = 'idempotency_key'
    ) THEN
        ALTER TABLE idempotency_keys RENAME COLUMN idempotency_key TO key;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'idempotency_keys' AND column_name = 'key'
    ) THEN
        ALTER TABLE idempotency_keys RENAME COLUMN key TO idempotency_key;
    END IF;
END $$;
-- +goose StatementEnd
