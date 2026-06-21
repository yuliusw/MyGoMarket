import http from 'k6/http';
import { authed, BASE_URL, checkStatus, options, setupAuth } from '../lib/common.js';

export { options };
export function setup() { return setupAuth(); }

export function singleApi(data) {
  const res = http.get(`${BASE_URL}/api/v1/iam/roles`, authed(data, 'iam_roles_list'));
  checkStatus(res, [200, 403], 'iam_roles_list');
}
