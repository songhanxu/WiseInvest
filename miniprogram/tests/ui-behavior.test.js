const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const markets = require('../utils/market');
const root = path.join(__dirname, '..');

function fixture(file, { records = [], authenticated = true } = {}) {
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
      if (module.endsWith('/conversation-service')) return { createConversation: async () => ({ id: 17 }) };
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
  assert.equal(owner.events[0].url, '/pages/conversation/conversation?market=us_stock');
});

test('native navigation initialization preserves the selected market and saved conversation', async () => {
  const { page, records } = fixture('pages/conversation/conversation.js');
  await page.onLoad({ market: 'us_stock' });
  assert.equal(page.data.market.id, 'us_stock');
  assert.equal(page.data.windowHeight, 724);
  assert.equal(page.data.backendConversationId, 17);
  assert.equal(page.data.messages.length, 1);
  assert.equal(records()[0].agentType, 'us_stock');
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

test('native catchtap copy accepts a WeChat event without DOM stopPropagation', () => {
  const { page, events } = fixture('components/message-bubble/message-bubble.js');
  page.properties = { message: { content: '研究结果' } };
  page.copyContent({});
  assert.equal(events[0].data, '研究结果');
});
