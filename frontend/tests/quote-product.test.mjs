import test from 'node:test';
import assert from 'node:assert/strict';
import { quoteProductIdentity, quoteProductQuery } from '../src/quote-product.ts';

test('crypto spot uses the market endpoints without a stock or futures identity', () => {
  for (const symbol of ['BTCUSDT', 'RUNEUSDT']) {
    const quote = { source:'bitget', provider_symbol:symbol, product_type:'crypto_spot' };
    assert.deepEqual(quoteProductIdentity(quote), {});
    assert.equal(quoteProductQuery(quote), '');
  }
});

test('stock spot and perpetuals retain their exact product category', () => {
  assert.deepEqual(quoteProductIdentity({ source:'bitget', provider_symbol:'RNVDAUSDT', product_type:'stock_token' }), { provider_symbol:'RNVDAUSDT', category:'SPOT' });
  for (const kind of ['quanto_perpetual','stock_perpetual','crypto_perpetual']) {
    const identity = quoteProductIdentity({ source:'bitget', provider_symbol:'TESTUSDT', product_type:kind });
    assert.equal(identity.category, 'USDT-FUTURES');
    assert.equal(identity.provider_symbol, 'TESTUSDT');
  }
  assert.equal(quoteProductQuery({ source:'tencent', provider_symbol:'RNVDAUSDT', product_type:'stock_token' }), '');
  assert.equal(quoteProductQuery({ source:'bitget', provider_symbol:'UNKNOWN' }), '');
});
