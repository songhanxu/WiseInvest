const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function jwt(exp) {
  const payload = Buffer.from(JSON.stringify({ exp })).toString('base64url');
  return `header.${payload}.signature`;
}

function appFixture(token, refreshedToken) {
  const storage = new Map([['auth_token', token], ['auth_user', { id: 2 }]]);
  const requests = [];
  let app;
  const wx = {
    getStorageSync: (key) => storage.get(key),
    setStorageSync: (key, value) => storage.set(key, value),
    removeStorageSync: (key) => storage.delete(key),
    base64ToArrayBuffer: (value) => Uint8Array.from(Buffer.from(value, 'base64')).buffer,
  };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../app.js'), 'utf8'), {
    App: (definition) => { app = definition; },
    wx, Date, Uint8Array, String, JSON,
    require: () => ({ request: async (options) => {
      requests.push(options);
      return { token: refreshedToken, user: { id: 2 } };
    } }),
  });
  return { app, storage, requests };
}

test('a valid cached login is restored and extended on app use', async () => {
  const renewed = jwt(Math.floor(Date.now() / 1000) + 30 * 86400);
  const { app, storage, requests } = appFixture(jwt(Math.floor(Date.now() / 1000) + 86400), renewed);
  app.onLaunch();
  assert.equal(app.isAuthenticated(), true);
  app.onShow();
  await new Promise(setImmediate);
  assert.equal(requests.length, 1);
  assert.equal(requests[0].url, '/api/v1/auth/refresh');
  assert.equal(storage.get('auth_token'), renewed);
});

test('an expired login is cleared before trial browsing', () => {
  const { app, storage, requests } = appFixture(jwt(Math.floor(Date.now() / 1000) - 1));
  app.onLaunch();
  app.onShow();
  assert.equal(app.isAuthenticated(), false);
  assert.equal(storage.has('auth_token'), false);
  assert.equal(requests.length, 0);
});
