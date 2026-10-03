import http from 'k6/http';
import { check, sleep } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

export const options = {
  scenarios: {
    hot_account_contention: {
      executor: 'constant-vus',
      vus: 100,
      duration: '45s',
    },
  },
};

const BASE_URL = __ENV.API_URL || 'http://localhost:8080';

// Fixed hot source account receiving 100 simultaneous debits
const HOT_FROM_ACCOUNT = 'c0a80001-0000-0000-0000-000000000001';
const TO_ACCOUNT = 'c0a80001-0000-0000-0000-000000000002';

export default function () {
  const idempotencyKey = uuidv4();

  const payload = JSON.stringify({
    from_account_id: HOT_FROM_ACCOUNT,
    to_account_id: TO_ACCOUNT,
    amount: 100, // 1 dollar in cents
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': idempotencyKey,
    },
  };

  const res = http.post(`${BASE_URL}/payments`, payload, params);

  check(res, {
    'status is 202 or 429': (r) => r.status === 202 || r.status === 429,
  });

  sleep(0.02);
}
