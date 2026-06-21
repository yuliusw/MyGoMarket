import http from 'k6/http';
import { authed, BASE_URL, checkStatus, createGroup, options, setupAuth } from '../lib/common.js';

export { options };
export function setup() {
  const data = setupAuth();
  data.groupID = __ENV.GROUP_ID || createGroup(data).groupID;
  return data;
}

export function singleApi(data) {
  const res = http.get(`${BASE_URL}/api/v1/iam/groups/${data.groupID}`, authed(data, 'iam_group_detail'));
  checkStatus(res, [200, 403], 'iam_group_detail');
}
