const { getMarketById } = require('../../utils/market');

function timeAgo(ts) {
  const now = Date.now();
  const diff = now - (new Date(ts).getTime() || now);
  if (diff < 3600000) {
    return `${Math.max(1, Math.floor(diff / 60000))} 分钟前`;
  }
  if (diff < 86400000) {
    return `${Math.floor(diff / 3600000)} 小时前`;
  }
  const d = new Date(ts);
  return `${d.getMonth() + 1}月${d.getDate()}日`;
}

Component({
  options: { styleIsolation: 'apply-shared' },
  properties: {
    conversation: {
      type: Object,
      value: null,
    },
  },

  observers: {
    conversation(v) {
      if (!v) return;
      const market = getMarketById(v.agentType || 'a_share');
      const roundtable = v.agentType === 'group_chat';
      this.setData({
        displayTitle: v.title || `${roundtable ? '慧投圆桌' : market.displayName} · 新对话`,
        timeText: timeAgo(v.updatedAt),
        marketMark: roundtable ? '圆' : market.mark,
        marketName: roundtable ? '慧投圆桌' : market.displayName,
      });
    },
  },

  data: {
    displayTitle: '',
    timeText: '',
    marketMark: 'A',
    marketName: 'A 股',
  },

  methods: {
    handleTap() {
      this.triggerEvent('taprow', this.properties.conversation || {});
    },

    handleLongPress() {
      this.triggerEvent('delete', this.properties.conversation || {});
    },
  },
});
