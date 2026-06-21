import http from 'k6/http';
import { BASE_URL, checkStatus, options, tagged } from '../lib/common.js';

export { options };

export function singleApi() {
  const res = http.get(`${BASE_URL}/api/v1/market/rankings?type=daily&limit=20`, tagged('market_rank_daily'));
  checkStatus(res, [200], 'market_rank_daily');
}
