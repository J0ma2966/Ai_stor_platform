// Catalog browsing: main page data, search, filters, product card.
// Target: 5000 VUs plateau (user's expected peak), p95 < 300ms, errors < 0.5%.
// Run: k6 run -e BASE_URL=https://staging.example tests/load/catalog.js
import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { BASE, API } from './lib.js';

const PEAK = Number(__ENV.PEAK_VUS || 5000);

export const options = {
  scenarios: {
    browse: {
      executor: 'ramping-vus',
      stages: [
        { duration: '5m', target: PEAK },
        { duration: '15m', target: PEAK },
        { duration: '2m', target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.005'],
    'http_req_duration{name:products_list}': ['p(95)<300'],
    'http_req_duration{name:product_card}': ['p(95)<300'],
    'http_req_duration{name:search}': ['p(95)<300'],
  },
};

const QUERIES = ['nike', 'kurtka', 'джинсы', 'платье', 'adidas', 'shapka', 'туфли'];
const SORTS = ['popular', 'new', 'price_asc', 'price_desc'];

export default function () {
  let ids = [];
  group('home', () => {
    http.get(`${BASE}${API.categories}`, { tags: { name: 'categories' } });
    const r = http.get(`${BASE}${API.products}?sort=popular&limit=24`, { tags: { name: 'products_list' } });
    check(r, { 'list 200': (x) => x.status === 200 });
    try { ids = (r.json('data') || []).map((p) => p.id); } catch (_) { ids = []; }
  });
  sleep(1 + Math.random() * 2);

  group('search_filter', () => {
    const q = encodeURIComponent(QUERIES[Math.floor(Math.random() * QUERIES.length)]);
    const s = SORTS[Math.floor(Math.random() * SORTS.length)];
    const r = http.get(`${BASE}${API.products}?q=${q}&sort=${s}&in_stock=true&price_max=200000000&limit=24`, { tags: { name: 'search' } });
    check(r, { 'search 200': (x) => x.status === 200 });
  });
  sleep(1 + Math.random() * 2);

  if (ids.length) {
    group('card', () => {
      const id = ids[Math.floor(Math.random() * ids.length)];
      const r = http.get(`${BASE}${API.product(id)}`, { tags: { name: 'product_card' } });
      check(r, { 'card 200': (x) => x.status === 200 });
    });
  }
  sleep(2 + Math.random() * 3);
}
