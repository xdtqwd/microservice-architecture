#!/usr/bin/env bash
API=http://localhost:8083
q() { docker compose exec -T postgres psql -U postgres -d orders_db -tAc "$1"; }
order() { curl -s -o /dev/null -w "%{http_code} за %{time_total}s" -X POST $API/orders \
  -H "Content-Type: application/json" -d "[{\"product_id\":$1,\"quantity\":1}]"; }
IDS=$(q "SELECT string_agg(id::text, ' ') FROM products WHERE name LIKE 'Bench %'")
set -- $IDS; HOT=$1; COLD=$2

docker compose up -d --build order-service product-service >/dev/null 2>&1
sleep 8
echo "  прогрев кеша, товар $HOT: $(order $HOT)"

docker compose stop product-service >/dev/null 2>&1
echo "== product-service остановлен"
echo "  товар $HOT, свежий в кеше:   $(order $HOT)"
echo "  товар $COLD, не был в кеше:   $(order $COLD)"
for i in 1 2 3 4 5 6 7; do echo "  товар $COLD, попытка $i:      $(order $COLD)"; done
echo "  ждём, пока кеш устареет (35 с)..."
sleep 35
echo "  товар $HOT, кеш устарел:     $(order $HOT)"
echo "  логи order-service:"
docker compose logs order-service --since 50s | grep -E '"level":"(warn|error)"' | grep -oE '"msg":"[^"]*"|"error":"[^"]{0,120}' | sort | uniq -c | head -6
echo "  метрики:"
curl -s $API/metrics | grep -E '^(rpc_client_requests_total|circuit_breaker_state|catalog_stale_served_total)' | head -6

docker compose start product-service >/dev/null 2>&1
sleep 2
echo "== product-service вернулся"
echo "  товар $COLD сразу:          $(order $COLD)"
sleep 6
echo "  товар $COLD через 6 с:      $(order $COLD)"
echo "  предохранитель: $(curl -s $API/metrics | grep '^circuit_breaker_transitions_total{.*catalog' | tr '\n' ' ')"
