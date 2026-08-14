import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    vus: 1000,
    duration: '30s',
};
const BASE_URL = 'http://localhost:8083';

export default function () {
    // GET /products/1
    let res = http.get(`${BASE_URL}/products/1`);
    check(res, { 'products status 200': (r) => r.status === 200 });

    // POST /orders
    res = http.post(`${BASE_URL}/orders`,
        JSON.stringify([{ "ProductID": 1, "Quantity": 1, "Price": 150000 }]),
        { headers: { 'Content-Type': 'application/json' } }
    );
    check(res, { 'orders status 201': (r) => r.status === 201 });

    sleep(1);
}