import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    thresholds: { http_req_duration: ["p(99)<500"] },
    thresholds: { http_req_duration: ["p(99)<500"] },
    vus: 10,
    duration: '30s',
};

export default function () {
    const res = http.post('http://localhost:8083/orders',
        JSON.stringify([{ product_id: Math.floor(Math.random() * 9999) + 1, quantity: 1 }]),
        { headers: { 'Content-Type': 'application/json' } }
    );
    check(res, { 'status 201': (r) => r.status === 201 });
    sleep(0.5);
}
