-- Миграция 002: таймзоны и ограничения целостности
-- Идемпотентна: использует IF NOT EXISTS и IF EXISTS

-- 1. TIMESTAMP → TIMESTAMPTZ (только если ещё не TIMESTAMPTZ)
DO $$
BEGIN
    IF (SELECT data_type FROM information_schema.columns
        WHERE table_name='orders' AND column_name='created_at') = 'timestamp without time zone' THEN
        ALTER TABLE orders ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC';
    END IF;
END $$;

-- 2. CHECK ограничения (только на корректных данных)
-- Сначала чистим грязные данные
UPDATE products SET stock = 0 WHERE stock < 0;
UPDATE order_items SET quantity = 1 WHERE quantity <= 0;
UPDATE order_items SET price = 0 WHERE price < 0;
UPDATE products SET price = 0 WHERE price < 0;

ALTER TABLE products DROP CONSTRAINT IF EXISTS check_stock_non_negative;
ALTER TABLE products ADD CONSTRAINT check_stock_non_negative CHECK (stock >= 0);

ALTER TABLE products DROP CONSTRAINT IF EXISTS check_price_non_negative;
ALTER TABLE products ADD CONSTRAINT check_price_non_negative CHECK (price >= 0);

ALTER TABLE order_items DROP CONSTRAINT IF EXISTS check_quantity_positive;
ALTER TABLE order_items ADD CONSTRAINT check_quantity_positive CHECK (quantity > 0);

ALTER TABLE order_items DROP CONSTRAINT IF EXISTS check_item_price_non_negative;
ALTER TABLE order_items ADD CONSTRAINT check_item_price_non_negative CHECK (price >= 0);

-- 3. NOT NULL только на order_id (product_id может быть NULL при ON DELETE SET NULL)
UPDATE order_items SET order_id = 0 WHERE order_id IS NULL;
ALTER TABLE order_items ALTER COLUMN order_id SET NOT NULL;

-- 4. ON DELETE политики
-- orders: RESTRICT — нельзя удалить заказ если есть позиции
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS order_items_order_id_fkey;
ALTER TABLE order_items ADD CONSTRAINT order_items_order_id_fkey
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE RESTRICT;

-- products: SET NULL — товар можно удалить, история заказа остаётся
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS order_items_product_id_fkey;
ALTER TABLE order_items ADD CONSTRAINT order_items_product_id_fkey
    FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE SET NULL;

-- 5. CHECK на статус заказа
ALTER TABLE orders DROP CONSTRAINT IF EXISTS check_order_status;
ALTER TABLE orders ADD CONSTRAINT check_order_status
    CHECK (status IN ('pending', 'paid', 'shipped', 'delivered', 'cancelled'));
