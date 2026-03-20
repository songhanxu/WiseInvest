const { wechatLogin } = require('../../utils/auth-service');

Page({
  data: {
    isLoading: false,
  },

  onShow() {
    const app = getApp();
    if (app.isAuthenticated()) {
      wx.reLaunch({ url: '/pages/home/home' });
    }
  },

  loginWithWeChat() {
    if (this.data.isLoading) return;
    this.setData({ isLoading: true });

    wx.login({
      success: async ({ code }) => {
        try {
          const resp = await wechatLogin(code || `MOCK_${Date.now()}`);
          const user = resp.user || {};

          getApp().setAuth(resp.token || '', user);

          if (resp.needs_phone_binding) {
            wx.reLaunch({ url: '/pages/phone-binding/phone-binding?fromLogin=1' });
          } else {
            wx.reLaunch({ url: '/pages/home/home' });
          }
        } catch (error) {
          const message = error?.message || '登录失败';
          if (message.includes('当前域名未加入小程序 request 合法域名')) {
            wx.showModal({
              title: '登录失败',
              content: `${message}\n\n请到微信公众平台 -> 开发管理 -> 开发设置 -> 服务器域名，把该域名加入 request 合法域名。`,
              showCancel: false,
            });
          } else {
            wx.showToast({ title: message, icon: 'none' });
          }
        } finally {
          this.setData({ isLoading: false });
        }
      },
      fail: () => {
        this.setData({ isLoading: false });
        wx.showToast({ title: '微信登录失败', icon: 'none' });
      },
    });
  },
});
