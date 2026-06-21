import http from 'k6/http';
import { check, sleep } from 'k6';

// Legacy mixed-suite script. Use /scripts/apis/mq_*.js for independent MQ pressure tests.

const BASE_URL = __ENV.BASE_URL || 'http://localhost:12660';
const EMAIL = __ENV.EMAIL || 'admin@example.com';
const PASSWORD = __ENV.PASSWORD || 'password';

export const options = {
  scenarios: {
    role_purge_broadcast: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.MQ_ROLE_RATE || 2),
      timeUnit: '1s',
      duration: __ENV.MQ_ROLE_DURATION || '20s',
      preAllocatedVUs: Number(__ENV.MQ_ROLE_VUS || 8),
      exec: 'rolePurgeBroadcast',
    },
    group_invalidate_broadcast: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.MQ_GROUP_RATE || 2),
      timeUnit: '1s',
      duration: __ENV.MQ_GROUP_DURATION || '20s',
      preAllocatedVUs: Number(__ENV.MQ_GROUP_VUS || 8),
      exec: 'groupInvalidateBroadcast',
      startTime: __ENV.MQ_GROUP_START || '2s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.10'],
    http_req_duration: ['p(95)<2000'],
    checks: ['rate>0.95'],
  },
};

export function setup() {
  const res = http.post(`${BASE_URL}/api/v1/iam/login`, JSON.stringify({ email: EMAIL, password: PASSWORD }), tagged('setup_login', { 'Content-Type': 'application/json' }));
  check(res, { 'setup login': (r) => r.status === 200 });
  return { cookies: res.cookies || {} };
}

export function rolePurgeBroadcast(data) {
  const role = http.get(`${BASE_URL}/api/v1/iam/roles/4`, authed(data, 'mq_role_get'));
  check(role, { 'role get': (r) => r.status === 200 });
  const permissions = safeJSON(role, 'data.Permissions') || [];
  const permissionIDs = permissions.map((item) => item.ID).filter(Boolean);
  if (permissionIDs.length === 0) {
    return;
  }
  const res = http.put(`${BASE_URL}/api/v1/iam/roles/4/permissions`, JSON.stringify({ permission_ids: permissionIDs }), authed(data, 'mq_role_purge_all'));
  check(res, { 'role purge broadcast': (r) => r.status === 200 });
  sleep(0.2);
}

export function groupInvalidateBroadcast(data) {
  const groupName = `mq-group-${uniqueID()}`;
  const group = http.post(`${BASE_URL}/api/v1/iam/groups`, JSON.stringify({ name: groupName }), authed(data, 'mq_group_create'));
  check(group, { 'group create': (r) => r.status === 201 });
  const groupID = safeJSON(group, 'GroupID');
  if (!groupID) {
    return;
  }
  const target = registerUser();
  if (target.userID) {
    const invite = http.post(`${BASE_URL}/api/v1/iam/groups/${groupID}/members`, JSON.stringify({ user_id: target.userID, role_id: 4 }), authed(data, 'mq_group_invalidate_domain'));
    check(invite, { 'group invalidate broadcast': (r) => r.status === 200 });
  }
  http.del(`${BASE_URL}/api/v1/iam/groups/${groupID}`, null, authed(data, 'mq_group_delete_cleanup'));
  sleep(0.2);
}

function registerUser() {
  const id = uniqueID();
  const res = http.post(`${BASE_URL}/api/v1/iam/register`, JSON.stringify({ username: `mq-${id}`, email: `mq-${id}@example.com`, password: 'password' }), tagged('mq_register_user', { 'Content-Type': 'application/json' }));
  check(res, { 'register target user': (r) => [201, 409].includes(r.status) });
  return { userID: safeJSON(res, 'user_id') };
}

function authed(data, name) {
  const params = tagged(name, { 'Content-Type': 'application/json' });
  params.cookies = data.cookies;
  return params;
}

function tagged(name, headers = {}) {
  return { headers: Object.assign({ 'X-Trace-ID': `k6-${name}-${uniqueID()}` }, headers), tags: { name } };
}

function safeJSON(res, selector) {
  try {
    return res.json(selector);
  } catch (_) {
    return undefined;
  }
}

function uniqueID() {
  const vu = typeof __VU === 'number' && __VU > 0 ? __VU : 'setup';
  const iter = typeof __ITER === 'number' ? __ITER : Date.now();
  return `${Date.now()}-${vu}-${iter}`;
}
