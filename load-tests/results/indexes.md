# DB-03 — Индексы с замерами

## Таблица замеров

| Запрос | До | После | Индекс |
|--------|-----|-------|--------|
| order_items WHERE order_id = N | 382ms | 0.176ms | idx_order_items_order_id |
| orders ORDER BY created_at DESC LIMIT 50 | 231ms | 1.575ms | idx_orders_created_at |
| orders WHERE status='pending' ORDER BY created_at DESC | 24ms | 1.223ms | idx_orders_status_created_at |

## Отвергнутый индекс

`CREATE INDEX ON orders(status)` — отвергнут.

Причина: `status = 'delivered'` возвращает 90% таблицы (4.5M из 5M строк).
Планировщик выбирает Seq Scan — чтение 90% строк через индекс дороже
чем последовательное сканирование из-за случайного I/O при Index Scan.

## Создание индексов на живой базе

`CREATE INDEX CONCURRENTLY` — не блокирует таблицу на запись.
Обычный `CREATE INDEX` берёт `SHARE` блокировку — вставки/обновления ждут.
На продакшене только CONCURRENTLY.
