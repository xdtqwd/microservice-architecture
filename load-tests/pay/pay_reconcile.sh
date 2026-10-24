#!/usr/bin/env bash
API=http://localhost:8083
q() { docker compose exec -T postgres psql -U postgres -d orders_db -tAc "$1"; }

goose -dir order-service/migrations/goose postgres \
  "postgres://postgres:password@localhost:5436/orders_db?sslmode=disable" up 2>&1 | tail -1
PROVIDER_FAIL_RATE=0 RECONCILE_EVERY=10s docker compose up -d --build order-service >/dev/null 2>&1
sleep 8
q "UPDATE products SET stock = 1000 WHERE id = 1; UPDATE fake_provider_settings SET down = false" >/dev/null

run=$(date +%s)
for x in A B C D; do
  id=$(curl -s -X POST $API/orders -H "Content-Type: application/json" \
    -d '[{"product_id":1,"quantity":1}]' | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
  curl -s -o /dev/null -H "Idempotency-Key: rc-$run-$x" -X POST $API/orders/$id/pay
  eval "order_$x=$id"
done

echo "== портим"
q "DELETE FROM fake_provider_charges WHERE idempotency_key = 'rc-$run-A'" >/dev/null
echo "  A: у нас paid, у провайдера записи нет"
q "UPDATE fake_provider_charges SET amount = amount - 1 WHERE idempotency_key = 'rc-$run-B'" >/dev/null
echo "  B: провайдер списал на 1 меньше"
q "UPDATE payments SET status = 'pending' WHERE idempotency_key = 'rc-$run-C'; UPDATE orders SET status = 'pending' WHERE id = $order_C" >/dev/null
echo "  C: у нас pending, провайдер списал (безопасное)"
q "UPDATE payments SET status = 'failed' WHERE idempotency_key = 'rc-$run-D'" >/dev/null
echo "  D: у нас failed, провайдер списал"
q "INSERT INTO fake_provider_charges (order_id, amount, status, idempotency_key) VALUES (0, 777, 'charged', 'rc-$run-E')" >/dev/null
echo "  E: у провайдера списание, у нас платежа нет"

echo "== ждём сверку"
sleep 12
r=$(q "SELECT max(id) FROM reconciliation_runs")
echo "  запуск $r: $(q "SELECT 'сверено ' || checked || ', совпало ' || matched || ', починено ' || auto_fixed || ', человеку ' || manual FROM reconciliation_runs WHERE id = $r")"
docker compose exec -T postgres psql -U postgres -d orders_db -c "
  SELECT kind, action, idempotency_key AS key, our_status AS ours, our_amount, provider_status AS provider, provider_amount, note
  FROM reconciliation_items WHERE run_id = $r ORDER BY action, kind;"
echo "  C после починки: заказ $(q "SELECT status FROM orders WHERE id = $order_C"), платёж $(q "SELECT status FROM payments WHERE idempotency_key = 'rc-$run-C'")"
curl -s $API/metrics | grep -E '^reconciliation_(discrepancies|auto_fixed_total)'
