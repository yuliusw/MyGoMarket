import http from 'k6/http';
import { authed, APP_FILE, BASE_URL, checkStatus, options, setupAuth, uniqueID } from '../lib/common.js';

const appFile = open(APP_FILE, 'b');

export { options };
export function setup() { return setupAuth(); }

export function singleApi(data) {
  const id = uniqueID('app');
  const res = http.post(`${BASE_URL}/api/v1/market/apps`, {
    name: `k6-app-${id}`,
    category: 'k6',
    tags: ['loadtest'],
    status: 'published',
    app_file: http.file(appFile, `app-${id}.gz`, 'application/gzip'),
  }, authed(data, 'market_app_publish', { 'Idempotency-Key': id }));
  checkStatus(res, [200, 201, 409], 'market_app_publish');
}
