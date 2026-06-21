import http from 'k6/http';
import { authedJSON, BASE_URL, checkStatus, options, safeJSON, setupAuth } from '../lib/common.js';

export { options };
export function setup() { return setupAuth(); }

export function singleApi(data) {
  const roleID = __ENV.ROLE_ID || 4;
  const role = http.get(`${BASE_URL}/api/v1/iam/roles/${roleID}`, authedJSON(data, 'mq_role_get'));
  const permissions = safeJSON(role, 'data.Permissions') || [];
  const permissionIDs = permissions.map((item) => item.ID).filter(Boolean);
  if (permissionIDs.length === 0) {
    checkStatus(role, [200], 'mq_role_get');
    return;
  }
  const res = http.put(`${BASE_URL}/api/v1/iam/roles/${roleID}/permissions`, JSON.stringify({ permission_ids: permissionIDs }), authedJSON(data, 'mq_role_purge_broadcast'));
  checkStatus(res, [200, 403], 'mq_role_purge_broadcast');
}
