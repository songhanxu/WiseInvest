App({
  globalData: {
    auth: {
      token: '',
      user: null,
    },
    ui: {
      theme: 'dark',
    },
  },

  onLaunch() {
    const token = wx.getStorageSync('auth_token') || '';
    const user = wx.getStorageSync('auth_user') || null;
    this.globalData.auth = {
      token,
      user,
    };
  },

  isAuthenticated() {
    return !!this.globalData.auth.token;
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
