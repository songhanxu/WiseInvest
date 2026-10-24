const { MARKETS } = require('../../utils/market');
const { loadConversations, removeConversationById } = require('../../utils/storage');

Page({
  data: {
    markets: MARKETS,
    recentConversations: [],
    conversationCount: 0,
    todayLabel: '',
  },

  onShow() {
    const app = getApp();
    if (!app.isAuthenticated()) {
      wx.reLaunch({ url: '/pages/login/login' });
      return;
    }

    this.refreshConversations();
    const today = new Date();
    this.setData({ todayLabel: `${today.getMonth() + 1}月${today.getDate()}日 · ${['周日', '周一', '周二', '周三', '周四', '周五', '周六'][today.getDay()]}` });
  },

  refreshConversations() {
    const conversations = loadConversations();
    this.setData({ recentConversations: conversations.slice(0, 5), conversationCount: conversations.length });
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
      url: `/pages/market/market?market=${market.id}`,
    });
  },

  openInvest() {
    wx.navigateTo({ url: '/pages/invest/invest' });
  },

  openRoundtable() {
    wx.navigateTo({ url: '/pages/roundtable/roundtable' });
  },

  onTapRecentConversation(e) {
    const conversation = e.detail || {};
    if (conversation.agentType === 'group_chat') {
      wx.navigateTo({ url: `/pages/roundtable/roundtable?conversationId=${conversation.id}` });
      return;
    }
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
        this.refreshConversations();
      },
    });
  },
});
