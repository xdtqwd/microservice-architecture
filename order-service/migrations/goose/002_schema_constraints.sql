-- +goose Up
ALTER TABLE orders
    DROP CONSTRAINT IF EXISTS orders_status_check,
    ADD CONSTRAINT orders_status_check
        CHECK (status IN ('pending','paid','shipped','delivered','cancelled'));

ALTER TABLE order_items
    DROP CONSTRAINT IF EXISTS order_items_quantity_check,
    ADD CONSTRAINT order_items_quantity_check CHECK (quantity > 0),
    DROP CONSTRAINT IF EXISTS order_items_price_check,
    ADD CONSTRAINT order_items_price_check CHECK (price >= 0);

ALTER TABLE products
    DROP CONSTRAINT IF EXISTS products_stock_check,
    ADD CONSTRAINT products_stock_check CHECK (stock >= 0),
    DROP CONSTRAINT IF EXISTS products_price_check,
    ADD CONSTRAINT products_price_check CHECK (price >= 0);

-- +goose Down
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_check;
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS order_items_quantity_check;
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS order_items_price_check;
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_stock_check;
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_price_check;
