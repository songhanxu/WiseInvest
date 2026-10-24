const { request } = require('../../utils/api');
const { MARKETS } = require('../../utils/market');

const base = '/api/v1';
const key = (stock) => `${stock.market}:${stock.id}`;

Page({
  data: {
    mode: 'watchlist', filter: 'all', markets: [{ id: 'all', displayName: '全部' }, ...MARKETS], searchMarkets: [{ id: 'all', displayName: '全部市场' }, ...MARKETS],
    stocks: [], visibleStocks: [], searchMarket: 'all', searchMarketIndex: 0, query: '', results: [],
    accounts: [], holdings: [], equities: [], warnings: [], snapshotStale: false,
    alertEmail: '', mailerConfigured: false, alerts: [], busy: false, error: '',
  },
  searchTimer: null,

  onShow() { this.refresh(); },
  onUnload() { if (this.searchTimer) clearTimeout(this.searchTimer); },
  onPullDownRefresh() { this.refresh().finally(() => wx.stopPullDownRefresh()); },

  async refresh() {
    this.setData({ busy: true, error: '' });
    const tasks = await Promise.allSettled([
      request({ url: `${base}/stocks/watchlist/all`, needAuth: true }),
      request({ url: `${base}/accounts`, needAuth: true }),
      request({ url: `${base}/accounts/all/holdings`, needAuth: true }),
      request({ url: `${base}/price-alerts`, needAuth: true }),
    ]);
    const [stocks, accounts, snapshot, alerts] = tasks.map((result) => result.status === 'fulfilled' ? result.value : null);
    this.setData({
      stocks: Array.isArray(stocks) ? stocks : [],
      accounts: Array.isArray(accounts) ? accounts : [],
      holdings: snapshot && Array.isArray(snapshot.holdings) ? snapshot.holdings.map((row) => {
        const saved = alerts && Array.isArray(alerts.alerts) ? alerts.alerts.find((item) => item.holding_id === row.holding_id) : null;
        return { ...row, targetPrice: saved ? String(saved.target_price) : '', hasAlert: !!saved };
      }) : [],
      equities: snapshot && Array.isArray(snapshot.account_equities) ? snapshot.account_equities : [],
      warnings: snapshot && Array.isArray(snapshot.warnings) ? snapshot.warnings : [],
      snapshotStale: !!(snapshot && snapshot.stale),
      alerts: alerts && Array.isArray(alerts.alerts) ? alerts.alerts : [],
      alertEmail: alerts ? alerts.email || '' : '',
      mailerConfigured: !!(alerts && alerts.mailer_configured),
      busy: false,
      error: tasks[0].status === 'rejected' ? tasks[0].reason.message : '',
    });
    this.applyFilter();
    if (this.data.stocks.some((row) => row.market === 'us_stock')) {
      request({ url: `${base}/stocks/watchlist/filing-insights/refresh`, method: 'POST', needAuth: true, data: {} }).catch(() => {});
    }
    return tasks;
  },

  setMode(e) { this.setData({ mode: e.currentTarget.dataset.mode }); },
  setFilter(e) { this.setData({ filter: e.currentTarget.dataset.market }, () => this.applyFilter()); },
  applyFilter() {
    const { stocks, filter } = this.data;
    this.setData({ visibleStocks: filter === 'all' ? stocks : stocks.filter((row) => row.market === filter) });
  },
  onQuery(e) {
    const query = e.detail.value || '';
    this.setData({ query });
    if (this.searchTimer) clearTimeout(this.searchTimer);
    if (!query.trim()) { this.setData({ results: [] }); return; }
    this.searchTimer = setTimeout(() => this.search(query), 280);
  },
  setSearchMarket(e) { const index = Number(e.detail.value); this.setData({ searchMarket: this.data.searchMarkets[index].id, searchMarketIndex: index }, () => this.onQuery({ detail: { value: this.data.query } })); },
  async search(query) {
    try {
      const markets = this.data.searchMarket === 'all' ? MARKETS.map((item) => item.id) : [this.data.searchMarket];
      const responses = await Promise.allSettled(markets.map((market) => request({ url: `${base}/stocks/search?q=${encodeURIComponent(query)}&market=${market}` })));
      const results = responses.flatMap((result) => result.status === 'fulfilled' && Array.isArray(result.value) ? result.value : []);
      if (query === this.data.query) this.setData({ results: Array.isArray(results) ? results.slice(0, 12) : [] });
    } catch (error) { this.setData({ error: error.message, results: [] }); }
  },
  async addStock(e) {
    const stock = this.data.results[e.currentTarget.dataset.index];
    if (!stock) return;
    try {
      await request({ url: `${base}/stocks/watchlist`, method: 'POST', needAuth: true,
        data: { stock_code: stock.id, symbol: stock.symbol, name: stock.name, market: stock.market } });
      this.setData({ query: '', results: [] });
      await this.refresh();
    } catch (error) { this.setData({ error: error.message }); }
  },
  async removeStock(e) {
    const stock = this.data.stocks.find((row) => key(row) === e.currentTarget.dataset.key);
    if (!stock) return;
    try {
      await request({ url: `${base}/stocks/watchlist`, method: 'DELETE', needAuth: true,
        data: { stock_code: stock.id, market: stock.market } });
      await this.refresh();
    } catch (error) { this.setData({ error: error.message }); }
  },
  openResearch(e) {
    const stock = this.data.stocks.find((row) => key(row) === e.currentTarget.dataset.key);
    if (!stock) return;
    wx.navigateTo({ url: `/pages/research/research?market=${stock.market}&code=${encodeURIComponent(stock.id)}&name=${encodeURIComponent(stock.name)}&symbol=${encodeURIComponent(stock.symbol || '')}` });
  },
  onEmail(e) { this.setData({ alertEmail: e.detail.value || '' }); },
  async saveEmail() {
    try {
      await request({ url: `${base}/price-alerts/settings`, method: 'PUT', needAuth: true, data: { email: this.data.alertEmail.trim() } });
      wx.showToast({ title: '邮箱已保存', icon: 'success' });
    } catch (error) { this.setData({ error: error.message }); }
  },
  onHoldingTarget(e) {
    const id = e.currentTarget.dataset.id;
    this.setData({ holdings: this.data.holdings.map((row) => row.holding_id === id ? { ...row, targetPrice: e.detail.value || '' } : row) });
  },
  async saveHoldingAlert(e) {
    const row = this.data.holdings.find((item) => item.holding_id === e.currentTarget.dataset.id);
    const price = Number(row && row.targetPrice);
    if (!row || !Number.isFinite(price) || price <= 0) { this.setData({ error: '请输入有效的目标价格' }); return; }
    try {
      await request({ url: `${base}/price-alerts`, method: 'PUT', needAuth: true,
        data: { account_id: row.account_id, holding_id: row.holding_id, target_price: price, direction: 'auto' } });
      this.setData({ holdings: this.data.holdings.map((item) => item.holding_id === row.holding_id ? { ...item, hasAlert: true } : item), error: '' });
    } catch (error) { this.setData({ error: error.message }); }
  },
  async removeHoldingAlert(e) {
    const id = e.currentTarget.dataset.id;
    try {
      await request({ url: `${base}/price-alerts?holding_id=${encodeURIComponent(id)}`, method: 'DELETE', needAuth: true });
      this.setData({ holdings: this.data.holdings.map((item) => item.holding_id === id ? { ...item, hasAlert: false, targetPrice: '' } : item), error: '' });
    } catch (error) { this.setData({ error: error.message }); }
  },
});
