#!/usr/bin/env bash
API=http://localhost:8083
q() { docker compose exec -T postgres psql -U postgres -d orders_db -tAc "$1"; }
ID=$(q "SELECT min(id) FROM products WHERE name LIKE 'Bench %'")

docker compose build order-service product-service >/dev/null 2>&1
env PRODUCT_CATALOG=grpc CATALOG_TIMEOUT=1s docker compose up -d order-service >/dev/null 2>&1

run() {
  echo "== $1"
  env CATALOG_SLOW_QUERY=5s CATALOG_IGNORE_CTX=$2 docker compose up -d product-service >/dev/null 2>&1
  sleep 6
  curl -s -o /dev/null -w "  клиент: %{http_code} за %{time_total}s\n" -X POST $API/orders \
    -H "Content-Type: application/json" -d "[{\"product_id\":$ID,\"quantity\":1}]"
  sleep 0.5
  echo "  pg_sleep в базе через 0.5 с после ухода клиента: $(q "SELECT count(*) FROM pg_stat_activity WHERE query LIKE 'SELECT pg_sleep%'") шт."
  sleep 5
  echo "  сервер: $(docker compose logs product-service --since 12s | grep -o 'GetProducts: client gone.*' | tail -1)"
  echo "  метрика сервера: $(curl -s localhost:8082/metrics | grep '^rpc_server_client_gone_total' | tr '\n' ' ')"
}

run "сервер глухой к отмене" 1
run "сервер слушает ctx" 0

echo "== метрика клиента"
curl -s $API/metrics | grep -E '^rpc_client_(deadline|skipped)_total'

env CATALOG_SLOW_QUERY=0s CATALOG_IGNORE_CTX=0 docker compose up -d product-service >/dev/null 2>&1
docker compose up -d order-service >/dev/null 2>&1
echo "== вернули обычный режим"
