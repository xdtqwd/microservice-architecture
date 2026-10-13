# Kubernetes (локально, kind)

## Запуск

    kind create cluster --name orders
    docker build -t order-service:dev ./order-service
    kind load docker-image order-service:dev --name orders
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
