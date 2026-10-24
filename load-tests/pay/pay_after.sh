#!/usr/bin/env bash
API=http://localhost:8083
q() { docker compose exec -T postgres psql -U postgres -d orders_db -tAc "$1"; }
restart_with() { env "$@" docker compose up -d --build order-service >/dev/null 2>&1; sleep 8; }
new_order() {
  curl -s -X POST $API/orders -H "Content-Type: application/json" \
    -d '[{"product_id":1,"quantity":1}]' | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])"
}
report() {
  echo "  -> заказ $1: $(q "SELECT status FROM orders WHERE id=$1")"
  echo "  -> платежи: $(q "SELECT coalesce(string_agg(status, ', ' ORDER BY id), 'нет') FROM payments WHERE order_id=$1")"
  echo "  -> провайдер: $(q "SELECT count(*) FILTER (WHERE status='charged') || ' списаний на ' || coalesce(sum(amount) FILTER (WHERE status='charged'), 0) FROM fake_provider_charges WHERE order_id=$1")"
}

echo "== миграция 005"
goose -dir order-service/migrations/goose postgres \
  "postgres://postgres:password@localhost:5436/orders_db?sslmode=disable" up 2>&1 | tail -1
q "UPDATE products SET stock = 100000 WHERE id = 1" >/dev/null

echo "== 1. Двойной клик"
restart_with PROVIDER_FAIL_RATE=0
id=$(new_order)
curl -s -o /dev/null -w "  клик A: %{http_code}\n" -X POST $API/orders/$id/pay &
curl -s -o /dev/null -w "  клик B: %{http_code}\n" -X POST $API/orders/$id/pay &
wait
report $id

echo "== 2. Ответ не дошёл, клиент повторил"
id=$(new_order)
curl -s -o /dev/null -w "  попытка 1: %{http_code}\n" --max-time 1 -X POST $API/orders/$id/pay
sleep 3
curl -s -w "  попытка 2: %{http_code}\n" -X POST $API/orders/$id/pay
report $id

echo "== 3. Падение между провайдером и базой"
restart_with PROVIDER_FAIL_RATE=0 PAY_CRASH_AFTER_CHARGE=1
id=$(new_order)
curl -s -o /dev/null -w "  оплата: %{http_code}\n" -X POST $API/orders/$id/pay
restart_with PROVIDER_FAIL_RATE=0
curl -s -w "  повтор после рестарта: %{http_code}\n" -X POST $API/orders/$id/pay
report $id

echo "== 5. Отменённый заказ"
id=$(new_order)
curl -s -o /dev/null -X POST $API/orders/$id/cancel
curl -s -w "  оплата отменённого: %{http_code}\n" -X POST $API/orders/$id/pay
report $id

restart_with
