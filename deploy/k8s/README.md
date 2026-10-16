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
    # топики Kafka и схема базы — Job-ами, как миграции

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

## K8S-04: миграции

Сначала сломано: миграции в init-контейнере каждого пода, 3 реплики разом
на пустую базу. Все три пришли в одну миллисекунду, все увидели, что таблицы
версий goose нет, и все бросились её создавать: один успел, двое упали с
`duplicate key ... goose_db_version already exists`. При постепенном
обновлении Deployment гонку не видно — поды сменяются по очереди, —
она проявляется при масштабировании с нуля или потере узла.

Стало:
- миграции — один Job на выкладку (`30-migrate-job.yaml`, образ
  `order-service/migrate.dockerfile`)
- /readyz проверяет не только Ping, но и версию схемы: сервис знает нужную
  версию из встроенных в бинарник миграций (`migrations.Latest`)
- порядок запуска больше не важен: поды без схемы — 0/1, трафика нет;
  после Job все становятся 1/1 сами

| | Было | Стало |
|---|---|---|
| пустая база | под 1/1, заказы падают с 500 | под 0/1, `schema not migrated` |
| 3 пода разом | гонка, 2 из 3 падают | одна попытка в Job |

Попутно: у Postgres не было пробы готовности, и «готов» наступал раньше,
чем он принимал соединения. Добавлена проба на pg_isready.

Запуск миграций:

    docker build -t order-migrate:dev -f order-service/migrate.dockerfile order-service
    kind load docker-image order-migrate:dev --name orders
    kubectl -n orders delete job order-service-migrate --ignore-not-found
    kubectl apply -f deploy/k8s/30-migrate-job.yaml

## K8S-05: выкатка без потерь

Нагрузка — k6 внутри кластера (`deploy/k8s/load/rollout.js`), 50 rps
на Service, через 15 с `kubectl rollout restart`.

| SHUTDOWN_DRAIN_DELAY | Неудачных запросов |
|---|---|
| 1ms (паузы нет) | 5 из 3000 (0.16%) |
| 5s | 0 |

Почему пауза нужна: при удалении пода Kubernetes одновременно шлёт SIGTERM
и убирает под из списка адресов сервиса. Список расходится по кластеру
не мгновенно — запросы ещё какое-то время летят в закрывающийся под.
Пауза: /readyz уже 503, но порт открыт и запросы обслуживаются.
В kind на одном узле окно — миллисекунды; на многоузловом кластере больше.

terminationGracePeriodSeconds: 20 — пауза 5s + до 12s на текущие запросы.

Находка по пути: SHUTDOWN_DRAIN_DELAY=0s молча превращается в дефолт 5s —
getDuration принимает только значения > 0. Отключить паузу через
окружение нельзя, а с виду настройка применилась. Первый прогон «без
паузы» поэтому дал 0 ошибок.

## K8S-06: три реплики и фоновые воркеры

Воркеры запускаются в каждом поде. Проверено, как каждый живёт на трёх:

| Воркер | Защита | Итог |
|---|---|---|
| релей outbox | FOR UPDATE SKIP LOCKED | реплики делят строки |
| возвраты | FOR UPDATE SKIP LOCKED | реплики делят строки |
| зависшие платежи | FOR UPDATE SKIP LOCKED | реплики делят строки |
| сверка с провайдером | не было | шла в каждой реплике |

Сначала сломано: RECONCILE_EVERY=10s, три реплики — запуски тройками
(:47.49, :48.91, :49.89): каждый под по своему таймеру делал полную
сверку. В обычном режиме это шло каждый час, просто незаметно.

Почему advisory lock на время работы не помогает: запуск длится 2 мс,
поды тикают со сдвигом в секунду — к тику второго первый уже закончил.
Нужна не блокировка на время работы, а право на период.

Стало: таблица job_leases (миграция 010). Реплика на тике пытается
условным UPDATE сдвинуть next_run_at вперёд — удаётся ровно одной.
Время из базы, одни часы на всех. Итог — один запуск на период.

Заодно K8S-04 в деле: новый образ, ждущий схему 10, поднялся в 0/1,
три старых пода продолжали обслуживать запросы, выкатка ждала. После
Job с миграцией дошла до конца сама.

### Outbox на трёх репликах

100 заказов через Service, три релея публикуют параллельно. В Kafka —
68 + 65 + 67 = 200 сообщений по трём партициям, в outbox — 200 событий.
SKIP LOCKED держит и на трёх процессах: дублей нет.

Найдено по пути: в кластере топика orders не было. Писатель Kafka в Go
по умолчанию не просит брокер создать топик, а релей считал
Unknown Topic Or Partition временной ошибкой (FAIL-02) — и бесконечно
ретраил. 200 событий копились, снаружи видно только «backing off» в логах.
В compose топик, видимо, создавал кто-то другой.

Решение — топики создаются явно, как миграции: Job 31-kafka-topics-job.yaml
(--if-not-exists, 3 партиции). После него релеи дослали всё за 15 секунд.
Урок: «временная» ошибка, которая не проходит, — постоянная. Нужен алерт
на возраст самого старого неопубликованного события.

### Автомасштабирование (HPA)

Нужен metrics-server. В kind он работает только без проверки TLS к kubelet:

~~~bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
kubectl -n kube-system patch deploy metrics-server --type=json \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
~~~

Запуск нагрузки и наблюдение:

~~~bash
kubectl apply -f deploy/k8s/25-order-service-hpa.yaml
kubectl -n orders run k6-hpa --rm -i --restart=Never --image=grafana/k6:latest --quiet \
  -- run --quiet - < deploy/k8s/load/hpa.js
kubectl -n orders get hpa order-service -w
~~~

**Результат:** CPU 112% от цели, реплики 3 → 6 за ~20 s. После нагрузки
через 30 s стабилизации обратно к 3. Под при сворачивании выходит сам
(`Completed`) благодаря drain-паузе.

**Что выяснилось по дороге:**

- **Проценты считаются от `requests`, а не от ядра.** При `requests.cpu: 100m`
  под нагрузкой выходило ровно 60m = 60%, и масштабирования не было.
- **У HPA допуск 10%.** Реплики меняются, только если отношение текущий/целевой
  выходит за 0.9–1.1. Нагрузка, стоящая ровно на цели, не масштабируется.
- **Без `-i` у `kubectl run` скрипт k6 не попадает в под.** k6 получает пустой
  stdin, сразу выходит, нагрузки нет.
- **Масштабирование умножает соединения к Postgres.** `DB_MAX_CONNS=10` ×
  6 реплик = 60 из 100 `max_connections`. `maxReplicas` выше ~9 упрётся в базу
  раньше, чем в CPU: новые поды не пройдут readiness или начнут ловить
  таймауты пула. Предел реплик надо считать от базы, а не от нагрузки.

## K8S-07. Отказоустойчивость

### Postgres на постоянном диске

**Как было сломано.** Postgres жил в Deployment с `emptyDir`. После
`kubectl delete pod` новый под стартовал с пустой базой: не было даже
таблицы `orders`. Все реплики `order-service` ушли в `0/1` с
`schema not migrated: goose_db_version is missing`, и без ручного
перезапуска Job миграций сервис не поднимался.

**Как исправлено.** Postgres переведён на StatefulSet с `volumeClaimTemplates`.
У пода постоянное имя `postgres-0` и свой PVC `data-postgres-0`
(в kind его создаёт StorageClass `standard`). PVC переживает удаление пода.
`PGDATA` указывает на подпапку тома: в корне может лежать `lost+found`,
и initdb откажется работать в непустой папке.

**Проверка:**

~~~bash
kubectl -n orders exec postgres-0 -- psql -U postgres -d orders_db \
  -c "create table k8s07_marker(note text)" \
  -c "insert into k8s07_marker values ('до удаления пода')"
kubectl -n orders delete pod postgres-0
kubectl -n orders wait --for=condition=ready pod/postgres-0 --timeout=60s
kubectl -n orders exec postgres-0 -- psql -U postgres -d orders_db -c "select * from k8s07_marker"
~~~

Строка на месте, миграции заново не нужны, `order-service` без рестартов.

**Что выяснилось по дороге:**

- Сразу после рестарта Postgres `/readyz` около секунды отвечает
  `context deadline exceeded`: в пуле остались соединения к старому поду.
  Через 5–10 секунд всё возвращается само.
- Пока схемы нет, воркер возвратов раз в секунду ходит в несуществующую
  таблицу `refunds`. Readiness убирает под из балансировки, но фоновые
  воркеры о ней не знают и продолжают работать.
- Переход Deployment → StatefulSet не делается через `apply`: старый
  Deployment надо удалить руками (`kubectl -n orders delete deploy postgres`).

### PodDisruptionBudget

**Как было сломано.** `kubectl drain` выселил все три пода `order-service`
одновременно. Новые повисли в `Pending` (узел помечен недоступным), и сервис
лежал, пока узел не вернули через `uncordon`. k6: 64 ошибки из 2216.

**Как исправлено.** `21-order-service-pdb.yaml`, `maxUnavailable: 1`:
добровольно (drain, обновление узлов, автоскейлер кластера) выселяется
не больше одного пода за раз. `maxUnavailable`, а не `minAvailable`:
при HPA реплик то 3, то 6, а «не больше одного недоступного» работает
при любом их числе.

**Проверка** (узел один, поэтому выселяем только `order-service`):

~~~bash
NODE=$(kubectl get nodes -o name | head -1 | cut -d/ -f2)
kubectl -n orders run k6-pdb --rm -i --restart=Never --image=grafana/k6:latest --quiet \
  -- run --quiet - < deploy/k8s/load/rollout.js &
sleep 10
kubectl drain $NODE --pod-selector=app=order-service \
  --ignore-daemonsets --delete-emptydir-data --timeout=30s
kubectl uncordon $NODE
~~~

| | Ошибок | rps |
|---|---|---|
| без PDB | 64 из 2216 | 37/s |
| с PDB | 0 из 3001 | 50/s |

С PDB выселен один под, на остальные drain получает
`Cannot evict pod as it would violate the pod's disruption budget`
и через 30 s выходит по таймауту: на единственном узле переселять некуда.
На кластере из нескольких узлов выселенный под поднялся бы на другом,
и drain пошёл бы дальше по одному.

**Что выяснилось по дороге:**

- PDB защищает только от добровольного выселения. Если под упал сам или
  умер узел, PDB ничего не сделает.
- Сразу после создания у PDB `ALLOWED DISRUPTIONS 0`: статус ещё не
  посчитан, и первая попытка выселения отбивается целиком.
- Имя узла зависит от имени кластера kind (`orders-control-plane`),
  поэтому в командах оно берётся через `kubectl get nodes`.
