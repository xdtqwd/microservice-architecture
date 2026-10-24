#!/usr/bin/env bash
API=http://localhost:8083
q() { docker compose exec -T postgres psql -U postgres -d orders_db -tAc "$1"; }
sleep 3
PID=$(q "INSERT INTO products (name, price, stock) VALUES ('Fix02', 100, 1000) RETURNING id" | head -1)
OID=$(curl -s -X POST $API/orders -H "Content-Type: application/json" \
  -d "[{\"product_id\":$PID,\"quantity\":1}]" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
echo "  товар $PID, заказ $OID"

echo "== отмена, пока строка заказа заблокирована (таймаут контекста)"
docker compose exec -T postgres psql -U postgres -d orders_db -c \
  "BEGIN; SELECT id FROM orders WHERE id = $OID FOR UPDATE; SELECT pg_sleep(13); COMMIT;" </dev/null >/dev/null 2>&1 &
sleep 1
printf "  POST /orders/$OID/cancel → "; curl -s -w "  %{http_code} за %{time_total}s\n" -X POST $API/orders/$OID/cancel
wait

echo "== заказ, пока таблица товаров заблокирована (таймаут на чтении цены)"
docker compose exec -T postgres psql -U postgres -d orders_db -c \
  "BEGIN; LOCK TABLE products IN ACCESS EXCLUSIVE MODE; SELECT pg_sleep(13); COMMIT;" </dev/null >/dev/null 2>&1 &
sleep 1
printf "  POST /orders → "; curl -s -w "  %{http_code} за %{time_total}s\n" -X POST $API/orders \
  -H "Content-Type: application/json" -d "[{\"product_id\":$PID,\"quantity\":1}]"
wait

echo "  ошибок в логе за 40 с: $(docker compose logs order-service --since 40s | grep -c '"level":"error"')"
