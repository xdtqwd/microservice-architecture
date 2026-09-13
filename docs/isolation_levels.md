# ORD-04 — Уровни изоляции на живом примере

## Воспроизведение lost update

Открыть две вкладки psql и выполнить:

**Вкладка 1:**
```sql
BEGIN;
SELECT stock FROM products WHERE id = 1; -- видим 10
-- пауза, пока вкладка 2 тоже читает
UPDATE products SET stock = stock - 1 WHERE id = 1; -- пишем 9... но не атомарно если через SELECT
COMMIT;
```

**Вкладка 2 (одновременно):**
```sql
BEGIN;
SELECT stock FROM products WHERE id = 1; -- тоже видим 10
UPDATE products SET stock = 9 WHERE id = 1; -- тоже пишем 9
COMMIT;
```

Результат: stock = 9, хотя продано 2 товара. Потерянное обновление.

## Три способа починки

### 1. Атомарный UPDATE (лучший)
```sql
UPDATE products SET stock = stock - 1 WHERE id = 1 AND stock > 0;
```
Postgres вычисляет новое значение внутри одного оператора.
Конкурентные транзакции выстраиваются в очередь на строку.
Нет extra roundtrip, нет блокировки SELECT.

### 2. SELECT FOR UPDATE
```sql
BEGIN;
SELECT stock FROM products WHERE id = 1 FOR UPDATE; -- блокирует строку
UPDATE products SET stock = stock - 1 WHERE id = 1;
COMMIT;
```
Явная блокировка строки. Вторая транзакция ждёт.
Цена: дополнительный roundtrip + держит блокировку дольше.

### 3. REPEATABLE READ с ретраем
```sql
BEGIN ISOLATION LEVEL REPEATABLE READ;
SELECT stock FROM products WHERE id = 1; -- читаем 10
UPDATE products SET stock = 9 WHERE id = 1;
-- если другая транзакция успела изменить строку — ERROR 40001
-- приложение делает retry
COMMIT;
```
Postgres обнаруживает конфликт и откатывает одну из транзакций.
Цена: нужен retry в приложении, выше вероятность rollback под нагрузкой.

## Сравнение

| Способ | Roundtrips | Блокировка | Retry |
|--------|------------|-----------|-------|
| Атомарный UPDATE | 1 | строка на время UPDATE | нет |
| SELECT FOR UPDATE | 2 | строка до COMMIT | нет |
| REPEATABLE READ | 2 | нет | нужен |

**Атомарный UPDATE лучший** — один roundtrip, минимальное время блокировки,
не нужен retry. Именно так реализован в нашем `CreateOrder`.

## Реальное воспроизведение на живой БД

Начальный stock = 6

    Терминал 1                          Терминал 2
    BEGIN;                              BEGIN;
    SELECT stock FROM products          SELECT stock FROM products
    WHERE id = 1;  -- видим 6          WHERE id = 1;  -- видим 6
    UPDATE products SET stock = 5       
    WHERE id = 1;  -- UPDATE 1          UPDATE products SET stock = 5
                                        WHERE id = 1;  -- ждёт...
    COMMIT;                             -- разблокировался, UPDATE 1
                                        COMMIT;

    SELECT stock FROM products WHERE id = 1;  -- результат: 5

Ожидалось 4 (продано 2 товара), получили 5 (списан 1).

## Почему блокировка не спасла

Postgres заблокировал строку — терминал 2 ждал. Но после разблокировки
терминал 2 не перечитал значение — он записал 5 поверх уже обновлённого 5.

Атомарный UPDATE решает это — Postgres вычисляет новое значение внутри
одного оператора, после разблокировки читает актуальное (5) и пишет 4:

    UPDATE products SET stock = stock - 1 WHERE id = 1;
