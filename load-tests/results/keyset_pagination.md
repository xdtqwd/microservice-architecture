# DB-04 — Keyset пагинация vs OFFSET

## Деградация OFFSET

| Страница | OFFSET | Время |
|----------|--------|-------|
| 1 | 0 | 0.8ms |
| 100 | 4950 | 6.9ms |
| 10000 | 499950 | 185ms |
| Последняя | 4999950 | 871ms |

## Keyset пагинация

| Запрос | Время |
|--------|-------|
| WHERE id < cursor LIMIT 50 | 0.081ms |

Время не зависит от глубины страницы — всегда ~0.1ms.

## API

GET /orders?limit=2 → {orders: [...], next_after_id: 10000004}
GET /orders?limit=2&after_id=10000004 → {orders: [...], next_after_id: 10000002}
