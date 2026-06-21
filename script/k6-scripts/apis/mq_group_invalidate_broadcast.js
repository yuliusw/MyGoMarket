import http from 'k6/http';
import { authedJSON, BASE_URL, checkStatus, createGroup, options, registerUser, setupAuth } from '../lib/common.js';

export { options };
export function setup() {
  const data = setupAuth();
  data.groupID = __ENV.GROUP_ID || createGroup(data, 'mq-group').groupID;
  data.userID = __ENV.USER_ID || registerUser('mq').userID;
  return data;
}

export function singleApi(data) {
  const res = http.post(`${BASE_URL}/api/v1/iam/groups/${data.groupID}/members`, JSON.stringify({ user_id: data.userID, role_id: 4 }), authedJSON(data, 'mq_group_invalidate_broadcast'));
  checkStatus(res, [200, 403, 409], 'mq_group_invalidate_broadcast');
}
