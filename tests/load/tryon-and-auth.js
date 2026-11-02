// Two scenarios:
//  - tryon: 100 generation starts/s with the AI worker stubbed (AI_PROVIDER=stub on the stand).
//    Enqueue p95 < 500ms; balances must reconcile with the ledger afterwards.
//  - otp_abuse: 20 OTP requests/s for a few phones; limiter must answer 429, server must stay healthy.
import http from 'k6/http';
import { check } from 'k6';
import exec from 'k6/execution';
import { BASE, API, jsonHeaders, login, loadPhone, uuid } from './lib.js';

export const options = {
  scenarios: {
    tryon: {
      executor: 'constant-arrival-rate', exec: 'tryon',
      rate: Number(__ENV.TRYON_RATE || 100), timeUnit: '1s', duration: '5m',
      preAllocatedVUs: 200, maxVUs: 800,
    },
    otp_abuse: {
      executor: 'constant-arrival-rate', exec: 'otpAbuse',
      rate: 20, timeUnit: '1s', duration: '2m', preAllocatedVUs: 20,
    },
  },
  thresholds: {
    'http_req_duration{name:tryon_start}': ['p(95)<500'],
    'checks{check:tryon accepted or no balance}': ['rate>0.995'],
    'checks{check:otp limited not crashed}': ['rate==1'],
  },
};

const tokens = {};

export function tryon() {
  const vu = exec.vu.idInTest;
  if (!tokens[vu]) tokens[vu] = login(loadPhone(100000 + vu));
  if (!tokens[vu]) return;
  const body = JSON.stringify({
    product_ids: (__ENV.TRYON_PRODUCTS || '').split(',').filter(Boolean),
    photo_set_id: __ENV.PHOTO_SET_ID,
  });
  const r = http.post(`${BASE}${API.tryon}`, body, jsonHeaders(tokens[vu], { 'Idempotency-Key': uuid() }), { tags: { name: 'tryon_start' } });
  // 202 queued, 402 insufficient tokens — both are correct behaviour; 5xx is not.
  check(r, { 'tryon accepted or no balance': (x) => x.status === 202 || x.status === 402 });
}

export function otpAbuse() {
  const phone = loadPhone(900000 + (exec.scenario.iterationInTest % 5));
  const r = http.post(`${BASE}${API.otpRequest}`, JSON.stringify({ phone }), jsonHeaders(), { tags: { name: 'otp_request' } });
  check(r, { 'otp limited not crashed': (x) => x.status === 200 || x.status === 202 || x.status === 429 });
}
