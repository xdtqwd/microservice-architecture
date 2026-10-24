package productclient

// ServiceConfig — политика повторов для каталога.
//
// Повторяем только UNAVAILABLE: сосед моргнул или запрос до него не дошёл.
// Не повторяем:
//   - DEADLINE_EXCEEDED — бюджет запроса уже потрачен (RPC-03);
//   - NOT_FOUND, INVALID_ARGUMENT — ответ от повтора не изменится.
//
// Повторять можно только потому, что GetProducts — чтение: вызванный дважды,
// он ничего не меняет. Для вызова, который что-то меняет, такая политика
// без ключа идемпотентности была бы ошибкой.
//
// Повторы укладываются в дедлайн вызова: gRPC не начнёт попытку,
// если на неё не осталось времени.
const ServiceConfig = `{
  "methodConfig": [{
    "name": [{"service": "product.v1.ProductService"}],
    "retryPolicy": {
      "maxAttempts": 3,
      "initialBackoff": "0.02s",
      "maxBackoff": "0.1s",
      "backoffMultiplier": 2,
      "retryableStatusCodes": ["UNAVAILABLE"]
    }
  }]
}`
