import test from 'node:test';
import assert from 'node:assert/strict';
import { formatMarketNumber } from '../src/number-format.ts';

test('small valid prices and normalized volumes never round to a misleading zero', () => {
  assert.equal(formatMarketNumber(0.00002849), '0.0000285');
  assert.equal(formatMarketNumber(0.0000001234), '0.000000123');
  assert.equal(formatMarketNumber(-0.00002849), '-0.0000285');
  assert.equal(formatMarketNumber(220.64), '220.64');
  assert.equal(formatMarketNumber(83140.93), '83,140.93');
  assert.equal(formatMarketNumber(0), '0.00');
  assert.equal(formatMarketNumber(NaN), '—');
});
