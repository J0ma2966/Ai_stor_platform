// Checkout with delivery data at ~50 orders/s; also hammers the same scarce
// product from many VUs to make sure stock never goes negative and no order is duplicated.
// After the run execute scripts/consistency-check.sql against the stand DB.
import http from 'k6/http';
import { check } from 'k6';
import exec from 'k6/execution';
import { BASE, API, jsonHeaders, login, loadPhone, uuid } from './lib.js';

export const options = {
  scenarios: {
    checkout: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RATE || 50),
      timeUnit: '1s',
      duration: __ENV.DURATION || '10m',
      preAllocatedVUs: 200,
      maxVUs: 1000,
    },
  },
  thresholds: {
    'http_req_duration{name:checkout}': ['p(95)<800'],
    'checks{check:checkout ok or out of stock}': ['rate>0.995'],
    'checks{check:idempotent replay same order}': ['rate==1'],
  },
};

const tokens = {};

export default function () {
  const vu = exec.vu.idInTest;
  if (!tokens[vu]) tokens[vu] = login(loadPhone(vu));
  const token = tokens[vu];
  if (!token) return;

  const productId = Math.random() < 0.1 ? __ENV.SCARCE_PRODUCT_ID : __ENV.PRODUCT_ID;
  http.post(`${BASE}${API.cartItems}`, JSON.stringify({ product_id: productId, quantity: 1 }), jsonHeaders(token), { tags: { name: 'cart_add' } });

  const key = uuid();
  const body = JSON.stringify({
    delivery: {
      street: 'Amir Temur', house: '15', entrance: '2', floor: '5', apartment: '42',
      lat: 41.3111, lng: 69.2797,
      comment: 'Домофон не работает, позвонить',
    },
    delivery_time_comment: 'после 18:00',
    comment: 'Load test order',
  });
  const r = http.post(`${BASE}${API.checkout}`, body, jsonHeaders(token, { 'Idempotency-Key': key }), { tags: { name: 'checkout' } });
  check(r, { 'checkout ok or out of stock': (x) => x.status === 201 || x.status === 409 });

  // Replay with the same key must return the same result, never a second order.
  if (r.status === 201 && Math.random() < 0.2) {
    const again = http.post(`${BASE}${API.checkout}`, body, jsonHeaders(token, { 'Idempotency-Key': key }), { tags: { name: 'checkout_replay' } });
    check(again, { 'idempotent replay same order': (x) => x.status === 201 && x.body === r.body });
  }
}
