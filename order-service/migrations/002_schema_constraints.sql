-- Миграция 002: таймзоны и ограничения целостности

-- 1. TIMESTAMP → TIMESTAMPTZ
ALTER TABLE orders ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC';

-- 2. CHECK ограничения
ALTER TABLE products ADD CONSTRAINT check_stock_non_negative CHECK (stock >= 0);
ALTER TABLE products ADD CONSTRAINT check_price_non_negative CHECK (price >= 0);
ALTER TABLE order_items ADD CONSTRAINT check_quantity_positive CHECK (quantity > 0);
ALTER TABLE order_items ADD CONSTRAINT check_item_price_non_negative CHECK (price >= 0);

-- 3. NOT NULL на внешние ключи в order_items
ALTER TABLE order_items ALTER COLUMN order_id SET NOT NULL;
ALTER TABLE order_items ALTER COLUMN product_id SET NOT NULL;

-- 4. ON DELETE политики
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS order_items_order_id_fkey;
ALTER TABLE order_items ADD CONSTRAINT order_items_order_id_fkey
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE RESTRICT;

ALTER TABLE order_items DROP CONSTRAINT IF EXISTS order_items_product_id_fkey;
ALTER TABLE order_items ADD CONSTRAINT order_items_product_id_fkey
    FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE SET NULL;
