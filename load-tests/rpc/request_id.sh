#!/usr/bin/env bash
API=http://localhost:8083
q() { docker compose exec -T postgres psql -U postgres -d orders_db -tAc "$1"; }
ID=$(q "SELECT min(id) FROM products WHERE name LIKE 'Bench %'")

docker compose build order-service product-service >/dev/null 2>&1
docker compose up -d product-service >/dev/null 2>&1
env PRODUCT_CATALOG=grpc docker compose up -d order-service >/dev/null 2>&1
sleep 8

RID="demo-$(date +%s)"
echo "== заказ с X-Request-Id: $RID"
curl -s -o /dev/null -w "  ответ: %{http_code}\n" -H "X-Request-Id: $RID" -X POST $API/orders \
  -H "Content-Type: application/json" -d "[{\"product_id\":$ID,\"quantity\":1}]"
sleep 1
echo "  order-service:"
docker compose logs order-service --since 10s | grep "$RID" | sed 's/^/    /' | cut -c1-220
echo "  product-service:"
docker compose logs product-service --since 10s | grep "$RID" | sed 's/^/    /' | cut -c1-220

echo "== заказ без заголовка: id создаёт order-service"
RID2=$(curl -s -D - -o /dev/null -X POST $API/orders -H "Content-Type: application/json" \
  -d "[{\"product_id\":$ID,\"quantity\":1}]" | grep -i '^x-request-id' | tr -d '\r' | awk '{print $2}')
echo "  id из ответа: $RID2"
sleep 1
echo "  записей в order-service: $(docker compose logs order-service --since 10s | grep -c "$RID2")"
echo "  записей в product-service: $(docker compose logs product-service --since 10s | grep -c "$RID2")"

echo "== метрики"
curl -s $API/metrics | grep -E '^rpc_client_requests_total'
curl -s localhost:8082/metrics | grep -E '^rpc_server_requests_total'

docker compose up -d order-service >/dev/null 2>&1
