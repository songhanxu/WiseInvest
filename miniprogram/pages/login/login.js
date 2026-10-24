const { wechatLogin, sendPhoneCode, phoneLogin } = require('../../utils/auth-service');

Page({
  data: {
    isLoading: false,
    mode: 'wechat',
    phone: '',
    code: '',
    codeSent: false,
    countdown: 0,
  },

  onUnload() { if (this.codeTimer) clearInterval(this.codeTimer); },

  switchMode() {
    this.setData({ mode: this.data.mode === 'wechat' ? 'phone' : 'wechat' });
  },

  onPhoneInput(e) { this.setData({ phone: e.detail.value || '' }); },
  onCodeInput(e) { this.setData({ code: e.detail.value || '' }); },

  async sendCode() {
    if (!/^1[3-9]\d{9}$/.test(this.data.phone) || this.data.countdown > 0 || this.data.isLoading) {
      wx.showToast({ title: '请输入有效的手机号', icon: 'none' });
      return;
    }
    this.setData({ isLoading: true });
    try {
      await sendPhoneCode(this.data.phone);
      this.setData({ codeSent: true, countdown: 60 });
      this.codeTimer = setInterval(() => {
        const countdown = this.data.countdown - 1;
        this.setData({ countdown });
        if (countdown <= 0) { clearInterval(this.codeTimer); this.codeTimer = null; }
      }, 1000);
      wx.showToast({ title: '验证码已发送', icon: 'none' });
    } catch (error) {
      wx.showToast({ title: error.message || '发送失败', icon: 'none' });
    } finally { this.setData({ isLoading: false }); }
  },

  async loginWithPhone() {
    if (!/^1[3-9]\d{9}$/.test(this.data.phone) || !/^\d{6}$/.test(this.data.code) || this.data.isLoading) return;
    this.setData({ isLoading: true });
    try {
      const resp = await phoneLogin(this.data.phone, this.data.code);
      getApp().setAuth(resp.token, resp.user);
      wx.reLaunch({ url: '/pages/home/home' });
    } catch (error) {
      wx.showToast({ title: error.message || '登录失败', icon: 'none' });
    } finally { this.setData({ isLoading: false }); }
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
          if (!code) throw new Error('微信未返回登录凭证，请重试');
          const resp = await wechatLogin(code);
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
