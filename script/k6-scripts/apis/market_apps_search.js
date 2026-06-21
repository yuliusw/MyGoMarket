import http from 'k6/http';
import { BASE_URL, checkStatus, options, tagged } from '../lib/common.js';

export { options };

export function singleApi() {
  const res = http.get(`${BASE_URL}/api/v1/market/apps?page=1&page_size=20&keyword=k6&category=k6&status=published`, tagged('market_apps_search'));
  checkStatus(res, [200], 'market_apps_search');
}
