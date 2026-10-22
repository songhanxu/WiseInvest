import { readFile } from 'node:fs/promises';
import test from 'node:test';
import assert from 'node:assert/strict';
import ts from 'typescript';
const source = await readFile(new URL('../src/account-valuation.ts', import.meta.url), 'utf8');
const { outputText } = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } });
const { accountValuation, holdingNumber, holdingQuantity } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);
const fx = { rates: { USD: 1, CNY: 7, HKD: 7.8 }, asOf: Date.now() };

test('mixed currencies and negative equity are converted before summing; contract notional is excluded', () => {
  const total = accountValuation([
    { holding_type: 'spot', market_value: '100', currency: 'USDT' },
    { holding_type: 'spot', market_value: '78', currency: 'HKD' },
    { holding_type: 'balance', market_value: '-7', currency: 'CNY' },
    { holding_type: 'futures', market_value: '900000', currency: 'USDT' },
  ], fx);
  assert.deepEqual(total, { usdt: 109, cny: 763, missing: 0, futures: 1 });
});
test('exchange-reported total equity replaces its holdings, including unrealized PnL, without double counting', () => {
  const total = accountValuation([
    { account_id: 'bitget', holding_type: 'spot', market_value: '1000', currency: 'USDT' },
    { account_id: 'bitget', holding_type: 'spot', currency: 'USDT' },
    { account_id: 'gate', holding_type: 'spot', market_value: '20', currency: 'USD' },
  ], fx, [{ account_id: 'bitget', amount: '990', currency: 'USDT' }]);
  assert.equal(total.usdt, 1010); assert.equal(total.missing, 0);
});
test('unavailable valuations and FX are not silently counted as zero', () => {
  const rows = [{ holding_type: 'spot', market_value: '78', currency: 'HKD' }, { holding_type: 'spot', market_value: 'NaN', currency: 'USDT' }];
  assert.deepEqual(accountValuation(rows, null), { usdt: null, cny: null, missing: 2, futures: 0 });
  assert.equal(accountValuation([], null).usdt, 0);
  assert.equal(accountValuation([], null, [], false).usdt, null);
  assert.equal(accountValuation([{ holding_type: 'spot', market_value: '0', currency: 'USDT' }], null).usdt, 0);
  assert.equal(holdingNumber('6.70390001'), '6.70');
  assert.equal(holdingNumber('1306.94673429'), '1,306.95');
  assert.equal(holdingNumber('invalid'), '—');
  assert.equal(holdingNumber(null), '—');
  assert.equal(holdingQuantity('0.00370658'), '< 0.01');
  assert.equal(holdingQuantity('-0.0001'), '> -0.01');
});
