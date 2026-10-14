# Kubernetes (локально, kind)

## Запуск

    kind create cluster --name orders
    docker build -t order-service:dev ./order-service
    kind load docker-image order-service:dev --name orders
    # секреты в git не хранятся — создаются командой
    kubectl -n orders create secret generic order-service-secrets \
      --from-literal=POSTGRES_PASSWORD=password \
      --from-literal=DATABASE_URL='postgres://postgres:password@postgres:5432/orders_db?sslmode=disable' \
      --dry-run=client -o yaml | kubectl apply -f -
    kubectl apply -f deploy/k8s/

    # миграции (в K8S-04 станут отдельным Job)
    kubectl -n orders port-forward svc/postgres 15432:5432 &
    goose -dir order-service/migrations/goose postgres \
      "postgres://postgres:password@localhost:15432/orders_db?sslmode=disable" up

    kubectl -n orders port-forward svc/order-service 18083:8083
    curl -X POST localhost:18083/orders -H "Content-Type: application/json" \
      -d '[{"product_id":1,"quantity":1}]'

Postgres, Redis и Kafka здесь только для разработки: Postgres на emptyDir,
данные пропадают вместе с подом.

Публичные образы кластер скачивает сам. `kind load` для них с Docker Desktop
падает на образах с несколькими архитектурами — подкладывать вручную нужно
только order-service:dev, которого нет ни в одном реестре.

## K8S-01: чего Kubernetes не знает без проб

1. **Старт раньше базы.** order-service падает на проверке Postgres при запуске
   (fail fast из OPS-10). Kubernetes сам перезапускает его с растущей паузой
   (CrashLoopBackOff), пока база не поднимется — 3 перезапуска. В compose он
   так и оставался лежать.
2. **«Готов» сразу после запуска процесса.** Каждый раз, едва процесс стартовал,
   под получал 1/1 Running — хотя через секунду падал.
3. **База погашена, под «здоров».** Postgres в 0 реплик: под 1/1 Running и в
   endpoints, трафик идёт; заказ — 500, а /readyz честно отвечает 503
   с причиной. Его просто никто не спрашивает. Чинит K8S-02.
4. **Готовность без схемы.** На пустой базе без таблиц /readyz отвечал 200:
   Ping проходит, а каждый заказ — 500. Тема для K8S-02 и K8S-04: что должна
   проверять готовность и почему миграции нужны до приёма трафика.

## K8S-02: пробы

- liveness — /healthz: процесс жив, без внешних зависимостей
- readiness — /readyz: база отвечает; при остановке сервиса /readyz заранее
  отдаёт 503 (FAIL-06)

Эксперимент: Postgres в 0 реплик на 40 с, затем обратно.

| | liveness на /readyz (ошибка) | liveness /healthz + readiness /readyz |
|---|---|---|
| база погашена | CrashLoopBackOff, перезапуски растут | 0/1 Running, 0 перезапусков |
| трафик на под | шёл бы в падающий контейнер | endpoints ready=false, под вне балансировки |
| база вернулась | ждёт паузу CrashLoopBackOff, до 5 минут | 1/1 за 5 с, без перезапуска |

Почему liveness не должна смотреть на зависимости: перезапуск не чинит
упавшую базу. Он только роняет сервис на старте (fail fast) и добавляет
растущую паузу CrashLoopBackOff — 30 секунд без базы превращаются
в минуты простоя. А если так настроены все сервисы, одна база
перезапускает их все одновременно.

Осталось открытым: /readyz проверяет Ping, но не схему — на пустой базе
под «готов», а заказы падают. Решается в K8S-04 миграциями до приёма трафика.

## K8S-03: конфиг и секреты

- ConfigMap order-service-config — несекретные настройки, в git
- Secret order-service-secrets — пароль и строка подключения, создаётся
  командой, в git не хранится

Secret — это не шифрование: значения лежат в base64,
`kubectl get secret ... | base64 -d` возвращает пароль одной командой.
Защищает он только от случайного чтения: не виден в описании пода,
права на чтение выдаются отдельно через RBAC. По умолчанию etcd хранит
секреты открытым текстом. Настоящая защита — шифрование etcd, RBAC и
внешнее хранилище секретов (Vault и аналоги).

Ограничение переменных окружения: сервис читает их только на старте.
Изменили ConfigMap — нужен `kubectl rollout restart deploy/order-service`.
