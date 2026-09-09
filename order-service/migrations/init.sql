CREATE TABLE IF NOT EXISTS products (
    id          SERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    price       NUMERIC NOT NULL CHECK (price >= 0),
    stock       INT NOT NULL DEFAULT 0 CHECK (stock >= 0)
);

CREATE TABLE IF NOT EXISTS orders (
    id          SERIAL PRIMARY KEY,
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','paid','shipped','delivered','cancelled')),
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS order_items (
    id          SERIAL PRIMARY KEY,
    order_id    INT REFERENCES orders(id),
    product_id  INT REFERENCES products(id),
    quantity    INT NOT NULL CHECK (quantity > 0),
    price       NUMERIC NOT NULL CHECK (price >= 0)
);


CREATE TABLE IF NOT EXISTS idempotency_keys (
    key         TEXT PRIMARY KEY,
    order_id    INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);
