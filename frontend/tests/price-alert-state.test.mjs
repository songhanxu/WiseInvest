import { readFile } from 'node:fs/promises';
import test from 'node:test';
import assert from 'node:assert/strict';
import ts from 'typescript';
const source = await readFile(new URL('../src/price-alert-state.ts', import.meta.url), 'utf8');
const { outputText } = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } });
const { PriceAlertState, priceAlertState, findPriceAlert } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);
const row = { id: 1, holding_id: 'watchlist:us_stock:MU', account_id: 'watchlist', target_price: 850, currency: 'USD', revision: 1, status: 'armed' };
const data = { alerts: [row], email: '', mailer_configured: true, check_interval_seconds: 60 };

test('same asset shares the legacy target while retaining its original quote currency', () => {
  assert.equal(findPriceAlert([row], 'bitget:spot:rMU', 'us_stock:MU'), row);
  assert.equal(findPriceAlert([row], 'gate:equity:us:MU', 'us_stock:MU').currency, 'USD');
  assert.equal(findPriceAlert([row], 'bitget:spot:rMU', 'hk_stock:MU'), undefined);
});
test('edits and removals reach both panels immediately; a stale request cannot undo them', async () => {
  const state = new PriceAlertState();
  await state.refresh(async () => data);
  let observations = 0;
  const unsubscribe = state.subscribe(() => observations++);
  let complete;
  const pending = state.refresh(() => new Promise(resolve => { complete = resolve; }));
  const edited = { ...row, asset_key: 'us_stock:MU', holding_id: 'bitget:spot:rMU', account_id: 'bitget', currency: 'USDT', target_price: 900, revision: 2 };
  state.saved(edited);
  assert.equal(findPriceAlert(state.read().data.alerts, row.holding_id, 'us_stock:MU'), edited);
  assert.equal(state.read().data.alerts.length, 1);
  complete(data); await pending;
  assert.equal(state.read().data.alerts[0].target_price, 900);
  state.removed(edited.holding_id);
  assert.equal(state.read().data.alerts.length, 0);
  assert.equal(observations, 2); unsubscribe();
});
test('reads are deduplicated and different users or backends never share state', async () => {
  assert.equal(priceAlertState('a', 'user1'), priceAlertState('a', 'user1'));
  assert.notEqual(priceAlertState('a', 'user1'), priceAlertState('a', 'user2'));
  assert.notEqual(priceAlertState('a', 'user1'), priceAlertState('b', 'user1'));
  const state = new PriceAlertState();
  let complete;
  const pending = state.refresh(() => new Promise(resolve => { complete = resolve; }));
  assert.equal(state.refresh(async () => { throw new Error('duplicate'); }), pending);
  complete(data); await pending;
  await state.refresh(async () => { throw new Error('offline'); });
  assert.equal(state.read().data, data); assert.equal(state.read().error, 'offline');
});
