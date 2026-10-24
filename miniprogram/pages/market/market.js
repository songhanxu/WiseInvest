const { request } = require('../../utils/api');
const { getMarketById } = require('../../utils/market');

Page({
  data: { market: getMarketById('a_share'), indices: [], stocks: [], results: [], query: '', loading: true, error: '' },
  searchTimer: null,
  onLoad(options) { this.setData({ market: getMarketById(options.market) }); this.refresh(); },
  onUnload() { if (this.searchTimer) clearTimeout(this.searchTimer); },
  onPullDownRefresh() { this.refresh().finally(() => wx.stopPullDownRefresh()); },
  async refresh() {
    const market = this.data.market.id;
    this.setData({ loading: true, error: '' });
    const responses = await Promise.allSettled([
      request({ url: `/api/v1/stocks/indices?market=${market}` }),
      request({ url: `/api/v1/stocks/watchlist?market=${market}`, needAuth: true }),
    ]);
    this.setData({ indices: responses[0].status === 'fulfilled' && Array.isArray(responses[0].value) ? responses[0].value : [],
      stocks: responses[1].status === 'fulfilled' && Array.isArray(responses[1].value) ? responses[1].value : [],
      error: responses[0].status === 'rejected' ? responses[0].reason.message : '', loading: false });
  },
  onSearchInput(e) {
    const query = e.detail.value || '';
    this.setData({ query });
    if (this.searchTimer) clearTimeout(this.searchTimer);
    if (!query.trim()) { this.setData({ results: [] }); return; }
    this.searchTimer = setTimeout(async () => {
      try {
        const results = await request({ url: `/api/v1/stocks/search?q=${encodeURIComponent(query)}&market=${this.data.market.id}` });
        if (this.data.query === query) this.setData({ results: Array.isArray(results) ? results.slice(0, 10) : [] });
      } catch (error) { this.setData({ error: error.message }); }
    }, 280);
  },
  openStock(e) {
    const index = Number(e.currentTarget.dataset.index);
    const stock = e.currentTarget.dataset.collection === 'results' ? this.data.results[index] : this.data.stocks[index];
    if (!stock) return;
    wx.navigateTo({ url: `/pages/research/research?market=${stock.market}&code=${encodeURIComponent(stock.id)}&name=${encodeURIComponent(stock.name)}&symbol=${encodeURIComponent(stock.symbol || '')}` });
  },
  openAgent() { wx.navigateTo({ url: `/pages/conversation/conversation?market=${this.data.market.id}` }); },
  openInvest() { wx.navigateTo({ url: '/pages/invest/invest' }); },
});
