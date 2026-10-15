Component({
  options: { styleIsolation: 'apply-shared' },
  properties: {
    wide: { type: Boolean, value: false },
    market: {
      type: Object,
      value: null,
    },
  },

  methods: {
    handleTap() {
      this.triggerEvent('tapcard', this.properties.market || {});
    },
  },
});
