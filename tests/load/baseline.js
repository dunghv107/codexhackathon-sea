import http from 'k6/http';
import { check } from 'k6';

// shortcut: empty CI database and read-only routes; add seeded business flows
// before treating this as a capacity test for real users.
export const options = {
  scenarios: {
    baseline: {
      executor: 'constant-arrival-rate',
      rate: 5,
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 5,
      maxVUs: 10,
    },
  },
  thresholds: {
    checks: ['rate==1'],
    http_req_failed: ['rate<0.01'],
    'http_req_duration{service:backend}': ['p(95)<500'],
    'http_req_duration{service:kbs}': ['p(95)<500'],
    dropped_iterations: ['count==0'],
  },
};

export default function () {
  const catalog = http.get('http://frontend:8080/api/catalog/categories', {
    tags: { service: 'backend' }, timeout: '5s',
  });
  check(catalog, {
    'catalog returns JSON': (r) => r.status === 200
      && (r.headers['Content-Type'] || '').includes('application/json'),
  });
  const ready = http.get('http://kbs:1114/ready', {
    tags: { service: 'kbs' }, timeout: '5s',
  });
  check(ready, { 'KBS ready': (r) => r.status === 200 });
}
