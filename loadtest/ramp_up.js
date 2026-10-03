import http from 'k6/http';
import { check, sleep } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

export const options = {
  stages: [
    { duration: '30s', target: 50 },  // Ramp up to 50 concurrent users
    { duration: '1m', target: 200 },  // Ramp up to 200 concurrent users
    { duration: '30s', target: 500 },  // Peak spike to 500 users
    { duration: '30s', target: 0 },    // Ramp down to zero
  ],
  thresholds: {
    http_req_duration: ['p(99)<50'], // Target: p99 latency < 50ms for 202 Accepted
    http_req_failed: ['rate<0.01'],  // Less than 1% errors (ignoring 429 backpressure)
  },
};

const BASE_URL = __ENV.API_URL || 'http://localhost:8080';

export default function () {
  // Use unique idempotency key per payment intent
  const idempotencyKey = uuidv4();
  const fromAccountId = 'c0a80001-0000-0000-0000-000000000001';
  const toAccountId = 'c0a80001-0000-0000-0000-000000000002';

  const payload = JSON.stringify({
    from_account_id: fromAccountId,
    to_account_id: toAccountId,
    amount: 1500, // $15.00 in cents
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': idempotencyKey,
    },
  };

  const res = http.post(`${BASE_URL}/payments`, payload, params);

  // Check expected status: 202 Accepted or 429 Backpressure under heavy load
  check(res, {
    'status is 202 or 429': (r) => r.status === 202 || r.status === 429,
  });

  sleep(0.01);
}
