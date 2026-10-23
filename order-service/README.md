# microservice-architecture — order-service

Учебный проект: микросервисная архитектура на Go. Сервис заказов с PostgreSQL, Redis, Kafka.

## Быстрый старт

    git clone https://github.com/xdtqwd/microservice-architecture
    cd microservice-architecture
    git config core.hooksPath .githooks
    docker compose up -d

Сервис будет доступен на http://localhost:8083.

## Сервисы и порты

| Сервис          | Порт |
|-----------------|------|
| order-service   | 8083 |
| product-service | 8082 |
| PostgreSQL      | 5436 |
| Redis           | 6379 |
| Kafka           | 9092 |

## Переменные окружения

| Переменная   | Значение по умолчанию                                 |
|--------------|-------------------------------------------------------|
| DATABASE_URL | postgres://postgres:password@localhost:5436/orders_db |
| REDIS_ADDR   | localhost:6379                                        |
| PORT         | :8083                                                 |

## Схема БД

- orders — заказы (id, status, created_at)
- order_items — позиции заказа (order_id, product_id, quantity, price)
- products — товары (id, name, price, stock)
- idempotency_keys — дедупликация запросов
- outbox — транзакционный outbox для Kafka
- outbox_dlq — dead letter queue для ядовитых событий

## Эндпоинты

Создать заказ:

    curl -X POST http://localhost:8083/orders \
      -H "Content-Type: application/json" \
      -H "Idempotency-Key: key-1" \
      -d '[{"product_id": 1, "quantity": 2}]'

Список заказов (keyset пагинация):

    curl "http://localhost:8083/orders?limit=10"
    curl "http://localhost:8083/orders?limit=10&after_id=100"

Отменить заказ:

    curl -X POST http://localhost:8083/orders/42/cancel

Список товаров:

    curl http://localhost:8083/products

Метрики Prometheus:

    curl http://localhost:8083/metrics

## Make-команды

    make help         — список всех команд
    make test         — юнит тесты с race detector
    make lint         — golangci-lint
    make docker-up    — поднять все сервисы
    make docker-down  — остановить
    make docker-logs  — логи order-service
    make coverage     — отчёт покрытия
    make load-test    — k6 нагрузочный тест

## Архитектура

    POST /orders
        → handler
        → service (idempotency singleflight)
        → txm.Do:
            → OrderRepo.CreateOrder (списывает stock FOR UPDATE)
            → OutboxRepo.Insert (событие в той же транзакции)
        → OutboxRelay (фоновый воркер) → Kafka
        → product-service consumer (inbox дедупликация)

## Нагрузочные тесты

Обновить stock перед тестом:

    docker exec microservice-architecture-postgres-1 \
      psql -U postgres -d orders_db \
      -c "UPDATE products SET stock = 100000 WHERE id IN (1,2);"

Запустить:

    k6 run load-tests/list_orders.js
    k6 run load-tests/deadlock.js

## Пул соединений с Postgres

| Переменная | По умолчанию | Что это |
|---|---|---|
| `DB_MAX_CONNS` | 10 | максимум соединений в пуле |
| `DB_MIN_CONNS` | 2 | держим открытыми всегда, чтобы первые запросы не ждали подключения |
| `DB_ACQUIRE_TIMEOUT` | 1s | сколько запрос ждёт свободного соединения, после этого 503 |

**Почему 10.** Подобрано замером в OPS-04 (`load-tests/results/pool_tuning.md`):
на 10 соединениях p95 лучше, чем на 25 и 50. Больше соединений не даёт
прироста — Postgres упирается в CPU и блокировки, а не в число подключений.
Ориентир для выделенного сервера БД — примерно число ядер × 2–4, и сумма
пулов всех инстансов сервиса должна оставаться меньше `max_connections`.

**Почему 1s.** Обычный запрос занимает соединение миллисекунды. Если за
секунду соединение не освободилось, пул исчерпан, и честнее сразу ответить
`503` с `Retry-After`, чем держать клиента до общего таймаута запроса (10s).
Таймаут ограничивает только ожидание соединения, сами запросы он не обрывает.

Когда пул исчерпан, растут `db_pool_acquire_timeouts_total` и
`db_pool_acquire_wait_seconds`, а `/readyz` отвечает `503` с ошибкой `db`.
