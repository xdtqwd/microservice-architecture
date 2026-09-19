import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    thresholds: { http_req_duration: ["p(99)<500"] },
    thresholds: { http_req_duration: ["p(99)<500"] },
    vus: 100,
    duration: '30s',
};

export default function () {
    const res = http.get('http://localhost:8083/orders?limit=50');
    check(res, { 'status 200': (r) => r.status === 200 });
    sleep(0.1);
}
