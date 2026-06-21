import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';

// Legacy mixed-suite script. Use /scripts/apis/*.js for single-interface pressure tests.

const BASE_URL = __ENV.BASE_URL || 'http://localhost:12660';
const EMAIL = __ENV.EMAIL || 'admin@example.com';
const PASSWORD = __ENV.PASSWORD || 'password';
const USERS = parseUsers(__ENV.USERS || '');

export const options = {
  scenarios: {
    login_single_account: {
      executor: 'constant-vus',
      vus: Number(__ENV.LOGIN_SINGLE_VUS || 50),
      duration: __ENV.LOGIN_SINGLE_DURATION || '2m',
      exec: 'loginSingleAccount',
    },
    login_multi_account: {
      executor: 'constant-vus',
      vus: Number(__ENV.LOGIN_MULTI_VUS || 10),
      duration: __ENV.LOGIN_MULTI_DURATION || '2m',
      exec: 'loginMultiAccount',
      startTime: __ENV.LOGIN_MULTI_START || '0s',
    },
    market_public: {
      executor: 'constant-vus',
      vus: Number(__ENV.MARKET_PUBLIC_VUS || 50),
      duration: __ENV.MARKET_PUBLIC_DURATION || '2m',
      exec: 'marketPublic',
      startTime: __ENV.MARKET_PUBLIC_START || '0s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<1000'],
  },
};

export function loginSingleAccount() {
  login(EMAIL, PASSWORD, `k6-login-single-${exec.vu.idInTest}-${exec.scenario.iterationInTest}`);
  sleep(1);
}

export function loginMultiAccount() {
  const user = USERS.length > 0 ? USERS[exec.vu.idInTest % USERS.length] : { email: EMAIL, password: PASSWORD };
  login(user.email, user.password, `k6-login-multi-${exec.vu.idInTest}-${exec.scenario.iterationInTest}`);
  sleep(1);
}

export function marketPublic() {
  const trace = `k6-market-public-${exec.vu.idInTest}-${exec.scenario.iterationInTest}`;
  const list = http.get(`${BASE_URL}/api/v1/market/apps?page=1&page_size=20`, {
    headers: { 'X-Trace-ID': trace },
  });
  check(list, {
    'market list status is 200': (r) => r.status === 200,
    'market list errors include request_id': (r) => r.status < 400 || Boolean(r.json('request_id')),
  });

  const ranking = http.get(`${BASE_URL}/api/v1/market/rankings?type=daily&limit=20`, {
    headers: { 'X-Trace-ID': trace },
  });
  check(ranking, {
    'market ranking status is 200': (r) => r.status === 200,
    'market ranking errors include request_id': (r) => r.status < 400 || Boolean(r.json('request_id')),
  });
  sleep(1);
}

function login(email, password, traceID) {
  const res = http.post(`${BASE_URL}/api/v1/iam/login`, JSON.stringify({ email, password }), {
    headers: { 'Content-Type': 'application/json', 'X-Trace-ID': traceID },
  });
  check(res, {
    'login status is 200': (r) => r.status === 200,
    'login errors include request_id': (r) => r.status < 400 || Boolean(r.json('request_id')),
  });
}

function parseUsers(value) {
  if (!value) {
    return [];
  }
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
    .map((item) => {
      const [email, password] = item.split(':');
      return { email, password };
    })
    .filter((user) => user.email && user.password);
}
