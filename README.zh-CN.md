# LinksPage

[English](README.md) | 简体中文

一个可自托管的社区聚合分享页：把你的 Discord、KOOK、QQ 群、微信群等社区汇总到**一个链接**里，实时展示服务器名称、在线人数、频道与在线成员，并提供加入入口。

> 状态：**M1 公开页 MVP**。公开页已完整可用，能实时展示 Discord 和 KOOK 数据；后台（M2a）还没有，内容通过 `seed.yaml` 配置。M1 的交付内容和已知限制见 [docs/m1/notes.md](docs/m1/notes.md)，技术选型见 [docs/tech-selection.md](docs/tech-selection.md)。

## 特性

| 特性 | 状态 |
|---|---|
| Discord 卡片：widget.json + 永久邀请（名称、图标、在线 / 成员数、频道、在线成员），点击才加载的官方 iframe | ✅ M1 |
| KOOK 卡片，免 Bot Token（徽章解析：名称、在线 / 总数） | ✅ M1 |
| QQ 群（复制群号、二维码）和微信群（内联二维码、兜底联系方式）卡片；12 个平台预设的静态卡片 | ✅ M1 |
| `/go/{slug}` 加入链接；微信 / QQ 内引导到浏览器打开；手机上「在电脑上打开」 | ✅ M1 |
| 服务端兜底 markup、hash CSP、OG 图和 favicon 自动生成、`/c/{slug}` 分享页、中英双语 | ✅ M1 |
| 可选的 Cloudflare Worker 边缘前端（拓扑 C，见 [deploy/cloudflare-worker](deploy/cloudflare-worker/README.zh-CN.md)） | ✅ M1 |
| 通过 `seed.yaml` 配置内容（[示例](config/seed.example.yaml)） | ✅ M1（后台上线前） |
| 后台；管理员写在配置文件中（先本地账号，后 OAuth2/OIDC 白名单） | M2a / M2b |
| 主题编辑器、更多语言 | M2b |
| 无 Cookie、不存原始 IP 的访问统计（`/go` 点击暂不计数） | M3 |
| KOOK Bot Token 级、在线人数曲线 | M4 |

技术栈：Vite + React + Tailwind CSS / Go / PostgreSQL，Docker Compose 部署，前置 Cloudflare。

## 快速开始（Docker Compose）

要求：Docker Engine 24+、Compose 2.20.1+，64 位系统。

> v0.1.0（M2a）发布前，GHCR 上还没有镜像，需要先在本地构建：`make docker TAG=dev`，再在 `.env` 中设置 `LINKSPAGE_IMAGE=ghcr.io/nanako1900/linkspage`、`LINKSPAGE_TAG=dev`。

在仓库根目录执行（release 部署包里脚本位于根目录，改用 `./init.sh` 和 `compose.direct.yaml`）：

```sh
sh deploy/init.sh         # 生成 .env、secrets/pg_password、config/config.yaml
# 编辑 .env 中的 LP_BASE_URL；使用 Tunnel 时把 token 写入 secrets/tunnel_token
# 可选：cp config/seed.example.yaml config/seed.yaml，修改后在 config/config.yaml 中设置
#   seed_file: /etc/linkspage/seed.yaml（只在首次启动、页面为空时导入一次）
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

端到端测试（Playwright + axe）在离线的 Docker Compose 栈中运行，Discord / KOOK 由 stub 回放录制的响应：

```sh
docker build -t linkspage:e2e . && e2e/run.sh
```

## 声明

本项目与 Discord、KOOK、腾讯无关，品牌图标归各自所有者。

## License

[MIT](LICENSE)
