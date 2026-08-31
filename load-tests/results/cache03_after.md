# CACHE-03 — с singleflight

**VU:** 100, **Duration:** 30s, **TTL:** 5min

| Метрика | Значение |
|---------|----------|
| RPS | 939 |
| avg | 5.84ms |
| p95 | 10.81ms |
| errors | 0% |
| DB запросов при stampede | 1 |

## Вывод
singleflight пропускает только один запрос в БД при cache miss.
Остальные 99 горутин получают тот же результат без похода в Postgres.
