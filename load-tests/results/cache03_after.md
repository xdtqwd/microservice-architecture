# CACHE-03 — с singleflight

## Как считали DB вызовы

Счётчик `dbCalls int64` в `CachedProductRepo` — атомарный, логируется на Info.
Команды для воспроизведения:
1. `docker exec redis redis-cli FLUSHALL` — сбросить кэш
2. `for i in $(seq 1 100); do curl -s http://localhost:8083/products/1 > /dev/null & done; wait`
3. `docker logs order-service | grep "db call"`

## Результат

| Условие | HTTP запросов | DB calls |
|---------|--------------|----------|
| 100 параллельных, холодный кэш | 100 | 2 |

2 вместо 100 — потому что singleflight дедуплицировал запросы к БД.

## k6 нагрузка (100 VU, 30s, TTL=5min)

| Метрика | Значение |
|---------|----------|
| RPS | 950 |
| p95 | 9.13ms |
| errors | 0% |

## Ограничения замера

- TTL захардкожен (5 min) — stampede при коротком TTL не воспроизведён через k6
- Для замера "до" нужен отдельный стенд без singleflight
- DB calls считаются через логи, не через Prometheus (CACHE-05 добавит метрики)
