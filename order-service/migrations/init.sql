CREATE TABLE IF NOT EXISTS products (
    id          SERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    price       NUMERIC NOT NULL,
    stock       INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS orders (
    id          SERIAL PRIMARY KEY,
    status      TEXT NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS order_items (
    id          SERIAL PRIMARY KEY,
    order_id    INT REFERENCES orders(id),
    product_id  INT REFERENCES products(id),
    quantity    INT NOT NULL,
    price       NUMERIC NOT NULL
);

-- тестовые данные
INSERT INTO products (name, price, stock) VALUES
    ('MacBook Pro', 150000, 10),
    ('iPhone 15', 80000, 25),
    ('AirPods Pro', 20000, 50);