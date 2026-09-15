# microservice-architecture

Учебный проект: микросервисная архитектура на Go.

## Сервисы

- order-service — приём и обработка заказов (порт 8083)
- product-service — каталог товаров, потребитель Kafka (порт 8082)

## Быстрый старт

    git clone https://github.com/xdtqwd/microservice-architecture
    cd microservice-architecture
    git config core.hooksPath .githooks
    docker compose up -d

Сервисы поднимутся автоматически. order-service доступен на http://localhost:8083.

## Структура репозитория

    order-service/    — основной сервис (Go, PostgreSQL, Redis, Kafka)
    product-service/  — потребитель событий (Go, Kafka)
    load-tests/       — k6 нагрузочные тесты
    .githooks/        — pre-push хук (lint + test)

## Документация

Подробный README с эндпоинтами, схемой БД и make-командами:
order-service/README.md
