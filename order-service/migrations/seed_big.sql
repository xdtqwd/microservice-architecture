INSERT INTO products (name, price, stock)
SELECT
    'Product ' || i,
    (random() * 99000 + 1000)::numeric(10,2),
    (random() * 1000)::int
FROM generate_series(1, 10000) AS i

INSERT INTO orders (status, created_at)
SELECT
    CASE
        WHEN r < 0.90 THEN 'delivered'
        WHEN r < 0.95 THEN 'pending'
        ELSE 'cancelled'
    END,
    NOW() - (random() * interval '730 days')
FROM (SELECT random() AS r FROM generate_series(1, 5000000)) AS rnd;

INSERT INTO order_items (order_id, product_id, quantity, price)
SELECT
    o.id,
    (random() * 9999 + 1)::int,
    (random() * 9 + 1)::int,
    (random() * 99000 + 1000)::numeric(10,2)
FROM orders o
CROSS JOIN generate_series(1, 3)
WHERE o.id > (SELECT max(id) - 5000000 FROM orders);

ANALYZE;
