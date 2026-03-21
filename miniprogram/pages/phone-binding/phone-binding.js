const { sendPhoneCode, bindPhone } = require('../../utils/auth-service');

Page({
  data: {
    phone: '',
    code: '',
    codeSent: false,
    countdown: 0,
    isLoading: false,
    disabledSubmit: true,
  },

  timer: null,

  onUnload() {
    if (this.timer) clearInterval(this.timer);
  },

  onPhoneInput(e) {
    this.setData({ phone: e.detail.value || '' }, this.syncDisabledState);
  },

  onCodeInput(e) {
    this.setData({ code: e.detail.value || '' }, this.syncDisabledState);
  },

  syncDisabledState() {
    const { phone, code, codeSent, isLoading } = this.data;
    const disabled = isLoading || phone.length < 11 || (codeSent && code.length < 6);
    this.setData({ disabledSubmit: disabled });
  },

  async sendCode() {
    if (this.data.phone.length < 11) {
      wx.showToast({ title: '请输入正确手机号', icon: 'none' });
      return;
    }

    this.setData({ isLoading: true }, this.syncDisabledState);
    try {
      await sendPhoneCode(this.data.phone);
      this.setData({ codeSent: true, countdown: 60 });
      this.startCountdown();
      wx.showToast({ title: '验证码已发送', icon: 'none' });
    } catch (error) {
      wx.showToast({ title: error.message || '发送失败', icon: 'none' });
    } finally {
      this.setData({ isLoading: false }, this.syncDisabledState);
    }
  },

  startCountdown() {
    if (this.timer) clearInterval(this.timer);
    this.timer = setInterval(() => {
      const countdown = this.data.countdown;
      if (countdown <= 1) {
        clearInterval(this.timer);
        this.timer = null;
        this.setData({ countdown: 0 });
        return;
      }
      this.setData({ countdown: countdown - 1 });
    }, 1000);
  },

  async handlePrimaryTap() {
    if (!this.data.codeSent) {
      await this.sendCode();
      return;
    }

    this.setData({ isLoading: true }, this.syncDisabledState);
    try {
      const resp = await bindPhone(this.data.phone, this.data.code);
      const app = getApp();
      app.setAuth(resp.token || app.globalData.auth.token, resp.user || app.globalData.auth.user);
      wx.reLaunch({ url: '/pages/home/home' });
    } catch (error) {
      wx.showToast({ title: error.message || '绑定失败', icon: 'none' });
    } finally {
      this.setData({ isLoading: false }, this.syncDisabledState);
    }
  },

  skip() {
    wx.reLaunch({ url: '/pages/home/home' });
  },
});
