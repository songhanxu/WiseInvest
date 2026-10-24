const { renderMarkdown } = require('../../utils/markdown');

Component({
  options: { styleIsolation: 'apply-shared' },
  properties: {
    message: {
      type: Object,
      value: null,
    },
    showRegenerate: {
      type: Boolean,
      value: false,
    },
  },

  observers: {
    message(v) {
      this.setData({ renderedContent: renderMarkdown(v && v.content) });
      if (!v || !v.timestamp) return;
      const d = new Date(v.timestamp);
      const hh = String(d.getHours()).padStart(2, '0');
      const mm = String(d.getMinutes()).padStart(2, '0');
      this.setData({ timeText: `${hh}:${mm}` });
    },
  },

  data: {
    timeText: '',
    renderedContent: '',
  },

  methods: {
    copyContent(e) {
      const content = this.properties.message?.content || '';
      wx.setClipboardData({ data: content });
    },

    regenerate(e) {
      this.triggerEvent('regenerate');
    },
  },
});
