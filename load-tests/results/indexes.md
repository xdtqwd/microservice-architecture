# DB-03 — Индексы с замерами и планами EXPLAIN

## 1. idx_order_items_order_id

**До (Seq Scan, без индекса):**

    Gather  (cost=1000.00..188302.71 rows=3 width=24) (actual time=2351.483..2361.457 rows=0 loops=1)
      Workers Planned: 2
      Workers Launched: 2
      Buffers: shared read=109133
      ->  Parallel Seq Scan on order_items  (actual time=2328.266..2328.270 rows=0 loops=3)
            Filter: (order_id = 1000000)
            Rows Removed by Filter: 5000393
            Buffers: shared read=109133
    Execution Time: 2391 ms

**После (Index Scan):**

    Index Scan using idx_order_items_order_id on order_items  (actual time=0.021..0.021 rows=0 loops=1)
      Index Cond: (order_id = 2500000)
      Buffers: shared hit=3
    Execution Time: 0.091 ms

**Результат: 2391ms → 0.091ms**

---

## 2. idx_orders_created_at

**До (Parallel Seq Scan + Sort):**

    Gather Merge  (cost=122888.05..609032.32 rows=4166660 width=21) (actual time=203.142..205.927 rows=50 loops=1)
      Workers Planned: 2
      Buffers: shared hit=72 read=31848
      ->  Sort  (actual time=194.890..194.892 rows=39 loops=3)
            Sort Key: created_at DESC
            ->  Parallel Seq Scan on orders  (actual time=0.044..119.364 rows=1666667 loops=3)
                  Buffers: shared read=31848
    Execution Time: 231 ms

**После (Index Scan):**

    Limit  (actual time=1.923..1.978 rows=50 loops=1)
      Buffers: shared hit=47 read=3
      ->  Index Scan using idx_orders_created_at on orders  (actual time=1.922..1.974 rows=50 loops=1)
            Buffers: shared hit=47 read=3
    Execution Time: 2.005 ms

**Результат: 231ms → 2ms**

---

## 3. idx_orders_status_created_at

**До (Parallel Seq Scan + Sort):**

    Limit  (actual time=1754.504..1756.794 rows=50 loops=1)
      Buffers: shared hit=11250 read=20679
      ->  Gather Merge
            Workers Planned: 2
            ->  Sort  (Sort Key: created_at DESC)
                  ->  Parallel Seq Scan on orders
                        Filter: (status = 'pending')
                        Rows Removed by Filter: 1583630
                        Buffers: shared hit=11176 read=20679
    Execution Time: 1759 ms

**После (Index Scan):**

    Limit  (actual time=4.888..4.907 rows=50 loops=1)
      Buffers: shared hit=47 read=3
      ->  Index Scan using idx_orders_status_created_at on orders  (actual time=4.887..4.903 rows=50 loops=1)
            Index Cond: (status = 'pending')
            Buffers: shared hit=47 read=3
    Execution Time: 4.924 ms

**Результат: 1759ms → 5ms**

---

## Отвергнутый индекс

CREATE INDEX ON orders(status) — отвергнут.

Для status='delivered' (90% строк) планировщик игнорирует индекс — берёт Seq Scan,
потому что читать 90% таблицы через индекс дороже чем последовательно.
Для status='cancelled' (5%) индекс работает: Index Scan, 0.077ms.

Индекс отвергнут не потому что бесполезен вообще, а потому что избыточен:
idx_orders_status_created_at уже содержит status левым префиксом и покрывает те же запросы.
