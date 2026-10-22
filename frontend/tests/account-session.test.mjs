import test from 'node:test';
import assert from 'node:assert/strict';
import { accountUserId, accountHistoryKey, readAccountHistory } from '../src/account-session.ts';
const token = (id, suffix = '') => `header.${Buffer.from(JSON.stringify({user_id:id})).toString('base64url')}.signature${suffix}`;
const storage = () => { const map = new Map(); return { getItem:key => map.get(key) ?? null, setItem:(key,value) => map.set(key,value) }; };

test('history belongs to the server and user, survives new sessions, and is hidden on logout', () => {
  assert.equal(accountUserId(token(2)),2);
  assert.equal(accountUserId('invalid'),null);
  assert.equal(accountUserId(token(0)),null);
  assert.equal(accountHistoryKey('server-a',token(2)),accountHistoryKey('server-a/',token(2,'rotated')));
  assert.notEqual(accountHistoryKey('server-a',token(2)),accountHistoryKey('server-a',token(3)));
  assert.notEqual(accountHistoryKey('server-a',token(2)),accountHistoryKey('server-b',token(2)));
  assert.equal(accountHistoryKey('server-a',''),null);
});

test('legacy history migrates to the existing account only and cannot be inherited by registration', () => {
  const store = storage(); store.setItem('wiseinvest.conversations','private-history');
  assert.equal(readAccountHistory(store,'server-a',token(2)),'private-history');
  assert.equal(readAccountHistory(store,'server-a',token(3)),null);
  assert.equal(readAccountHistory(store,'server-b',token(2)),null);
  assert.equal(readAccountHistory(store,'server-a',''),null);
  assert.equal(readAccountHistory(store,'server-a',token(2,'new-login')),'private-history');
});

test('unclaimed history is not assigned to the next newly registered account', () => {
  const store = storage(); store.setItem('wiseinvest.conversations','unclaimed-history');
  readAccountHistory(store,'server-a','');
  assert.equal(readAccountHistory(store,'server-a',token(3)),null);
});
