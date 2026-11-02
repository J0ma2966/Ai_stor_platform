// Shared helpers for k6 load tests. Run against a staging stand, never production.
// Paths live here so they can be adjusted in one place when the API changes.
import http from 'k6/http';

export const BASE = __ENV.BASE_URL || 'http://localhost:8080';

export const API = {
  products: '/v1/products',
  product: (id) => `/v1/products/${id}`,
  categories: '/v1/categories',
  stores: '/v1/stores',
  otpRequest: '/v1/auth/otp/request',
  otpVerify: '/v1/auth/otp/verify',
  cart: '/v1/cart',
  cartItems: '/v1/cart/items',
  checkout: '/v1/orders/checkout',
  tryon: '/v1/tryon/generations',
};

export function uuid() {
  // RFC4122 v4, good enough for idempotency keys in tests.
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16);
  });
}

export function jsonHeaders(token, extra = {}) {
  const h = { 'Content-Type': 'application/json', ...extra };
  if (token) h.Authorization = `Bearer ${token}`;
  return { headers: h };
}

// Load-test customers are seeded on the STAND only, and their access tokens are minted
// there with the stand's JWT secret into tokens.json (["<jwt>", ...]). No OTP backdoor:
// the production code path stays untouched. Pass with -e TOKENS_FILE=./tokens.json.
const TOKENS = __ENV.TOKENS_FILE ? JSON.parse(open(__ENV.TOKENS_FILE)) : [];

// login(i) returns a pre-minted token for test customer i (wraps around).
export function login(i) {
  if (!TOKENS.length) return null;
  return TOKENS[Number(String(i).replace(/\D/g, '')) % TOKENS.length];
}

export function loadPhone(i) {
  // +998 90 0000000 .. — seeded range of test customers.
  return `+99890${String(i).padStart(7, '0')}`;
}
