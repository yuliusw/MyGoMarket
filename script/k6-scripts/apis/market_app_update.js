import http from 'k6/http';
import { authedJSON, APP_FILE, BASE_URL, checkStatus, createMarketApp, options, setupAuth, uniqueID } from '../lib/common.js';

const appFile = open(APP_FILE, 'b');

export { options };
export function setup() {
  const data = setupAuth();
  data.appID = __ENV.APP_ID || createMarketApp(data, appFile);
  return data;
}

export function singleApi(data) {
  const res = http.put(`${BASE_URL}/api/v1/market/apps/${data.appID}`, JSON.stringify({ name: `k6-app-${uniqueID('update')}`, category: 'k6', tags: ['loadtest'], status: 'published' }), authedJSON(data, 'market_app_update'));
  checkStatus(res, [200, 403], 'market_app_update');
}
