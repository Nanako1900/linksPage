# LinksPage

[English](README.md) | 简体中文

一个可自托管的社区聚合分享页：把你的 Discord、KOOK、QQ 群、微信群等社区汇总到**一个链接**里，实时展示服务器名称、在线人数、频道与在线成员，并提供加入入口。

> 状态：**M0 脚手架**。服务能启动、迁移数据库、渲染公开页骨架；社区卡片、后台和统计尚未实现。技术选型见 [docs/tech-selection.md](docs/tech-selection.md)。

## 计划特性

- 访客无需登录；管理员通过配置文件中的 OAuth2/OIDC 白名单或本地账号登录
- Discord（widget.json + invite 接口 + 官方 iframe）、KOOK（免 token 徽章解析）、QQ 群 / 微信群二维码卡片
- 在微信 / QQ 内置浏览器中打开 Discord 时引导到系统浏览器
- 无 Cookie、不存原始 IP 的访问统计
- 所有个性化内容（标题、头像、主题、字体、社区、页脚、OG 图、语言……）均可由管理员修改
- 技术栈：Vite + React + Tailwind CSS / Go / PostgreSQL，Docker Compose 部署，前置 Cloudflare

## 快速开始（Docker Compose）

要求：Docker Engine 24+、Compose 2.20.1+，64 位系统。

> v0.1.0（M2a）发布前，GHCR 上还没有镜像，需要先在本地构建：`make docker TAG=dev`，再在 `.env` 中设置 `LINKSPAGE_IMAGE=ghcr.io/nanako1900/linkspage`、`LINKSPAGE_TAG=dev`。

在仓库根目录执行（release 部署包里脚本位于根目录，改用 `./init.sh` 和 `compose.direct.yaml`）：

```sh
sh deploy/init.sh         # 生成 .env、secrets/pg_password、config/config.yaml
# 编辑 .env 中的 LP_BASE_URL；使用 Tunnel 时把 token 写入 secrets/tunnel_token
docker compose run --rm --no-deps app config check
docker compose up -d
```

不使用 Tunnel、由已有反代转发时，叠加 `deploy/compose.direct.yaml`（默认只监听 `127.0.0.1:8080`）：

```sh
docker compose -f compose.yaml -f deploy/compose.direct.yaml up -d
```

## 开发

```sh
make web-install          # 安装前端依赖（pnpm，通过 corepack）
make dev                  # 启动开发数据库、Go 服务和 Vite（http://localhost:5173）
make lint test            # Go 与前端的 lint 和测试
make gen                  # 重新生成 sqlc、OpenAPI 和 orval 客户端
make docker               # 构建镜像
```

## 声明

本项目与 Discord、KOOK、腾讯无关，品牌图标归各自所有者。

## License

[MIT](LICENSE)
