import { readFile } from 'node:fs/promises';
import test from 'node:test';
import assert from 'node:assert/strict';
import ts from 'typescript';

const source = await readFile(new URL('../src/stock-navigation.ts', import.meta.url), 'utf8');
const { outputText } = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } });
const { enterStock, leaveStock, stockBackLabel } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);

test('stock returns to its actual entry page with the original market and scroll position', () => {
  for (const view of ['home', 'market', 'watchlist', 'accounts', 'chat', 'profile']) {
    const origin = { view, marketId: 'us_stock', stock: null, scrollY: 650 };
    const trail = enterStock([], origin);
    const back = leaveStock(trail);
    assert.deepEqual(back.origin, origin);
    assert.deepEqual(back.trail, []);
  }
  assert.equal(stockBackLabel({ view: 'home' }), '返回总览');
  assert.equal(stockBackLabel({ view: 'watchlist' }), '返回自选列表');
  assert.equal(stockBackLabel({ view: 'market' }, '美股'), '返回美股板块');
  assert.equal(stockBackLabel({ view: 'market' }, '美 股'), '返回美股板块');
});

test('searching another stock from detail returns through the previous stock to the original page', () => {
  const home = { view: 'home', marketId: 'a_share', stock: null, scrollY: 280 };
  const nvda = { view: 'stock', marketId: 'us_stock', stock: { id: 'NVDA', symbol: 'NVDA' }, scrollY: 450 };
  const trail = enterStock(enterStock([], home), nvda);
  const back = leaveStock(trail);
  assert.deepEqual(back.origin, nvda);
  assert.equal(stockBackLabel(back.origin), '返回 NVDA');
  assert.deepEqual(leaveStock(back.trail).origin, home);
  // A later entry from another page starts a fresh return path.
  assert.deepEqual(enterStock(trail, { ...home, view: 'watchlist' }), [{ ...home, view: 'watchlist' }]);
});
