import http from 'k6/http';
import { authedJSON, BASE_URL, checkStatus, options, setupAuth, uniqueID } from '../lib/common.js';

export { options };
export function setup() { return setupAuth(); }

export function singleApi(data) {
  const res = http.post(`${BASE_URL}/api/v1/iam/groups`, JSON.stringify({ name: `k6-group-${uniqueID('group')}` }), authedJSON(data, 'iam_group_create'));
  checkStatus(res, [201, 409], 'iam_group_create');
}
