#!/usr/bin/env bash
API=http://localhost:8083
q() { docker compose exec -T postgres psql -U postgres -d orders_db -tAc "$1"; }

goose -dir order-service/migrations/goose postgres \
  "postgres://postgres:password@localhost:5436/orders_db?sslmode=disable" up 2>&1 | tail -1
PROVIDER_FAIL_RATE=0 docker compose up -d --build order-service >/dev/null 2>&1
sleep 8
q "UPDATE products SET stock = 1000 WHERE id = 1; UPDATE fake_provider_settings SET down = false" >/dev/null

id=$(curl -s -X POST $API/orders -H "Content-Type: application/json" \
  -d '[{"product_id":1,"quantity":3}]' | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
curl -s -o /dev/null -w "оплата: %{http_code}\n" -H "Idempotency-Key: refund-demo-$id" -X POST $API/orders/$id/pay
echo "остаток после оплаты: $(q "SELECT stock FROM products WHERE id = 1")"

echo "== провайдер лёг"
q "UPDATE fake_provider_settings SET down = true" >/dev/null
curl -s -o /dev/null -w "отмена: %{http_code}\n" -X POST $API/orders/$id/cancel
echo "  заказ: $(q "SELECT status FROM orders WHERE id = $id")"
echo "  остаток: $(q "SELECT stock FROM products WHERE id = 1")"
sleep 8
echo "  возврат: $(q "SELECT r.status || ', попыток ' || r.attempts || ', ошибка: ' || coalesce(r.last_error, '-') FROM refunds r JOIN payments p ON p.id = r.payment_id WHERE p.order_id = $id")"

echo "== провайдер поднялся"
q "UPDATE fake_provider_settings SET down = false" >/dev/null
start=$(date +%s)
while true; do
  s=$(q "SELECT r.status FROM refunds r JOIN payments p ON p.id = r.payment_id WHERE p.order_id = $id")
  [ "$s" = "succeeded" ] && break
  [ $(( $(date +%s) - start )) -gt 90 ] && echo "не ушёл за 90 с" && break
  sleep 1
done
echo "  ушёл через $(( $(date +%s) - start )) с"
echo "  возврат: $(q "SELECT r.status || ', попыток ' || r.attempts || ', ' || coalesce(r.provider_refund_id, '-') FROM refunds r JOIN payments p ON p.id = r.payment_id WHERE p.order_id = $id")"
echo "  платёж: $(q "SELECT status FROM payments WHERE order_id = $id")"
echo "  у провайдера возвратов: $(q "SELECT count(*) FROM fake_provider_refunds WHERE idempotency_key LIKE 'refund-%' AND charge_id = (SELECT provider_payment_id FROM payments WHERE order_id = $id)")"
