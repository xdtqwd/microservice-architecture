#!/usr/bin/env bash
API=http://localhost:8083
q() { docker compose exec -T postgres psql -U postgres -d orders_db -tAc "$1"; }
up() { env PROVIDER_FAIL_RATE=0 PAY_STUCK_AFTER=15s "$@" docker compose up -d --build order-service >/dev/null 2>&1; sleep 8; }

goose -dir order-service/migrations/goose postgres \
  "postgres://postgres:password@localhost:5436/orders_db?sslmode=disable" up 2>&1 | tail -1
q "UPDATE products SET stock = 1000 WHERE id = 1; UPDATE fake_provider_settings SET down = false" >/dev/null

echo "== падение между провайдером и записью результата"
up PAY_CRASH_AFTER_CHARGE=1
id=$(curl -s -X POST $API/orders -H "Content-Type: application/json" \
  -d '[{"product_id":1,"quantity":1}]' | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
curl -s -o /dev/null -w "  оплата: %{http_code} (сервис упал)\n" -H "Idempotency-Key: stuck-$id" -X POST $API/orders/$id/pay

up   # обычный режим, клиент запрос НЕ повторяет
echo "  после рестарта: заказ $(q "SELECT status FROM orders WHERE id=$id"), платёж $(q "SELECT status FROM payments WHERE order_id=$id")"
echo "  у провайдера: $(q "SELECT count(*) FROM fake_provider_charges WHERE idempotency_key = 'stuck-$id' AND status = 'charged'") списание"

start=$(date +%s)
while [ "$(q "SELECT status FROM orders WHERE id=$id")" != "paid" ]; do
  [ $(( $(date +%s) - start )) -eq 12 ] && echo "  метрика: $(curl -s $API/metrics | grep '^payments_stuck ')"
  [ $(( $(date +%s) - start )) -gt 60 ] && echo "  не разобран за 60 с" && break
  sleep 1
done
echo "  разобран через $(( $(date +%s) - start )) с после рестарта"
echo "  заказ: $(q "SELECT status FROM orders WHERE id=$id")"
echo "  платёж: $(q "SELECT status || ', ' || provider_payment_id || ', попыток сверки ' || reconcile_attempts FROM payments WHERE order_id=$id")"
echo "  история: $(q "SELECT string_agg(coalesce(from_status,'-') || '→' || to_status || ' (' || reason || ')', '; ' ORDER BY e.id) FROM payment_events e JOIN payments p ON p.id = e.payment_id WHERE p.order_id=$id")"
echo "  у провайдера: $(q "SELECT count(*) FROM fake_provider_charges WHERE idempotency_key = 'stuck-$id' AND status = 'charged'") списание"
curl -s $API/metrics | grep -E '^payments_(stuck|reconciled_total)'
