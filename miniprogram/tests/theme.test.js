const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function fixture(saved = 'light') {
  const storage = new Map([['appearance', saved]]);
  const native = [];
  const pages = [];
  let app;
  const context = {
    App: (definition) => { app = definition; },
    getApp: () => app, getCurrentPages: () => pages,
    wx: {
      getStorageSync: (key) => storage.get(key), setStorageSync: (key, value) => storage.set(key, value),
      removeStorageSync: (key) => storage.delete(key),
      setNavigationBarColor: (value) => native.push(value),
      setBackgroundColor: (value) => native.push(value),
    }, module: { exports: {} },
  };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../app.js'), 'utf8'), context);
  app.onLaunch();
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../utils/theme.js'), 'utf8'), context);
  const { themedPage } = context.module.exports;
  function page(definition = {}) {
    const result = themedPage(definition);
    result.setData = (patch) => Object.assign(result.data, patch);
    pages.push(result);
    return result;
  }
  return { app, storage, native, page };
}

test('saved appearance is restored before page load and applied to native navigation', async () => {
  const f = fixture('dark');
  const page = f.page({ data: { query: 'AAPL' }, async onLoad(options) { return options.code; } });
  assert.equal(await page.onLoad({ code: 'AAPL' }), 'AAPL');
  assert.equal(page.data.theme, 'dark');
  assert.equal(page.data.query, 'AAPL');
  assert.equal(f.native[0].backgroundColor, '#000000');
  assert.equal(f.native[0].frontColor, '#ffffff');
});

test('switching appearance updates cached pages, persists, and survives returning to a page', () => {
  const f = fixture();
  const home = f.page(); const research = f.page();
  home.onShow(); research.onLoad();
  home.toggleTheme();
  assert.equal(f.storage.get('appearance'), 'dark');
  assert.equal(research.data.theme, 'dark');
  research.onShow();
  assert.equal(research.data.theme, 'dark');
  research.toggleTheme();
  assert.equal(home.data.theme, 'light');
  assert.equal(f.native.at(-2).backgroundColor, '#f2f2f7');
});
