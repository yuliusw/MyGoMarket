import http from 'k6/http';
import { check } from 'k6';

export const BASE_URL = __ENV.BASE_URL || 'http://localhost:12660';
export const EMAIL = __ENV.EMAIL || 'admin@example.com';
export const PASSWORD = __ENV.PASSWORD || 'password';
export const AUTH_MODE = __ENV.AUTH_MODE || 'bypass';
export const APP_ID = __ENV.APP_ID || '';
export const APP_FILE = __ENV.APP_FILE || '/scripts/fixtures/app.gz';
export const AVATAR_FILE = __ENV.AVATAR_FILE || '/scripts/fixtures/avatar.png';

const singleApiScenario = __ENV.EXECUTOR === 'constant-vus'
  ? {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS || 100),
      duration: __ENV.DURATION || '1m',
      exec: 'singleApi',
    }
  : {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RATE || 50),
      timeUnit: __ENV.TIME_UNIT || '1s',
      duration: __ENV.DURATION || '1m',
      preAllocatedVUs: Number(__ENV.VUS || 100),
      maxVUs: Number(__ENV.MAX_VUS || 200),
      exec: 'singleApi',
    };

export const options = {
  scenarios: {
    single_api: singleApiScenario,
  },
  thresholds: {
    http_req_failed: [__ENV.FAIL_THRESHOLD || 'rate<0.05'],
    http_req_duration: [__ENV.DURATION_THRESHOLD || 'p(95)<1500'],
    checks: [__ENV.CHECK_THRESHOLD || 'rate>0.95'],
  },
};

export const smokeOptions = {
  scenarios: {
    smoke: {
      executor: 'shared-iterations',
      vus: Number(__ENV.SMOKE_VUS || 1),
      iterations: Number(__ENV.SMOKE_ITERATIONS || 1),
      maxDuration: __ENV.SMOKE_MAX_DURATION || '1m',
      exec: 'smoke',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.10'],
    checks: ['rate>0.90'],
  },
};

export function setupAuth() {
  if (AUTH_MODE === 'bypass') {
    return { cookies: {} };
  }
  const res = jsonPost('/api/v1/iam/login', { email: EMAIL, password: PASSWORD }, [200], 'setup_login');
  return { cookies: res.cookies || {} };
}

export function registerUser(prefix = 'k6') {
  const id = uniqueID(prefix);
  const res = jsonPost('/api/v1/iam/register', {
    username: `${prefix}-${id}`,
    email: `${id}@example.com`,
    password: 'password',
  }, [201, 409], 'iam_register_seed');
  return { userID: safeJSON(res, 'user_id'), email: `${id}@example.com` };
}

export function createGroup(data, prefix = 'k6-group') {
  const name = `${prefix}-${uniqueID('group')}`;
  const res = http.post(`${BASE_URL}/api/v1/iam/groups`, JSON.stringify({ name }), authedJSON(data, 'iam_group_create_seed'));
  checkStatus(res, [201, 409], 'iam_group_create_seed');
  return { groupID: safeJSON(res, 'GroupID'), name };
}

export function createMarketApp(data, appFile) {
  const id = uniqueID('app');
  const res = http.post(`${BASE_URL}/api/v1/market/apps`, {
    name: `k6-app-${id}`,
    category: 'k6',
    tags: ['loadtest'],
    status: 'published',
    app_file: http.file(appFile, `app-${id}.gz`, 'application/gzip'),
  }, authed(data, 'market_app_create_seed', { 'Idempotency-Key': id }));
  checkStatus(res, [200, 201, 409], 'market_app_create_seed');
  return safeJSON(res, 'app_id');
}

export function jsonPost(path, body, statuses, name) {
  const res = http.post(`${BASE_URL}${path}`, JSON.stringify(body), tagged(name, { 'Content-Type': 'application/json' }));
  checkStatus(res, statuses, name);
  return res;
}

export function authed(data, name, extraHeaders = {}) {
  const params = tagged(name, extraHeaders);
  if (AUTH_MODE !== 'bypass' && data && data.cookies) {
    params.cookies = data.cookies;
  }
  return params;
}

export function authedJSON(data, name, extraHeaders = {}) {
  return authed(data, name, Object.assign({ 'Content-Type': 'application/json' }, extraHeaders));
}

export function tagged(name, headers = {}) {
  return { headers: Object.assign({ 'X-Trace-ID': `k6-${name}-${uniqueID('trace')}` }, headers), tags: { name } };
}

export function checkStatus(res, statuses, name) {
  check(res, { [`${name} status`]: (r) => statuses.includes(r.status) });
}

export function safeJSON(res, selector) {
  try {
    return res.json(selector);
  } catch (_) {
    return undefined;
  }
}

export function uniqueID(prefix) {
  const vu = typeof __VU === 'number' && __VU > 0 ? __VU : 'setup';
  const iter = typeof __ITER === 'number' ? __ITER : Date.now();
  return `${prefix}-${Date.now()}-${vu}-${iter}`;
}
