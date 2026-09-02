# DB-02 — EXPLAIN (ANALYZE, BUFFERS) до индексов

## 1. GET /orders LIMIT 50 OFFSET 0
✅ Быстро — читает только первые 50 строк по PK индексу.

## 2. GET /orders LIMIT 100 OFFSET 4999900
⚠️ Медленно — Index Scan читает все 5М строк до нужного offset.
Проблема: OFFSET заставляет пройти все предыдущие строки.

## 3. GET /order_items WHERE order_id = 1000000
❌ Seq Scan — нет индекса на order_id. Читает 15М строк чтобы найти 3.

## Анализ

**rows= vs actual rows=:**
- `rows=` — оценка планировщика на основе статистики
- `actual rows=` — реальное количество строк
- Расхождение в разы = устаревшая статистика или плохая селективность

**shared read vs shared hit:**
- `shared read` — страница читается с диска (медленно)
- `shared hit` — страница в shared_buffers кэше (быстро)

## Нужны индексы
1. `order_items(order_id)` — устранит Seq Scan
2. Cursor-based pagination — устранит проблему с большим OFFSET
