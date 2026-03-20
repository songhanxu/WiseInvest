Component({
  properties: {
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
