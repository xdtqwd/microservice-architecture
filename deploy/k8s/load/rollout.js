// Постоянная нагрузка на Service изнутри кластера, пока идёт выкатка.
// Критерий K8S-05: ни одного неудачного запроса.
import http from 'k6/http';
import { check } from 'k6';

export const options = {
  scenarios: {
    steady: {
      executor: 'constant-arrival-rate',
      rate: 50, timeUnit: '1s', duration: '60s',
      preAllocatedVUs: 20, maxVUs: 100,
    },
  },
  thresholds: { http_req_failed: ['rate==0'] },
};

export default function () {
  const r = http.get('http://order-service:8083/orders?limit=10');
  check(r, { 'status 200': (x) => x.status === 200 });
}
