import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';

// Legacy mixed-suite script. Use /scripts/apis/market_*.js for single-interface pressure tests.

const BASE_URL = __ENV.BASE_URL || 'http://localhost:12660';
const EMAIL = __ENV.EMAIL || 'admin@example.com';
const PASSWORD = __ENV.PASSWORD || 'password';
const APP_ID = __ENV.APP_ID || '';
const APP_FILE = __ENV.APP_FILE || '';

export const options = {
  scenarios: {
    market_public: {
      executor: 'constant-vus',
      vus: Number(__ENV.PUBLIC_VUS || 50),
      duration: __ENV.PUBLIC_DURATION || '2m',
      exec: 'publicMarket',
    },
    market_publish: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.PUBLISH_RATE || 2),
      timeUnit: '1s',
      duration: __ENV.PUBLISH_DURATION || '1m',
      preAllocatedVUs: Number(__ENV.PUBLISH_VUS || 10),
      exec: 'publishMarket',
    },
    market_download: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.DOWNLOAD_RATE || 20),
      timeUnit: '1s',
      duration: __ENV.DOWNLOAD_DURATION || '2m',
      preAllocatedVUs: Number(__ENV.DOWNLOAD_VUS || 30),
      exec: 'downloadMarket',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<1000'],
  },
};

const appFile = APP_FILE ? open(APP_FILE, 'b') : null;

function login() {
  const res = http.post(`${BASE_URL}/api/v1/iam/login`, JSON.stringify({ email: EMAIL, password: PASSWORD }), {
    headers: { 'Content-Type': 'application/json', 'X-Trace-ID': `k6-login-${exec.vu.idInTest}` },
  });
  check(res, { 'login status is 200': (r) => r.status === 200 });
  return res.cookies;
}

export function publicMarket() {
  const trace = `k6-public-${exec.vu.idInTest}-${exec.scenario.iterationInTest}`;
  const list = http.get(`${BASE_URL}/api/v1/market/apps?page=1&page_size=20`, { headers: { 'X-Trace-ID': trace } });
  check(list, { 'list status is 200': (r) => r.status === 200 });

  const ranking = http.get(`${BASE_URL}/api/v1/market/rankings?type=daily&limit=20`, { headers: { 'X-Trace-ID': trace } });
  check(ranking, { 'ranking status is 200': (r) => r.status === 200 });
  sleep(1);
}

export function publishMarket() {
  if (!appFile) {
    sleep(1);
    return;
  }
  const cookies = login();
  const idempotencyKey = `k6-${exec.vu.idInTest}-${exec.scenario.iterationInTest}`;
  const data = {
    name: `k6-app-${idempotencyKey}`,
    category: 'k6',
    tags: 'loadtest',
    status: 'published',
    app_file: http.file(appFile, `k6-${idempotencyKey}.zip`, 'application/zip'),
  };
  const res = http.post(`${BASE_URL}/api/v1/market/apps`, data, {
    cookies,
    headers: { 'Idempotency-Key': idempotencyKey, 'X-Trace-ID': `k6-publish-${idempotencyKey}` },
  });
  check(res, {
    'publish accepted': (r) => [200, 409].includes(r.status),
    'publish has request_id on error': (r) => r.status < 400 || Boolean(r.json('request_id')),
  });
  sleep(1);
}

export function downloadMarket() {
  if (!APP_ID) {
    sleep(1);
    return;
  }
  const res = http.get(`${BASE_URL}/api/v1/market/apps/${APP_ID}/download`, {
    redirects: 0,
    headers: { 'X-Trace-ID': `k6-download-${exec.vu.idInTest}-${exec.scenario.iterationInTest}` },
  });
  check(res, {
    'download redirects': (r) => r.status === 307,
    'download exposes checksum': (r) => Boolean(r.headers['X-App-Sha256'] || r.headers['X-App-SHA256']),
  });
  sleep(1);
}
