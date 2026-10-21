import test from 'node:test';
import assert from 'node:assert/strict';
import { mergeQuoteRows, quoteCurrency, quoteSourceLabel, quoteTimeLabel, quoteStatusLabel } from '../src/quote-presentation.ts';

test('quote times retain the exchange clock across daylight saving time', () => {
  assert.equal(quoteTimeLabel({ market: 'us_stock', source: 'tencent', quote_time: '2026-10-20T16:00:01-04:00' }), '美东 10/20 16:00');
  assert.equal(quoteTimeLabel({ market: 'us_stock', source: 'tencent', quote_time: '2026-12-01T16:00:01-05:00' }), '美东 12/01 16:00');
  assert.equal(quoteTimeLabel({ source: 'bitget', quote_time: '1792565189000' }), '北京 10/21 14:46');
  assert.equal(quoteTimeLabel({ quote_time: 'bad' }), '');
  assert.equal(quoteTimeLabel({}), '');
  assert.equal(quoteCurrency({ market: 'hk_stock' }), 'HKD');
  assert.equal(quoteCurrency({ market: 'us_stock', currency: 'USDT' }), 'USDT');
  assert.equal(quoteSourceLabel({ source: 'bitget', product_type: 'stock_token' }), 'Bitget · 24h');
});

test('a failed quote keeps only eligible cached prices without resurrecting removed symbols', () => {
  const cached = [{ id: 'NVDA', market: 'us_stock', source: 'bitget', name: 'NVIDIA', current_price: 220 }, { id: 'MU', market: 'us_stock', source: 'tencent', name: 'Micron', current_price: 896 }, { id: 'OLD', market: 'us_stock', source: 'bitget', name: 'Removed', current_price: 10 }];
  const missing = cached.slice(0, 2).map(row => ({ id: row.id, market: row.market, name: row.name, current_price: 0, quote_available: false }));
  const result = mergeQuoteRows(cached, missing);
  assert.equal(result.length, 2);
  assert.equal(result[0].current_price, 220);
  assert.equal(result[0].source, 'bitget');
  assert.equal(result[0].quote_stale, true);
  assert.equal(result[1].current_price, 896);
  assert.equal(result[1].quote_stale, true);
  const updated = mergeQuoteRows(result, [{ ...cached[0], current_price: 221 }]);
  assert.equal(updated[0].quote_stale, false);
  assert.equal(updated[0].current_price, 221);
});

test('freshness labels distinguish real-time, the actual close date and stale cached quotes', () => {
  const now = 1792565189000;
  assert.equal(quoteStatusLabel({ source:'bitget', quote_time:String(now) }, now), 'Bitget · 实时 14:46');
  assert.equal(quoteStatusLabel({ source:'tencent', market:'us_stock', quote_time:'2026-10-20T16:00:01-04:00', quote_session:'close' }, now), '腾讯 · 10/20 收盘');
  assert.ok(!quoteStatusLabel({ source:'bitget', quote_time:String(now-600000) }, now).includes('实时'));
  assert.ok(quoteStatusLabel({ source:'bitget', quote_time:String(now), quote_stale:true }, now).includes('缓存'));
  assert.equal(quoteStatusLabel({ source:'tencent' }, now), '腾讯 · 报价时间未提供');
});
