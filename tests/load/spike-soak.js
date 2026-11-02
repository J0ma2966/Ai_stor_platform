// Spike (0 -> 10000 VUs in 30s (2x expected peak)) and soak (2000 VUs for 2h) over the catalog flow.
// Select with: k6 run -e MODE=spike|soak tests/load/spike-soak.js
// Watch /metrics during soak: process memory and pgxpool connections must stay flat.
import browse from './catalog.js';

const MODE = __ENV.MODE || 'spike';

const scenarios = {
  spike: {
    executor: 'ramping-vus',
    stages: [
      { duration: '30s', target: Number(__ENV.SPIKE_VUS || 10000) },
      { duration: '3m', target: Number(__ENV.SPIKE_VUS || 10000) },
      { duration: '1m', target: 0 },
    ],
  },
  soak: {
    executor: 'constant-vus', vus: Number(__ENV.SOAK_VUS || 2000), duration: __ENV.SOAK_DURATION || '2h',
  },
};

export const options = {
  scenarios: { [MODE]: scenarios[MODE] },
  thresholds: {
    // During a spike some shedding (429/503) is acceptable; sustained 5xx is not.
    http_req_failed: [MODE === 'spike' ? 'rate<0.05' : 'rate<0.005'],
    'http_req_duration{name:products_list}': [MODE === 'spike' ? 'p(95)<1500' : 'p(95)<300'],
  },
};

export default browse;
