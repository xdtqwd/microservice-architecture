#!/usr/bin/env bash
API=http://localhost:8083
q() { docker compose exec -T postgres psql -U postgres -d orders_db -tAc "$1"; }
mode() { env PRODUCT_CATALOG=$1 docker compose up -d order-service >/dev/null 2>&1; sleep 6; }
order() { curl -s -o /dev/null -w "%{http_code} за %{time_total}s" -X POST $API/orders \
  -H "Content-Type: application/json" -d "[{\"product_id\":$1,\"quantity\":1}]"; }

docker compose up -d --build order-service product-service >/dev/null 2>&1
sleep 8
IDS=$(q "SELECT string_agg(id::text, ',') FROM products WHERE name LIKE 'Bench %'")
FIRST=${IDS%%,*}
LAST=${IDS##*,}

echo "== нагрузка: 20 VU, 30 с, заказ из 1–2 товаров"
for m in db grpc grpc-cached; do
  mode $m
  docker compose logs order-service --since 10s | grep -o '"msg":"prices from[^"]*"' | head -1
  printf "%-12s " "$m"
  k6 run -q -e IDS=$IDS load-tests/rpc/create_order.js 2>&1 \
    | grep -E "http_req_duration\.|http_reqs\.|http_req_failed\." \
    | sed -E 's/\.{2,}:/:/' | tr -s ' ' | tr '\n' ' '
  echo
done
echo "  кеш каталога: $(curl -s $API/metrics | grep -E 'cache_(hits|misses)_total\{.*catalog' | tr '\n' ' ')"

echo
echo "== product-service лежит"
mode grpc
docker compose stop product-service >/dev/null 2>&1
echo "  grpc,        товар $FIRST: $(order $FIRST)"
docker compose start product-service >/dev/null 2>&1; sleep 4

mode grpc-cached
echo "  grpc-cached, прогрев $FIRST: $(order $FIRST)"
docker compose stop product-service >/dev/null 2>&1
echo "  grpc-cached, товар $FIRST из кеша:   $(order $FIRST)"
echo "  grpc-cached, товар $LAST не в кеше: $(order $LAST)"
echo "  ждём TTL кеша (30 с)..."
sleep 31
echo "  grpc-cached, товар $FIRST после TTL: $(order $FIRST)"
docker compose start product-service >/dev/null 2>&1; sleep 4

mode db
echo "  db,          товар $FIRST: $(order $FIRST) (соседа не спрашиваем)"

mode grpc-cached
echo "== вернули режим по умолчанию: grpc-cached"
