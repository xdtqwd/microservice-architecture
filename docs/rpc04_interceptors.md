# RPC-04 · request_id, логи и метрики через границу сервисов

## Как едет request_id

    curl -H "X-Request-Id: ..."      (или без заголовка)
    └─ order-service, HTTP RequestID     берёт безопасный входящий id или создаёт uuid,
                                         кладёт в контекст и в заголовок ответа
       └─ gRPC-клиент RequestIDPropagation   из контекста в метаданные x-request-id
          └─ product-service requestID       из метаданных в контекст;
                                             нет или небезопасный — создаёт новый

Безопасный id: `[A-Za-z0-9._-]{1,64}`. Перевод строки отбивает уже HTTP/2,
а кавычки, пробелы и длину проверяем сами — иначе через id можно подделать
поля в текстовых логах.

## Цепочки интерцепторов

Клиент (order-service): RequestIDPropagation → ClientObserver (лог и
метрики) → DeadlineMetrics.

Сервер (product-service), порядок важен:

1. requestID — первым: id нужен всем, включая лог паники;
2. accessLog — видит итоговый код, в том числе Internal после паники;
3. recoverPanic — паника становится Internal, процесс жив;
4. observe — RPC-03: ушёл ли клиент раньше, чем сервер закончил.

## Метрики

`rpc_client_requests_total`, `rpc_client_duration_seconds`,
`rpc_server_requests_total`, `rpc_server_duration_seconds`.
Лейблы — только полное имя метода и gRPC-код: методы из .proto,
кодов 17, число рядов ограничено. Никаких id и текстов ошибок (OPS-13).

## Проверка

`load-tests/rpc/request_id.sh`: один POST /orders даёт записи в обоих
сервисах с одним request_id — и с заголовком от клиента, и без него.
