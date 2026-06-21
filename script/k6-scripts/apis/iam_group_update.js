import http from 'k6/http';
import { authedJSON, BASE_URL, checkStatus, createGroup, options, setupAuth, uniqueID } from '../lib/common.js';

export { options };
export function setup() {
  const data = setupAuth();
  data.groupID = __ENV.GROUP_ID || createGroup(data).groupID;
  return data;
}

export function singleApi(data) {
  const res = http.put(`${BASE_URL}/api/v1/iam/groups/${data.groupID}`, JSON.stringify({ name: `k6-group-updated-${uniqueID('group')}` }), authedJSON(data, 'iam_group_update'));
  checkStatus(res, [200, 403], 'iam_group_update');
}
