Component({
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
      if (!v || !v.timestamp) return;
      const d = new Date(v.timestamp);
      const hh = String(d.getHours()).padStart(2, '0');
      const mm = String(d.getMinutes()).padStart(2, '0');
      this.setData({ timeText: `${hh}:${mm}` });
    },
  },

  data: {
    timeText: '',
  },

  methods: {
    copyContent(e) {
      e.stopPropagation();
      const content = this.properties.message?.content || '';
      wx.setClipboardData({ data: content });
    },

    regenerate(e) {
      e.stopPropagation();
      this.triggerEvent('regenerate');
    },
  },
});
