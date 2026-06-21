import http from 'k6/http';
import { jsonPost, BASE_URL, checkStatus, smokeOptions, tagged, uniqueID } from './lib/common.js';

export const options = smokeOptions;

export function smoke() {
  const id = uniqueID('smoke');
  jsonPost('/api/v1/iam/register', { username: `smoke-${id}`, email: `${id}@example.com`, password: 'password' }, [201, 409], 'smoke_iam_register');
  jsonPost('/api/v1/iam/login', { email: __ENV.EMAIL || 'admin@example.com', password: __ENV.PASSWORD || 'password' }, [200], 'smoke_iam_login');
  checkStatus(http.get(`${BASE_URL}/api/v1/market/apps?page=1&page_size=1`, tagged('smoke_market_apps_list')), [200], 'smoke_market_apps_list');
  checkStatus(http.get(`${BASE_URL}/api/v1/market/rankings?type=daily&limit=1`, tagged('smoke_market_rank_daily')), [200], 'smoke_market_rank_daily');
}
