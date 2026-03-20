const { MARKETS } = require('../../utils/market');
const { loadConversations, removeConversationById } = require('../../utils/storage');

Page({
  data: {
    markets: MARKETS,
    recentConversations: [],
  },

  onShow() {
    const app = getApp();
    if (!app.isAuthenticated()) {
      wx.reLaunch({ url: '/pages/login/login' });
      return;
    }

    this.setData({
      recentConversations: loadConversations().slice(0, 5),
    });
  },

  openAccountMenu() {
    wx.showActionSheet({
      itemList: ['退出登录'],
      success: ({ tapIndex }) => {
        if (tapIndex !== 0) return;
        wx.showModal({
          title: '退出登录',
          content: '确认退出当前账号吗？',
          success: ({ confirm }) => {
            if (!confirm) return;
            getApp().clearAuth();
            wx.reLaunch({ url: '/pages/login/login' });
          },
        });
      },
    });
  },

  onTapMarket(e) {
    const market = e.detail || {};
    wx.navigateTo({
      url: `/pages/conversation/conversation?market=${market.id}`,
    });
  },

  onTapRecentConversation(e) {
    const conversation = e.detail || {};
    wx.navigateTo({
      url: `/pages/conversation/conversation?market=${conversation.agentType}&conversationId=${conversation.id}`,
    });
  },

  onDeleteRecentConversation(e) {
    const conversation = e.detail || {};
    wx.showModal({
      title: '删除对话',
      content: '确认删除这条最近对话吗？',
      success: ({ confirm }) => {
        if (!confirm) return;
        removeConversationById(conversation.id);
        this.setData({ recentConversations: loadConversations().slice(0, 5) });
      },
    });
  },
});
