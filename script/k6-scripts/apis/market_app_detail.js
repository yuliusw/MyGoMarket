import http from 'k6/http';
import { APP_FILE, BASE_URL, checkStatus, createMarketApp, options, setupAuth, tagged } from '../lib/common.js';

const appFile = open(APP_FILE, 'b');

export { options };
export function setup() {
  const data = setupAuth();
  data.appID = __ENV.APP_ID || createMarketApp(data, appFile);
  return data;
}

export function singleApi(data) {
  const res = http.get(`${BASE_URL}/api/v1/market/apps/${data.appID}`, tagged('market_app_detail'));
  checkStatus(res, [200], 'market_app_detail');
}
