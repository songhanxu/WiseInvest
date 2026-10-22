import { readFile } from 'node:fs/promises';
import test from 'node:test';
import assert from 'node:assert/strict';
import ts from 'typescript';

const source = await readFile(new URL('../src/session-cache.ts', import.meta.url), 'utf8');
const { outputText } = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } });
const { SessionCache } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);

test('cached detail data retains fetch time and isolates server/product/period keys', () => {
  const cache = new SessionCache(3);
  cache.set('cloud:BTC:spot:1d', [1, 2]);
  const entry = cache.get('cloud:BTC:spot:1d');
  assert.deepEqual(entry.value, [1, 2]);
  assert.ok(entry.updatedAt > 0 && entry.updatedAt <= Date.now());
  assert.equal(cache.get('cloud:BTC:futures:1d'), undefined);
  assert.equal(cache.get('local:BTC:spot:1d'), undefined);
  assert.equal(cache.get('cloud:BTC:spot:1w'), undefined);
  assert.equal(cache.get('cloud:BTC:spot:1d').updatedAt, entry.updatedAt);
});

test('visiting many stocks evicts the least recently viewed data', () => {
  const cache = new SessionCache(2);
  cache.set('a', 1); cache.set('b', 2); cache.get('a'); cache.set('c', 3);
  assert.equal(cache.get('b'), undefined);
  assert.equal(cache.get('a').value, 1);
  cache.set('a', 4);
  assert.equal(cache.get('c').value, 3);
  assert.equal(cache.get('a').value, 4);
});
