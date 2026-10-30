const { request } = require('../../utils/api');
const { renderMarkdown } = require('../../utils/markdown');

const metricLabels = { revenue: '营业收入', net_income: '净利润', operating_income: '营业利润', assets: '总资产', liabilities: '总负债', operating_cash_flow: '经营现金流', diluted_eps: '摊薄每股收益' };

Page({
  data: {
    market: '', code: '', name: '', symbol: '', fundamentals: null, reports: [], insight: null,
    insightHTML: '', insightCurrent: false, sources: [], targetPrice: '', alert: null, loading: true, error: '',
    quote: null, news: [], candles: [], period: '1d', watched: false,
  },
  pollTimer: null,
  onLoad(options) {
    this.setData({ market: options.market || '', code: decodeURIComponent(options.code || ''), name: decodeURIComponent(options.name || ''), symbol: decodeURIComponent(options.symbol || '') });
    this.refresh();
  },
  onUnload() { if (this.pollTimer) clearTimeout(this.pollTimer); },
  onPullDownRefresh() { this.refresh().finally(() => wx.stopPullDownRefresh()); },
  async refresh() {
    this.setData({ loading: true, error: '' });
    const { market, code } = this.data;
    const tasks = await Promise.allSettled([
      request({ url: `/api/v1/stocks/fundamentals?market=${market}&code=${encodeURIComponent(code)}` }),
      request({ url: '/api/v1/price-alerts', needAuth: true }),
      request({ url: `/api/v1/stocks/quote?market=${market}&code=${encodeURIComponent(code)}` }),
      request({ url: `/api/v1/stocks/news?market=${market}&code=${encodeURIComponent(code)}&name=${encodeURIComponent(this.data.name)}` }),
      request({ url: '/api/v1/stocks/watchlist/all', needAuth: true }),
    ]);
    const fundamentals = tasks[0].status === 'fulfilled' ? tasks[0].value : null;
    const reports = fundamentals && Array.isArray(fundamentals.financial_reports) ? fundamentals.financial_reports.map((report) => ({
      ...report, displayForm: report.kind === 'earnings_release' ? '8-K 业绩公告' : report.form,
      metricRows: Object.keys(report.metrics || {}).map((key) => ({ label: metricLabels[key] || key.replace(/_/g, ' '), ...report.metrics[key] })),
    })) : [];
    const alerts = tasks[1].status === 'fulfilled' ? tasks[1].value.alerts || [] : [];
    const alert = alerts.find((row) => row.holding_id === `watchlist:${market}:${code}`) || null;
    const quote = tasks[2].status === 'fulfilled' ? tasks[2].value : null;
    const news = tasks[3].status === 'fulfilled' && Array.isArray(tasks[3].value) ? tasks[3].value : [];
    const watchlist = tasks[4].status === 'fulfilled' && Array.isArray(tasks[4].value) ? tasks[4].value : [];
    this.setData({ fundamentals, reports, alert, targetPrice: alert ? String(alert.target_price) : '', loading: false,
      quote, news, watched: watchlist.some((item) => item.market === market && item.id === code),
      error: tasks[0].status === 'rejected' ? tasks[0].reason.message : '' });
    await this.loadKline();
    if (market === 'us_stock' && this.data.watched && reports.length) await this.loadInsight(true);
  },
  async loadKline() {
    try {
      const { market, code, period } = this.data;
      const candles = await request({ url: `/api/v1/stocks/kline?market=${market}&code=${encodeURIComponent(code)}&period=${period}&limit=80` });
      this.setData({ candles: Array.isArray(candles) ? candles : [] }, () => this.drawKline());
    } catch (_) { this.setData({ candles: [] }); }
  },
  setPeriod(e) { this.setData({ period: e.currentTarget.dataset.period }, () => this.loadKline()); },
  drawKline() {
    if (!this.data.candles.length || !wx.createCanvasContext || !wx.createSelectorQuery) return;
    wx.createSelectorQuery().in(this).select('#kline').boundingClientRect((rect) => {
      if (!rect || !rect.width) return;
      const ctx = wx.createCanvasContext('kline', this);
      const candles = this.data.candles.slice(-60);
      const width = rect.width; const height = rect.height;
      const min = Math.min(...candles.map((item) => item.low));
      const max = Math.max(...candles.map((item) => item.high));
      const range = max - min || 1;
      const y = (price) => 10 + (max - price) / range * (height - 20);
      const step = width / candles.length;
      ctx.clearRect(0, 0, width, height);
      candles.forEach((item, index) => {
        const up = item.close >= item.open;
        ctx.setStrokeStyle(up ? '#bb5a4b' : '#328269');
        ctx.setFillStyle(up ? '#bb5a4b' : '#328269');
        const x = index * step + step / 2;
        ctx.beginPath(); ctx.moveTo(x, y(item.high)); ctx.lineTo(x, y(item.low)); ctx.stroke();
        const top = y(Math.max(item.open, item.close));
        const body = Math.max(1, Math.abs(y(item.open) - y(item.close)));
        ctx.fillRect(x - Math.max(1, step * 0.28), top, Math.max(2, step * 0.56), body);
      });
      ctx.draw();
    }).exec();
  },
  async addToWatchlist() {
    try {
      await request({ url: '/api/v1/stocks/watchlist', method: 'POST', needAuth: true,
        data: { stock_code: this.data.code, symbol: this.data.symbol || this.data.code, name: this.data.name, market: this.data.market } });
      this.setData({ watched: true, error: '' });
      if (this.data.market === 'us_stock' && this.data.reports.length) await this.loadInsight(true);
    } catch (error) { this.setData({ error: error.message }); }
  },
  openAgent() { wx.navigateTo({ url: `/pages/conversation/conversation?market=${this.data.market}` }); },
  openRoundtable() { wx.navigateTo({ url: '/pages/roundtable/roundtable' }); },
  async loadInsight(start) {
    try {
      if (start) await request({ url: '/api/v1/stocks/watchlist/filing-insights/refresh', method: 'POST', needAuth: true,
        data: { code: this.data.code } });
      const response = await request({ url: '/api/v1/stocks/watchlist/filing-insights', needAuth: true });
      const insight = (response.insights || {})[this.data.code] || null;
      const current = !!(insight && insight.status === 'ready' && this.data.reports[0] && insight.accession === this.data.reports[0].accession);
      this.setData({ insight, insightCurrent: current, insightHTML: renderMarkdown(current && insight.content), sources: current ? insight.sources || [] : [] });
      if (this.pollTimer) clearTimeout(this.pollTimer);
      if (insight && insight.status === 'pending') this.pollTimer = setTimeout(() => this.loadInsight(false), 5000);
    } catch (error) { this.setData({ error: error.message }); }
  },
  async retryInsight() {
    try {
      await request({ url: '/api/v1/stocks/watchlist/filing-insights/refresh', method: 'POST', needAuth: true,
        data: { code: this.data.code, retry: true, accession: this.data.insight && this.data.insight.accession } });
      await this.loadInsight(false);
    } catch (error) { this.setData({ error: error.message }); }
  },
  onTargetPrice(e) { this.setData({ targetPrice: e.detail.value || '' }); },
  async saveAlert() {
    if (!this.data.watched) { this.setData({ error: '请先加入自选，再设置价格提醒' }); return; }
    const target = Number(this.data.targetPrice);
    if (!Number.isFinite(target) || target <= 0) { this.setData({ error: '请输入有效的目标价格' }); return; }
    try {
      const alert = await request({ url: '/api/v1/price-alerts', method: 'PUT', needAuth: true,
        data: { account_id: 'watchlist', holding_id: `watchlist:${this.data.market}:${this.data.code}`, target_price: target, direction: 'auto' } });
      this.setData({ alert, error: '' }); wx.showToast({ title: '提醒已保存', icon: 'success' });
    } catch (error) { this.setData({ error: error.message }); }
  },
  async removeAlert() {
    try {
      await request({ url: `/api/v1/price-alerts?holding_id=${encodeURIComponent(`watchlist:${this.data.market}:${this.data.code}`)}`, method: 'DELETE', needAuth: true });
      this.setData({ alert: null, targetPrice: '' });
    } catch (error) { this.setData({ error: error.message }); }
  },
  copySource(e) {
    const url = e.currentTarget.dataset.url;
    if (url && /^https?:\/\//.test(url)) wx.setClipboardData({ data: url });
  },
});
