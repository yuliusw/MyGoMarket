import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';

// Legacy mixed-suite script. Use run-load-tests.sh with /scripts/apis/*.js for single-interface pressure tests.

const BASE_URL = __ENV.BASE_URL || 'http://localhost:12660';
const EMAIL = __ENV.EMAIL || 'admin@example.com';
const PASSWORD = __ENV.PASSWORD || 'password';
const AUTH_MODE = __ENV.AUTH_MODE || 'bypass';
const APP_FILE = __ENV.APP_FILE || '/scripts/fixtures/app.gz';
const AVATAR_FILE = __ENV.AVATAR_FILE || '/scripts/fixtures/avatar.png';
const SYSTEM_GROUP_ID = __ENV.SYSTEM_GROUP_ID || '11111111-1111-1111-1111-111111111111';

const appFile = open(APP_FILE, 'b');
const avatarFile = open(AVATAR_FILE, 'b');

export const options = {
  scenarios: {
    iam_public: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.IAM_PUBLIC_RATE || 2),
      timeUnit: '1s',
      duration: __ENV.IAM_PUBLIC_DURATION || '20s',
      preAllocatedVUs: Number(__ENV.IAM_PUBLIC_VUS || 8),
      exec: 'iamPublic',
    },
    iam_private: {
      executor: 'constant-vus',
      vus: Number(__ENV.IAM_PRIVATE_VUS || 2),
      duration: __ENV.IAM_PRIVATE_DURATION || '20s',
      exec: 'iamPrivate',
      startTime: __ENV.IAM_PRIVATE_START || '2s',
    },
    iam_group: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.IAM_GROUP_RATE || 1),
      timeUnit: '1s',
      duration: __ENV.IAM_GROUP_DURATION || '20s',
      preAllocatedVUs: Number(__ENV.IAM_GROUP_VUS || 6),
      exec: 'iamGroupLifecycle',
      startTime: __ENV.IAM_GROUP_START || '4s',
    },
    iam_role: {
      executor: 'constant-vus',
      vus: Number(__ENV.IAM_ROLE_VUS || 1),
      duration: __ENV.IAM_ROLE_DURATION || '20s',
      exec: 'iamRoleReadAndBroadcast',
      startTime: __ENV.IAM_ROLE_START || '6s',
    },
  market_public: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.MARKET_PUBLIC_RATE || 10),
      timeUnit: '1s',
      duration: __ENV.MARKET_PUBLIC_DURATION || '20s',
      preAllocatedVUs: Number(__ENV.MARKET_PUBLIC_VUS || 20),
      maxVUs: 200, // 我们刚刚加上的最大 VU
      exec: 'marketPublic', // <--- 就是漏了这一行！告诉 k6 去执行这个函数
      startTime: __ENV.MARKET_PUBLIC_START || '0s',
    },
    market_write: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.MARKET_WRITE_RATE || 1),
      timeUnit: '1s',
      duration: __ENV.MARKET_WRITE_DURATION || '20s',
      preAllocatedVUs: Number(__ENV.MARKET_WRITE_VUS || 8),
      exec: 'marketLifecycle',
      startTime: __ENV.MARKET_WRITE_START || '8s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.10'],
    http_req_duration: ['p(95)<1500'],
    checks: ['rate>0.95'],
  },
};

export function setup() {
  return { cookies: authCookies() };
}

export function iamPublic() {
  const uniq = uniqueID('register');
  const email = `${uniq}@example.com`;
  jsonPost('/api/v1/iam/register', {
    username: `u-${uniq}`,
    email,
    password: 'password',
  }, [201, 409], 'iam_register');
  jsonPost('/api/v1/iam/login', { email: EMAIL, password: PASSWORD }, [200], 'iam_login');
  sleep(0.2);
}

export function iamPrivate(data) {
  const params = authed(data, 'iam_profile_get');
	check(http.get(`${BASE_URL}/api/v1/iam/profile`, params), { 'profile get': (r) => r.status === 200 });
	const username = `admin-${uniqueID('profile')}`;
	const profileUpdate = http.put(`${BASE_URL}/api/v1/iam/profile`, JSON.stringify({ username }), authedJSON(data, 'iam_profile_update'));
  logUnexpected(profileUpdate, [200], 'iam_profile_update');
  check(profileUpdate, {
    'profile update': (r) => r.status === 200,
  });
  const avatarRes = http.post(`${BASE_URL}/api/v1/iam/profile/avatar`, {
    avatar: http.file(avatarFile, `avatar-${uniqueID('avatar')}.png`, 'image/png'),
  }, authed(data, 'iam_profile_avatar'));
  check(avatarRes, { 'avatar upload': (r) => r.status === 200 });
  sleep(0.5);
}

export function iamGroupLifecycle(data) {
  const groupName = `k6-group-${uniqueID('group')}`;
  const groupRes = http.post(`${BASE_URL}/api/v1/iam/groups`, JSON.stringify({ name: groupName }), authedJSON(data, 'iam_group_create'));
  check(groupRes, { 'group create': (r) => r.status === 201 });
  const groupID = safeJSON(groupRes, 'GroupID');
  check(http.get(`${BASE_URL}/api/v1/iam/groups`, authed(data, 'iam_group_list')), { 'group list': (r) => r.status === 200 });
  if (!groupID) {
    return;
  }
  check(http.get(`${BASE_URL}/api/v1/iam/groups/${groupID}`, authed(data, 'iam_group_detail')), { 'group detail': (r) => [200, 403].includes(r.status) });
  check(http.put(`${BASE_URL}/api/v1/iam/groups/${groupID}`, JSON.stringify({ name: `${groupName}-updated` }), authedJSON(data, 'iam_group_update')), { 'group update': (r) => [200, 403].includes(r.status) });
  const target = registerTestUser('member');
  if (target.userID) {
    check(http.post(`${BASE_URL}/api/v1/iam/groups/${groupID}/members`, JSON.stringify({ user_id: target.userID, role_id: 4 }), authedJSON(data, 'iam_group_invite')), { 'group invite': (r) => [200, 403].includes(r.status) });
    check(http.put(`${BASE_URL}/api/v1/iam/groups/${groupID}/members/${target.userID}/role`, JSON.stringify({ role_id: 3 }), authedJSON(data, 'iam_group_member_role')), { 'group member role': (r) => [200, 403].includes(r.status) });
    check(http.del(`${BASE_URL}/api/v1/iam/groups/${groupID}/members/${target.userID}`, null, authed(data, 'iam_group_kick')), { 'group kick': (r) => [200, 403].includes(r.status) });
  }
  check(http.del(`${BASE_URL}/api/v1/iam/groups/${groupID}`, null, authed(data, 'iam_group_delete')), { 'group delete': (r) => [200, 403].includes(r.status) });
  sleep(0.5);
}

export function iamRoleReadAndBroadcast(data) {
  const roles = http.get(`${BASE_URL}/api/v1/iam/roles`, authed(data, 'iam_roles_list'));
  check(roles, { 'roles list': (r) => [200, 403].includes(r.status) });
  check(http.get(`${BASE_URL}/api/v1/iam/roles/4`, authed(data, 'iam_role_get')), { 'role get': (r) => [200, 403, 404].includes(r.status) });
  const permissions = http.get(`${BASE_URL}/api/v1/iam/permissions`, authed(data, 'iam_permissions_list'));
  check(permissions, { 'permissions list': (r) => [200, 403].includes(r.status) });
  const role = safeJSON(http.get(`${BASE_URL}/api/v1/iam/roles/4`, authed(data, 'iam_role_get_for_replace')), 'data') || {};
  const permissionIDs = Array.isArray(role.Permissions) ? role.Permissions.map((item) => item.ID).filter(Boolean) : [];
  if (permissionIDs.length > 0) {
    const res = http.put(`${BASE_URL}/api/v1/iam/roles/4/permissions`, JSON.stringify({ permission_ids: permissionIDs }), authedJSON(data, 'iam_role_replace_permissions'));
    check(res, { 'role permissions replace': (r) => [200, 403].includes(r.status) });
  }
  sleep(1);
}

export function marketPublic() {
  check(http.get(`${BASE_URL}/api/v1/market/apps?page=1&page_size=20`, tagged('market_apps_list')), { 'market list': (r) => r.status === 200 });
  check(http.get(`${BASE_URL}/api/v1/market/apps?page=1&page_size=20&keyword=k6&category=k6&status=published`, tagged('market_apps_search')), { 'market search': (r) => r.status === 200 });
  check(http.get(`${BASE_URL}/api/v1/market/rankings?type=daily&limit=20`, tagged('market_rank_daily')), { 'market rank daily': (r) => r.status === 200 });
  check(http.get(`${BASE_URL}/api/v1/market/rankings?type=weekly&limit=20`, tagged('market_rank_weekly')), { 'market rank weekly': (r) => r.status === 200 });
  check(http.get(`${BASE_URL}/api/v1/market/rankings?type=total&limit=20`, tagged('market_rank_total')), { 'market rank total': (r) => r.status === 200 });
  sleep(0.2);
}

export function marketLifecycle(data) {
  const idempotencyKey = uniqueID('app');
  const createRes = http.post(`${BASE_URL}/api/v1/market/apps`, {
    name: `k6-app-${idempotencyKey}`,
    category: 'k6',
    tags: ['loadtest', 'full-api'],
    status: 'published',
    app_file: http.file(appFile, `app-${idempotencyKey}.gz`, 'application/gzip'),
  }, authed(data, 'market_app_publish', { 'Idempotency-Key': idempotencyKey }));
  logUnexpected(createRes, [200, 201, 409], 'market_app_publish');
  check(createRes, { 'market publish': (r) => [200, 201, 409].includes(r.status) });
  const appID = safeJSON(createRes, 'app_id');
  if (!appID) {
    return;
  }
  check(http.get(`${BASE_URL}/api/v1/market/apps/${appID}`, tagged('market_app_detail')), { 'market detail': (r) => r.status === 200 });
  const dl = http.get(`${BASE_URL}/api/v1/market/apps/${appID}/download`, Object.assign({ redirects: 0 }, tagged('market_app_download')));
  check(dl, { 'market download': (r) => r.status === 307 });
  const updateRes = http.put(`${BASE_URL}/api/v1/market/apps/${appID}`, JSON.stringify({ name: `k6-app-${idempotencyKey}-updated`, category: 'k6', tags: ['loadtest'], status: 'published' }), authedJSON(data, 'market_app_update'));
  logUnexpected(updateRes, [200, 403], 'market_app_update');
  check(updateRes, { 'market update': (r) => [200, 403].includes(r.status) });
  check(http.put(`${BASE_URL}/api/v1/market/apps/${appID}/offshelf`, null, authedJSON(data, 'market_app_offshelf')), { 'market offshelf': (r) => [200, 403].includes(r.status) });
  check(http.del(`${BASE_URL}/api/v1/market/apps/${appID}`, null, authed(data, 'market_app_delete')), { 'market delete': (r) => [200, 403].includes(r.status) });
  sleep(0.5);
}

function registerTestUser(prefix) {
  const uniq = uniqueID(prefix);
  const res = jsonPost('/api/v1/iam/register', { username: `${prefix}-${uniq}`, email: `${uniq}@example.com`, password: 'password' }, [201, 409], `iam_register_${prefix}`);
  return { userID: safeJSON(res, 'user_id') };
}

function authCookies() {
  if (AUTH_MODE === 'bypass') {
    return {};
  }
  const res = jsonPost('/api/v1/iam/login', { email: EMAIL, password: PASSWORD }, [200], 'setup_login');
  return res.cookies || {};
}

function jsonPost(path, body, statuses, name) {
  const res = http.post(`${BASE_URL}${path}`, JSON.stringify(body), tagged(name, { 'Content-Type': 'application/json' }));
  check(res, { [`${name} status`]: (r) => statuses.includes(r.status) });
  return res;
}

function authed(data, name, extraHeaders = {}) {
  const params = tagged(name, extraHeaders);
  if (AUTH_MODE !== 'bypass' && data && data.cookies) {
    params.cookies = data.cookies;
  }
  return params;
}

function authedJSON(data, name, extraHeaders = {}) {
  return authed(data, name, Object.assign({ 'Content-Type': 'application/json' }, extraHeaders));
}

function tagged(name, headers = {}) {
  return { headers: Object.assign({ 'X-Trace-ID': `k6-${name}-${uniqueID('trace')}` }, headers), tags: { name } };
}

function safeJSON(res, selector) {
  try {
    return res.json(selector);
  } catch (_) {
    return undefined;
  }
}

function logUnexpected(res, statuses, name) {
  if (!statuses.includes(res.status)) {
    console.error(`${name} unexpected status=${res.status} body=${truncate(res.body || '', 800)}`);
  }
}

function truncate(value, max) {
  const text = String(value);
  return text.length > max ? `${text.slice(0, max)}...` : text;
}

function uniqueID(prefix) {
  const vu = typeof __VU === 'number' && __VU > 0 ? __VU : 'setup';
  const iter = typeof __ITER === 'number' ? __ITER : Date.now();
  return `${prefix}-${Date.now()}-${vu}-${iter}`;
}
