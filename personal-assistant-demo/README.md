# 个人工具助手演示站

这是一个保留的独立静态演示项目，提供待办清单、专注计时、随手记和浅色／深色切换。页面不调用慧投 API；清单和便签只存于访问者当前浏览器的 `localStorage`。它已不参与服务器部署；`hanxusong.asia` 现在提供慧投网站。

## 本地预览

```bash
cd personal-assistant-demo
python3 -m http.server 8765
```

访问 `http://127.0.0.1:8765/`。无需安装前端依赖。
