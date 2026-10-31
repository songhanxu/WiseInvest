# 个人工具助手演示站

这是一个独立的静态站点，提供待办清单、专注计时、随手记和浅色／深色切换。页面不调用慧投 API；清单和便签只存于访问者当前浏览器的 `localStorage`。

## 在现有腾讯云部署上启用

仓库根目录的 `docker-compose.yml` 已把本目录只读挂载到现有 Nginx 容器。`nginx/conf.d/personal-assistant-demo.conf` 仅处理 `hanxusong.asia` 和 `www.hanxusong.asia`，不会把这两个 Host 转发到慧投后端。IP 访问和其他 Host 仍由原有 `wiseinvest.conf` 处理。

在服务器拉取本分支后，从仓库根目录执行：

```bash
docker compose config --quiet
docker compose up -d --no-deps --force-recreate nginx
docker exec wiseinvest-nginx nginx -t
```

若服务器使用旧版 Compose，把前两条命令中的 `docker compose` 换成 `docker-compose`。这次只需重建 Nginx 容器，不需要重建后端或数据库。

在域名解析生效前，可直接在服务器验证两个虚拟主机的隔离：

```bash
curl -I -H 'Host: hanxusong.asia' http://127.0.0.1/
curl -I -H 'Host: hanxusong.asia' http://127.0.0.1/api/v1/accounts
curl -I -H 'Host: 127.0.0.1' http://127.0.0.1/health
```

预期依次为 `200`、`404`、后端健康检查响应。域名指向服务器公网 IP 后，站点会通过 HTTP 的 80 端口提供演示页；本配置没有启用 HTTPS，也没有修改慧投的 API 地址。

## 本地预览

```bash
cd personal-assistant-demo
python3 -m http.server 8765
```

访问 `http://127.0.0.1:8765/`。无需安装前端依赖。
