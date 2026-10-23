# DB-07 — Нагрузочные тесты после блока 3

Все замеры сняты с одинаковым профилем: 100 VU, sleep=0.1s, 30s.

## GET /orders?limit=50 (keyset пагинация)

    checks_total.......: 28607   952/s
    http_req_duration..: avg=4.67ms p(90)=8.1ms p(95)=9.88ms p(99)=13.7ms
    http_req_failed....: 0.00%

## GET /products/{id}

    checks_total.......: 28333   943/s
    http_req_duration..: avg=5.45ms p(90)=9.09ms p(95)=11.64ms p(99)=27.86ms
    http_req_failed....: 0.00%

## POST /orders (10 VU, sleep=0.5s)

    checks_total.......: 590    19.6/s
    http_req_duration..: avg=10.39ms p(90)=14.85ms p(95)=21.07ms p(99)=140.14ms
    http_req_failed....: 0.00%

## Замер "до" — EXPLAIN ANALYZE (не k6)

Честного k6-замера до оптимизаций нет — OFFSET пагинация удалена.
Результаты из EXPLAIN ANALYZE на тех же данных:

| Запрос | До | После | Правка |
|--------|----|-------|--------|
| order_items WHERE order_id=N | 2391ms | 0.091ms | индекс |
| orders ORDER BY created_at LIMIT 50 | 231ms | 2ms | индекс |
| orders OFFSET 4999900 | 5922ms | 0.08ms | keyset |
| GET /orders (N+1) | 51 DB calls | 2 DB calls | ANY($1) |
