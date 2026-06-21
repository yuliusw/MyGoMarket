import { jsonPost, options, PASSWORD, EMAIL } from '../lib/common.js';

export { options };

export function singleApi() {
  jsonPost('/api/v1/iam/login', { email: EMAIL, password: PASSWORD }, [200], 'iam_login');
}
