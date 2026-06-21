import { jsonPost, options, uniqueID } from '../lib/common.js';

export { options };

export function singleApi() {
  const id = uniqueID('register');
  jsonPost('/api/v1/iam/register', {
    username: `u-${id}`,
    email: `${id}@example.com`,
    password: 'password',
  }, [201, 409], 'iam_register');
}
