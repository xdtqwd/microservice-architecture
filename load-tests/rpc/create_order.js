import http from 'k6/http';
import { check } from 'k6';

// id товаров передаём снаружи: k6 run -e IDS=1,2,3 ...
const ids = (__ENV.IDS || '1').split(',').map(Number);

export const options = {
  vus: 20,
  duration: '30s',
  thresholds: { http_req_failed: ['rate<0.01'] },
};

export default function () {
  const a = ids[Math.floor(Math.random() * ids.length)];
  const b = ids[Math.floor(Math.random() * ids.length)];
  const items = a === b ? [{ product_id: a, quantity: 1 }]
                        : [{ product_id: a, quantity: 1 }, { product_id: b, quantity: 1 }];
  const res = http.post('http://localhost:8083/orders', JSON.stringify(items),
    { headers: { 'Content-Type': 'application/json' } });
  check(res, { created: (r) => r.status === 201 });
}
