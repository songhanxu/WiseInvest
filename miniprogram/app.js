function tokenIsActive(token) {
  if (!token) return false;
  try {
    const payload = token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/');
    const padded = payload + '='.repeat((4 - payload.length % 4) % 4);
    const claims = JSON.parse(String.fromCharCode(...new Uint8Array(wx.base64ToArrayBuffer(padded))));
    return Number(claims.exp) * 1000 > Date.now();
  } catch (_) { return false; }
}

App({
  globalData: {
    auth: {
      token: '',
      user: null,
    },
    ui: {
      theme: 'light',
    },
  },

  onLaunch() {
    const token = wx.getStorageSync('auth_token') || '';
    const user = wx.getStorageSync('auth_user') || null;
    this.globalData.auth = tokenIsActive(token) ? { token, user } : { token: '', user: null };
    if (token && !tokenIsActive(token)) this.clearAuth();
  },

  onShow() {
    if (this.globalData.auth.token && !this.isAuthenticated()) this.clearAuth();
    if (!this.isAuthenticated() || this.refreshing || Date.now() - (this.lastRefresh || 0) < 86400000) return;
    this.refreshing = true;
    this.lastRefresh = Date.now();
    const { request } = require('./utils/api');
    request({ url: '/api/v1/auth/refresh', method: 'POST', needAuth: true })
      .then((result) => { if (result.token && result.user) this.setAuth(result.token, result.user); })
      .catch((error) => {
        this.lastRefresh = 0;
        if (/invalid or expired token/.test(error.message || '')) this.clearAuth();
      })
      .finally(() => { this.refreshing = false; });
  },

  isAuthenticated() {
    return tokenIsActive(this.globalData.auth.token);
  },

  setAuth(token, user) {
    this.globalData.auth = { token, user };
    wx.setStorageSync('auth_token', token || '');
    wx.setStorageSync('auth_user', user || null);
  },

  clearAuth() {
    this.globalData.auth = { token: '', user: null };
    wx.removeStorageSync('auth_token');
    wx.removeStorageSync('auth_user');
  },
});
