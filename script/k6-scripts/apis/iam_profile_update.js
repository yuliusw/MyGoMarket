import http from 'k6/http';
import { authedJSON, BASE_URL, checkStatus, options, setupAuth, uniqueID } from '../lib/common.js';

export { options };
export function setup() { return setupAuth(); }

export function singleApi(data) {
  const res = http.put(`${BASE_URL}/api/v1/iam/profile`, JSON.stringify({ username: `admin-${uniqueID('profile')}` }), authedJSON(data, 'iam_profile_update'));
  checkStatus(res, [200], 'iam_profile_update');
}
