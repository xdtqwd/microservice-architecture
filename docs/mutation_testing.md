# TEST-06 · Мутационное тестирование

Инструмент: [gremlins](https://github.com/go-gremlins/gremlins).
Пакеты: `internal/service`, `internal/domain`.

## Итог

| Пакет | До | После |
|---|---|---|
| service | убито 10, выжило 4, не покрыто 3 — efficacy **71.43%** | убито 12, выжило 2, не покрыто 3 — **85.71%** |
| domain | убито 0, не покрыто 1 — **0%** | убито 1 — **100%** |

## Разбор мутантов

| Место | Мутация | Вердикт | Что сделано |
|---|---|---|---|
| `order_service.go:62` | `Quantity <= 0` → `< 0` | **дыра**: проверялось только `-1`, не `0` | `TestCreateOrder_ZeroQuantity_Rejected` |
| `order_service.go:111` | `limit <= 0` → `< 0` | **дыра**: `limit=0` не проверялся | `TestGetOrders_ZeroOrNegativeLimit_UsesDefault` |
| `domain/order.go:55` | `s == to` → `!=` | **дыра**: у `CanTransition` не было тестов | `TestCanTransition`, 13 переходов |
| `order_service.go:114` | `limit > maxLimit` → `>=` | эквивалентная: при `limit == maxLimit` обе ветки дают `maxLimit` | граница задокументирована в `TestGetOrders_LimitCeiling` |
| `order_service.go:97` | `err == nil` → `!=` | не тестируется: `producer` — конкретный `*kafka.Producer`, в тестах `nil` | см. ниже |
| `order_service.go:88, 91, 103` | не покрыто | ветка с `txm`/outbox, в юнит-тестах `txm == nil` | покрыто интеграционными тестами репозитория (TEST-02) |

## Что осталось

Строка 97 — наивная отправка в Kafka из MQ-01, живущая рядом с outbox.
Её нельзя проверить без интерфейса для producer. Если в `app.go` передаётся
настоящий producer, событие уходит дважды: сразу и через outbox relay.
Предлагается отдельная задача: удалить наивную отправку.

## Как воспроизвести

    go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
    cd order-service
    gremlins unleash ./internal/service
    gremlins unleash ./internal/domain
