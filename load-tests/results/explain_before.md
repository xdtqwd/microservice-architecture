# DB-02 — EXPLAIN (ANALYZE, BUFFERS) до индексов

## 1. GET /products (все товары)

    Seq Scan on products  (cost=0.00..371.55 rows=21355 width=28) (actual time=0.027..21.200 rows=20003 loops=1)
      Buffers: shared hit=158
    Planning Time: 1.021 ms
    Execution Time: 22.352 ms

Seq Scan — таблица маленькая (20k строк), планировщик прав.

## 2. GET /orders LIMIT 50 OFFSET 0

    Limit  (actual time=1.929..1.936 rows=50 loops=1)
      Buffers: shared hit=4
      ->  Index Scan using orders_pkey on orders  (actual time=1.928..1.932 rows=50 loops=1)
            Buffers: shared hit=4
    Execution Time: 1.966 ms

Index Scan по PK — 4 страницы из кэша, быстро.

## 3. GET /orders LIMIT 100 OFFSET 4999900

    Limit  (actual time=5588.881..5588.898 rows=100 loops=1)
      Buffers: shared hit=4 read=45508 written=1318
      ->  Index Scan using orders_pkey on orders  (actual time=0.064..5426.817 rows=5000000 loops=1)
            Buffers: shared hit=4 read=45508 written=1318
    Execution Time: 5922.000 ms

Index Scan читает все 5M строк до нужного offset. 45508 страниц с диска.

## 4. GET /order_items WHERE order_id = N (без индекса)

    Gather  (actual time=2351.483..2361.457 rows=0 loops=1)
      Workers Planned: 2
      Workers Launched: 2
      Buffers: shared read=109133
      ->  Parallel Seq Scan on order_items  (actual time=2328.266..2328.270 rows=0 loops=3)
            Filter: (order_id = 1000000)
            Rows Removed by Filter: 5000393
            Buffers: shared read=109133
    Execution Time: 2391.353 ms

Parallel Seq Scan — 2 воркера читают 15M строк. 109133 страниц с диска.
rows=1 vs actual rows=0 — планировщик ошибся в оценке.

## 5. GET /products/{id}

    Index Scan using products_pkey on products  (actual time=0.712..0.713 rows=1 loops=1)
      Index Cond: (id = 5000)
      Buffers: shared read=3
    Execution Time: 0.775 ms

Index Scan по PK — 3 страницы, быстро.

## Анализ

rows= vs actual rows=: rows=1, actual rows=0 — нет статистики по order_id без индекса.

shared read vs shared hit: read — с диска, hit — из кэша. В запросе 4 все 109133 страниц с диска.

Почему Parallel Seq Scan: без индекса нет другого пути. Postgres запустил 2 воркера чтобы ускорить сканирование 15M строк — это не просто "нет индекса", это физическое ограничение: без структуры данных нельзя прыгнуть к нужным строкам.
