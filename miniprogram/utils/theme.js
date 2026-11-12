function applyTheme(page) {
  const app = getApp();
  const theme = app.globalData.ui.theme === 'dark' ? 'dark' : 'light';
  page.setData({ theme });
  if (wx.setNavigationBarColor) wx.setNavigationBarColor({ frontColor: theme === 'dark' ? '#ffffff' : '#000000', backgroundColor: theme === 'dark' ? '#000000' : '#f2f2f7' });
  if (wx.setBackgroundTextStyle) wx.setBackgroundTextStyle({ textStyle: theme === 'dark' ? 'light' : 'dark' });
  if (wx.setBackgroundColor) wx.setBackgroundColor({ backgroundColor: theme === 'dark' ? '#000000' : '#f2f2f7', backgroundColorTop: theme === 'dark' ? '#000000' : '#f2f2f7', backgroundColorBottom: theme === 'dark' ? '#000000' : '#f2f2f7' });
}

function themedPage(definition) {
  const load = definition.onLoad;
  const show = definition.onShow;
  return {
    ...definition,
    data: { ...definition.data, theme: 'light' },
    onLoad(...args) { applyTheme(this); return load && load.apply(this, args); },
    onShow(...args) { applyTheme(this); return show && show.apply(this, args); },
    toggleTheme() {
      const app = getApp();
      app.setTheme(app.globalData.ui.theme === 'dark' ? 'light' : 'dark');
      applyTheme(this);
    },
  };
}
module.exports = { themedPage };
