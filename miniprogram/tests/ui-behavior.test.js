const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const markets = require('../utils/market');
const root = path.join(__dirname, '..');

function fixture(file, { records = [], authenticated = true, api = {} } = {}) {
  let definition;
  let conversations = structuredClone(records);
  const events = [];
  const app = { isAuthenticated: () => authenticated, clearAuth() { events.push('logout'); } };
  const wx = {
    getWindowInfo: () => ({ windowHeight: 724, statusBarHeight: 47 }),
    reLaunch(options) { events.push(options); },
    navigateTo(options) { events.push(options); },
    showModal(options) { options.success({ confirm: true }); },
    showActionSheet(options) { events.push(options); },
    setClipboardData(options) { events.push(options); },
    showToast(options) { events.push(options); },
  };
  const storage = {
    loadConversations: () => conversations,
    removeConversationById(id) { conversations = conversations.filter(item => item.id !== id); },
    upsertConversation(item) { conversations = [item, ...conversations.filter(row => row.id !== item.id)]; },
  };
  vm.runInNewContext(fs.readFileSync(path.join(root, file), 'utf8'), {
    Page(config) { definition = config; }, Component(config) { definition = config; },
    getApp: () => app, getCurrentPages: () => [], wx, Date, Math,
    require(module) {
      if (module.endsWith('/market')) return markets;
      if (module.endsWith('/storage')) return storage;
      if (module.endsWith('/conversation-service')) return {
        createConversation: async () => ({ id: 17 }),
        sendStreamMessage: (_id, content, handlers) => {
          events.push({ sent: content });
          handlers.onChunk({ type: 'content', content: '更新后的回答' });
          handlers.onDone();
          return { abort() {} };
        },
      };
      if (module.endsWith('/markdown')) return require('../utils/markdown');
      if (module.endsWith('/api')) return api;
      throw new Error(`Unexpected dependency ${module}`);
    },
  }, { filename: file });
  const page = { ...definition, ...definition.methods, data: structuredClone(definition.data) };
  page.setData = (patch, callback) => { Object.assign(page.data, patch); if (callback) callback.call(page); };
  return { page, wx, events, records: () => conversations };
}

test('home summary counts all records while preview and deletion remain consistent', () => {
  const records = Array.from({ length: 8 }, (_, i) => ({ id: String(i), agentType: 'a_share', title: '研究' }));
  const { page } = fixture('pages/home/home.js', { records });
  page.onShow();
  assert.equal(page.data.conversationCount, 8);
  assert.equal(page.data.recentConversations.length, 5);
  page.onDeleteRecentConversation({ detail: records[0] });
  assert.equal(page.data.conversationCount, 7);
  assert.equal(page.data.recentConversations.length, 5);
  assert.equal(page.data.recentConversations[0].id, '1');
});

test('home authentication and market navigation retain their original behavior', () => {
  const guest = fixture('pages/home/home.js', { authenticated: false });
  guest.page.onShow();
  assert.equal(guest.events[0].url, '/pages/login/login');
  const owner = fixture('pages/home/home.js');
  owner.page.onTapMarket({ detail: markets.MARKETS[1] });
  assert.equal(owner.events[0].url, '/pages/market/market?market=us_stock');
});

test('opening an agent only displays the welcome message without creating history', async () => {
  const { page, records } = fixture('pages/conversation/conversation.js');
  await page.onLoad({ market: 'us_stock' });
  assert.equal(page.data.market.id, 'us_stock');
  assert.equal(page.data.windowHeight, 724);
  assert.equal(page.data.backendConversationId, 0);
  assert.equal(page.data.messages.length, 1);
  assert.equal(records().length, 0);
});

test('composer handles keyboard opening, dismissal, resize, and malformed events', () => {
  const { page } = fixture('pages/conversation/conversation.js');
  page.onKeyboardHeightChange({ detail: { height: 320 } });
  assert.equal(page.data.keyboardHeight, 320);
  page.onKeyboardHeightChange({ detail: { height: 0 } });
  assert.equal(page.data.keyboardHeight, 0);
  for (const height of [-1, 'invalid', undefined]) {
    page.onKeyboardHeightChange({ detail: { height } });
    assert.equal(page.data.keyboardHeight, 0);
  }
  page.onResize({ size: { windowHeight: 500 } });
  assert.equal(page.data.windowHeight, 500);
  page.onKeyboardHeightChange({ detail: { height: 280 } });
  assert.equal(page.data.keyboardHeight, 280);
  page.onInputBlur();
  assert.equal(page.data.keyboardHeight, 0);
});

test('streaming cannot open a destructive conversation menu', () => {
  const { page, events } = fixture('pages/conversation/conversation.js');
  page.data.isLoading = true;
  page.openMenu();
  assert.equal(events.length, 0);
  page.data.isLoading = false;
  page.openMenu();
  events[0].success({ tapIndex: 0 });
  assert.equal(events[1].url, '/pages/home/home');
});

test('clearing a conversation removes history, and regenerate keeps one user question', async () => {
  const user = { id: 'u', role: 'user', content: '分析苹果' };
  const answer = { id: 'a', role: 'assistant', content: '原回答', isStreaming: false };
  const { page, records, events } = fixture('pages/conversation/conversation.js', {
    records: [{ id: 'saved', agentType: 'us_stock', messages: [user, answer] }],
  });
  page.setData({ market: markets.getMarketById('us_stock'), conversationId: 'saved', backendConversationId: 17,
    messages: [user, answer] });
  await page.onRegenerate();
  assert.equal(page.data.messages.filter((item) => item.role === 'user').length, 1);
  assert.equal(page.data.messages[1].content, '更新后的回答');
  page.openMenu();
  events.find((item) => item.itemList)?.success({ tapIndex: 1 });
  assert.equal(records().length, 0);
  assert.equal(page.data.conversationId, '');
});

test('native catchtap copy accepts a WeChat event without DOM stopPropagation', () => {
  const { page, events } = fixture('components/message-bubble/message-bubble.js');
  page.properties = { message: { content: '研究结果' } };
  page.copyContent({});
  assert.equal(events[0].data, '研究结果');
});

test('Markdown formatting escapes HTML while preserving headings, lists, and emphasis', () => {
  const { renderMarkdown } = require('../utils/markdown');
  const html = renderMarkdown('# 结论\n- **增长** <script>alert(1)</script>');
  assert.match(html, /font-weight:600/);
  assert.match(html, /<strong>增长<\/strong>/);
  assert.doesNotMatch(html, /<script>/);
});

test('investment page loads all-market watchlist and keeps account failures independent', async () => {
  const calls = [];
  const api = { request: async ({ url }) => {
    calls.push(url);
    if (url.endsWith('/watchlist/all')) return [{ id: 'AAPL', symbol: 'AAPL', market: 'us_stock', name: 'Apple' }];
    if (url.endsWith('/accounts')) throw new Error('账户暂不可用');
    if (url.endsWith('/all/holdings')) return { holdings: [], account_equities: [], warnings: [], stale: false };
    if (url.endsWith('/price-alerts')) return { alerts: [], email: '', mailer_configured: false };
    throw new Error(`Unexpected ${url}`);
  } };
  const { page } = fixture('pages/invest/invest.js', { api });
  await page.refresh();
  assert.equal(page.data.visibleStocks[0].id, 'AAPL');
  assert.deepEqual([...calls].sort(), ['/api/v1/stocks/watchlist/all', '/api/v1/stocks/watchlist/filing-insights/refresh', '/api/v1/accounts', '/api/v1/accounts/all/holdings', '/api/v1/price-alerts'].sort());
});

test('roundtable does not persist a welcome-only session and preserves speaker events', async () => {
  const calls = [];
  const api = {
    request: async () => ({ id: 29 }),
    streamRequest: ({ onChunk, onDone, data }) => {
      calls.push(data);
      onChunk({ type: 'agent_start', agent_id: 'value', content: '' });
      onChunk({ type: 'content', agent_id: 'value', content: '**估值**' });
      onDone();
      return { abort() {} };
    },
  };
  const { page, records } = fixture('pages/roundtable/roundtable.js', { api });
  page.onLoad({});
  assert.equal(records().length, 0);
  page.onInput({ detail: { value: '@价值派 分析苹果' } });
  await page.send();
  assert.equal(calls[0].participants[0], 'value');
  assert.equal(page.data.messages[2].name, '价值派');
  assert.match(page.data.messages[2].html, /<strong>估值<\/strong>/);
  assert.equal(records().length, 1);
});

test('market overview loads indices and scoped watchlist', async () => {
  const api = { request: async ({ url }) => url.includes('/indices')
    ? [{ id: 'sh000001', name: '上证指数', value: 3000 }]
    : [{ id: 'sh600519', market: 'a_share', name: '贵州茅台' }] };
  const { page } = fixture('pages/market/market.js', { api });
  page.setData({ market: markets.getMarketById('a_share') });
  await page.refresh();
  assert.equal(page.data.indices[0].name, '上证指数');
  assert.equal(page.data.stocks[0].id, 'sh600519');
});

test('company research loads quote, K-line, news and financial data independently', async () => {
  const api = { request: async ({ url }) => {
    if (url.includes('/fundamentals')) return { eps: 2, financial_reports: [] };
    if (url.includes('/price-alerts')) return { alerts: [] };
    if (url.includes('/quote')) return { current_price: 123 };
    if (url.includes('/news')) return [{ id: 'n1', title: '公司新闻' }];
    if (url.includes('/watchlist/all')) return [];
    if (url.includes('/kline')) return [{ open: 1, close: 2, low: 1, high: 2 }];
    throw new Error(`Unexpected ${url}`);
  } };
  const { page } = fixture('pages/research/research.js', { api });
  page.setData({ market: 'a_share', code: 'sh600519', name: '贵州茅台' });
  await page.refresh();
  assert.equal(page.data.quote.current_price, 123);
  assert.equal(page.data.candles.length, 1);
  assert.equal(page.data.news[0].title, '公司新闻');
  assert.equal(page.data.fundamentals.eps, 2);
});
