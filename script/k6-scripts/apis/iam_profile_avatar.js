import http from 'k6/http';
import { authed, AVATAR_FILE, BASE_URL, checkStatus, options, setupAuth, uniqueID } from '../lib/common.js';

const avatarFile = open(AVATAR_FILE, 'b');

export { options };
export function setup() { return setupAuth(); }

export function singleApi(data) {
  const res = http.post(`${BASE_URL}/api/v1/iam/profile/avatar`, {
    avatar: http.file(avatarFile, `avatar-${uniqueID('avatar')}.png`, 'image/png'),
  }, authed(data, 'iam_profile_avatar'));
  checkStatus(res, [200], 'iam_profile_avatar');
}
