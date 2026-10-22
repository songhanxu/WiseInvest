import { readFile } from 'node:fs/promises';
import test from 'node:test';
import assert from 'node:assert/strict';
import ts from 'typescript';

const source = await readFile(new URL('../src/watchlist-state.ts', import.meta.url), 'utf8');
const { outputText } = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } });
const { WatchlistState, watchlistState, watchlistCode, applyWatchlistChange } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);
const nvda = { id: 'NVDA', symbol: 'NVDA', market: 'us_stock' };
const apple = { id: 'AAPL', symbol: 'AAPL', market: 'us_stock' };

test('market aliases resolve to a single membership without confusing A-share exchanges', () => {
  for (const id of ['btc_price', 'BTC/USDT', 'BTCUSDT', 'BTC']) assert.equal(watchlistCode({ id, symbol: id, market: 'crypto' }), 'BTC');
  assert.equal(watchlistCode({ id: 'USDT', symbol: 'USDT', market: 'crypto' }), 'USDT');
  for (const id of ['HK00700', '700.HK', '00700']) assert.equal(watchlistCode({ id, symbol: id, market: 'hk_stock' }), '00700');
  assert.equal(watchlistCode({ id: '000001', symbol: 'SH000001', market: 'a_share' }), 'sh000001');
  assert.equal(watchlistCode({ id: '000001', symbol: 'SZ000001', market: 'a_share' }), 'sz000001');
  const btc = { id: 'BTC', symbol: 'BTC/USDT', market: 'crypto' };
  assert.equal(applyWatchlistChange([btc], { ...btc, id: 'btc_price' }, false).length, 1);
  assert.equal(applyWatchlistChange([btc], { ...btc, id: 'BTC/USDT' }, true).length, 0);
});

test('membership changes update all subscribers, retain other markets, and unsubscribe cleanly', () => {
  const state = new WatchlistState();
  state.record('us_stock', [nvda], state.version);
  state.record('hk_stock', [], state.version);
  const observed = [];
  const unsubscribe = state.subscribe(change => observed.push(change));
  state.changed(apple, false);
  assert.deepEqual(state.snapshot('us_stock').rows, [nvda, apple]);
  state.changed(nvda, true);
  assert.deepEqual(state.snapshot('us_stock').rows, [apple]);
  assert.deepEqual(state.snapshot('hk_stock').rows, []);
  assert.deepEqual(observed.map(change => change.type), ['mutation', 'mutation']);
  unsubscribe(); state.changed(apple, true);
  assert.equal(observed.length, 2);
});

test('a slow read cannot resurrect a removed stock or erase a newly added stock', async () => {
  const state = new WatchlistState();
  state.record('us_stock', [nvda], state.version);
  let finish;
  const pending = state.load('us_stock', () => new Promise(resolve => { finish = resolve; }));
  assert.equal(state.load('us_stock', () => { throw new Error('duplicate load'); }), pending);
  state.changed(nvda, true); state.changed(apple, false);
  finish([nvda]); await pending;
  assert.deepEqual(state.snapshot('us_stock').rows, [apple]);
});

test('a failed read preserves the last known membership and account scopes are isolated', async () => {
  const state = new WatchlistState();
  state.record('us_stock', [nvda], state.version);
  await assert.rejects(state.load('us_stock', async () => { throw new Error('offline'); }), /offline/);
  assert.deepEqual(state.snapshot('us_stock').rows, [nvda]);
  assert.notEqual(watchlistState('server-a', 'user-one'), watchlistState('server-b', 'user-one'));
  assert.notEqual(watchlistState('server-a', 'user-one'), watchlistState('server-a', 'user-two'));
});
