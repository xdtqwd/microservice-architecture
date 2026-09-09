import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    vus: 100,
    duration: '30s',
    thresholds: { http_req_duration: ['p(99)<500'] },
};

export default function () {
    // Имитируем OFFSET деградацию — запрос последней страницы
    const res = http.get('http://localhost:8083/orders?limit=50&offset=4999900');
    check(res, { 'status 200': (r) => r.status === 200 });
    sleep(0.1);
}
