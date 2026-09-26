-- +goose NO TRANSACTION
-- +goose Up
-- Частичный индекс только по неопубликованным строкам.
-- Обычный индекс по published_at хранил бы все строки, в том числе миллионы
-- опубликованных, и рос бы вместе с историей. Частичный содержит ровно рабочую
-- очередь: сотни строк при нормальной работе, и релей находит их за пару
-- чтений страниц независимо от размера таблицы.
-- Ключ — id, чтобы ORDER BY id LIMIT 100 читался из индекса без сортировки.
-- CONCURRENTLY — чтобы не блокировать запись в outbox на проде.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_outbox_unpublished
    ON outbox (id) WHERE published_at IS NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_outbox_unpublished;
