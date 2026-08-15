import http from 'k6/http';
import { check, sleep } from 'k6';
import { config } from './config.js';

export const options = {
    vus: config.vus,
    duration: config.duration,
};

export default function () {
    let res = http.get(`${config.baseUrl}/products/1`);
    check(res, { 'products status 200': (r) => r.status === 200 });

    res = http.post(`${config.baseUrl}/orders`,
        JSON.stringify([{ "ProductID": 1, "Quantity": 1, "Price": 150000 }]),
        { headers: { 'Content-Type': 'application/json' } }
    );
    check(res, { 'orders status 201': (r) => r.status === 201 });

    sleep(1);
}