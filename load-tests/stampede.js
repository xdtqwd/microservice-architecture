import http from 'k6/http';
import { check, sleep } from 'k6';
import { config } from './config.js';

export const options = {
    vus: 100,
    duration: '30s',
};

export default function () {
    const res = http.get(`${config.baseUrl}/products/1`);
    check(res, { 'status 200': (r) => r.status === 200 });
    sleep(0.1);
}