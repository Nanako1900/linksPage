# M1 公开页 MVP：交付说明

> 日期：2026-10-05　分支：`feat/m1-public-page`　范围：`docs/tech-selection.md` 第 14 节 M1。
> 接口细节以 [contract.md](contract.md) 为准；Provider 实测结论见 [../spikes/providers.md](../spikes/providers.md)。

## 1. M1 交付了什么

| 方面 | 内容 |
|---|---|
| Provider | Discord（widget.json + 永久邀请 `invites?with_counts=true`）；KOOK 免 token 模式（徽章 302 的 Location 解析，style 0 取名称、style 2 取在线/总数）；static 卡片；QQ 群卡片（群号复制 + 可选二维码）；微信群二维码卡片（内联 PNG + 兜底联系方式） |
| 平台预设 | discord、kook、telegram、qq-group、qq-channel、wechat-group、bilibili、steam-group、teamspeak、matrix、revolt、link；需要外部浏览器的是 discord、telegram、matrix、revolt。可在 seed 中追加自定义平台（一律 static 卡片） |
| 卡片状态机 | `pending / live / stale / degraded / static / qr-only / unavailable` 七种状态，规则见契约第 6 节。瞬时错误保留上次数据（含内存中的成员列表）并标为 stale，退避上限 1 小时；live / degraded 超过 3 个刷新周期（至少 15 分钟）没有成功拉取时，页面重建（每分钟）也会显示为 stale（leader 挂掉或任务卡住时不再永远显示 live）。Widget 的临时邀请（instant invite）按首次看到的时间 + 23 小时视为过期（Discord 实测为 24 小时），stale 期间过期后加入按钮和 `/go` 改用 fallback_url 或显示不可用 |
| `/go/{slug}` | 只解析**当前已发布**的社区和链接（有可见、在显示时段内的块；隐藏、定时未开始、已过期或没放进任何块的 slug 与未知 slug 一样返回 404 页，不泄露群号、二维码或链接地址）；目标解析（永久邀请 → 未过期的 instant invite → fallback_url）；微信 / QQ 内对 Discord 等平台返回「在浏览器打开」引导页（200，不 302）；微信内的 QQ 群返回群号页；无目标时返回「邀请暂不可用」页，**永不 302 到已知失效的链接**，也永不返回 429。M1 不计数，`golink.NoopClickHook` 是给 M3 留的钩子 |
| 前端 | Signal Paper 预设（含 focus-visible、active、入场动画和 reduced-motion）；「在电脑上打开」弹层（手机上的 Discord 卡片）；微信 / QQ 内的引导遮罩；Discord iframe facade（点击才加载）；60 秒可见时轮询 `/api/v1/public/live`（ETag/304；revision 变化后重新加载 bootstrap，失败时不保存 ETag，下一轮重试）。Markdown 子集的服务端（无 JS 兜底）和前端渲染由共享语料 `web/src/test/fixtures/markdown-cases.json` 钉成一致：段落内的换行就是换行，不支持自动链接 `<https://…>` 和引用式链接；微信 / QQ / 手机 UA 判断由共享语料 `web/src/test/fixtures/user-agents.json` 钉住 Go 与 TS 两份实现 |
| 服务端渲染 | 兜底 markup + 关键 CSS（无 JS 也能看到内容）；hash CSP；OG、favicon、manifest、robots 自动生成；`/c/{slug}` 单社区分享页；404 页；zh-CN 和 en，服务端按 `?lang=` → `Accept-Language` → 默认语言 |
| 后台任务 | leader 选举的 scheduler：provider 刷新（30 秒 tick，Discord widget 5 分钟、invite 15 分钟）、定时显示边界检查、每分钟重建页面 |
| 图片 | `/media/u`（上传与生成的图片，内容寻址）；`/media/p` 不透明 key 外链代理（只允许登记过的 Discord CDN 地址）；`/media/q` 二维码（替换后 410） |
| 内容来源 | `seed.yaml`（首次启动、页面为空时导入一次；M2a 之前没有后台编辑） |
| 拓扑 C | `deploy/cloudflare-worker/`：Workers Static Assets + HTMLRewriter 注入源站 `/api/v1/public/render` 的结果，见该目录 README |
| 门禁 | Playwright E2E（含 axe 和 CSP violation 监听）、size-limit、stylelint 浏览器兼容检查，全部接入 CI，见第 5 节 |

## 2. 配置 `seed.yaml`

完整、可直接解析的例子：[`config/seed.example.yaml`](../../config/seed.example.yaml)；字段说明见契约第 10 节。

1. 复制示例并改成自己的内容，图片放在它旁边的目录里：

   ```sh
   cp config/seed.example.yaml config/seed.yaml
   mkdir -p config/images     # 头像、二维码、自定义图标（png / jpg / webp，不支持 SVG）
   ```

2. 在 `config/config.yaml` 中指向容器内的路径（`./config` 挂载在 `/etc/linkspage`），或在 compose 的 `environment` 中设置 `LP_SEED_FILE`：

   ```yaml
   seed_file: /etc/linkspage/seed.yaml
   ```

3. 先校验再启动：`docker compose run --rm --no-deps app config check`，然后 `docker compose up -d`。导入结果在日志 `seed: imported` 中（社区、链接、块的数量）。

要点：

- **只在页面没有任何块、社区和链接时导入一次**，之后的启动直接跳过。要重新导入，只能清空这三张表（M2a 之后改用后台）。
- 未知键、格式错误都会让启动失败，错误信息带 YAML 路径（如 `communities[2].guild_id`）。
- 服务器 ID、QQ 群号**必须加引号**，否则 YAML 会当成数字。
- Discord：服务器设置 → 小部件（Widget）必须开启，否则卡片降级为 static；`invite` 填永久邀请（永不过期、不限次数）。Widget 刚开启时，Discord 的错误响应带 `max-age=300`，最多要等 5 分钟才能拿到数据。
- KOOK：服务器必须设为公开；没有 Bot Token 时拿不到图标，可以用 `icon` 自己上传。
- 微信群二维码 7 天过期，过期后替换图片（M1 只能改 seed 并清库重导，M2a 起在后台替换）。
- 大陆服务器访问 Discord 需要设置 `LP_PROVIDERS__HTTP_PROXY`。

## 3. 拓扑 C 部署摘要

详细步骤见 [`deploy/cloudflare-worker/README.zh-CN.md`](../../deploy/cloudflare-worker/README.zh-CN.md)，这里只列要点：

1. 源站先按拓扑 A（橙云直连）或 B（Tunnel）接入，**与 Worker 使用同一个主机名**。
2. 生成共享密钥（≥32 字节），两端使用同一个值：Worker 端 `wrangler secret put LP_PROXY_AUTH`，源站端 `LP_EDGE__PROXY_AUTH_FILE=/run/secrets/edge_proxy_auth`。
3. Workers Routes：`主机名/*` 指向 `linkspage-edge`；`/api/*`、`/go/*`、`/media/*`、`/healthz`、`/readyz`、`/favicon.ico`、`/robots.txt`、`/site.webmanifest`、`/admin*` 设为 **None**。`/c/*` **不要**设为 None（由 Worker 渲染，与 tech-selection 12.3 的旧写法不同）。
4. 把**同一个 release tag** 的 `web-dist.tar.gz`（release 附件，附带 `SHA256SUMS`）解压到 `web/dist`，再 `pnpm check:deploy` → `pnpm run deploy`。
5. 不要使用 `*.workers.dev`（大陆不可访问）；不要对 HTML 配置 Cache Everything。Worker 自己按 path + 语言缓存渲染结果 60 秒。

## 4. 已知限制

| 限制 | 说明 / 后续 |
|---|---|
| 多实例时只有 leader 有成员列表 | 在线成员列表保存在 leader 进程的内存中（`provider.LiveStore`），其他实例的页面没有头像列表。单实例部署（默认 compose）不受影响 |
| `/api/v1/public/render` 未配置密钥时公开可访问 | 契约规定的行为：没有配置 `edge.proxy_auth` 时不鉴权（返回的内容与公开页相同）。带正确密钥的请求不按 IP 限流，而是共用一个每分钟 1200 次的边缘桶（超出时 Worker 回退到静态壳）。Worker 只在 https（或本机 `wrangler dev`）上发送密钥，`LP_ORIGIN` 不接受非本机的 http 地址 |
| `/go`、`/media/u`、`/media/q` 的数据库开销 | 这些公开路由每个请求查一次数据库，没有负缓存（`/go` 按设计不能返回 429）；大量随机 slug / key 会占用连接池（默认 10 个连接）。M2a 计划让 `/go` 改读内存快照（服务端 slug 索引），媒体路由加未命中缓存和并发上限 |
| Android 微信 XWeb 内核版本 | 仍是 **UNVERIFIED**。浏览器基线按 Chrome ≥ 99 / Safari ≥ 15.4（`web/.browserslistrc`），更旧的 WebView 依靠兜底 markup |
| E2E 只跑 Chromium | 微信 / QQ 用 UA 模拟（逻辑判断只看 UA）；真实 WebView、WebKit 和 Firefox 未覆盖。130% 字号用 Chromium 的默认字号设置模拟 |
| 图片代理失败不做负缓存 | 上游网络错误（不是 404/410）不缓存，每次请求都会再试一次（受并发信号量限制） |
| 限流规则未验证 | Discord invite 接口和 KOOK 徽章接口的限流规则都是 **UNVERIFIED**，按每 5 / 15 分钟拉一次控制 |
| 内容只能通过 seed 修改 | M2a 提供后台 |
| `/go` 不计数 | M3 用统计实现替换 `NoopClickHook` |

## 5. 质量门禁

| 门禁 | 位置 | 命令 |
|---|---|---|
| E2E（Playwright 1.63 + axe） | `e2e/`，CI job `e2e` | `docker build -t linkspage:e2e . && e2e/run.sh` |
| 公开入口体积 | `web/.size-limit.js`，CI job `web` | `pnpm size`（JS ≤ 90 kB gz、CSS ≤ 15 kB gz，含懒加载 chunk 的 JS ≤ 110 kB gz） |
| CSS 浏览器兼容 | `web/stylelint.config.mjs`、`web/.browserslistrc` | `pnpm lint:css`（源码与 Go 端样式）、`pnpm lint:css:dist`（构建产物） |
| Worker | `deploy/cloudflare-worker/`，CI job `worker` | lint、typecheck、测试覆盖率、`wrangler deploy --dry-run` |

E2E 的设计：

- **完全离线**：compose 栈（`e2e/compose.yaml`）的所有服务都在 `internal: true` 网络里，测试不可能访问真实的 Discord / KOOK。`providers.*.api_base` 指向 stub（`e2e/stub/`），stub 按 ID 回放 `internal/provider/*/testdata` 中的录制响应；`stub.spec.ts` 校验 app 确实请求了每个 fixture。
- 内容来自固定的 `e2e/seed/seed.yaml`：live（Discord、KOOK）、static（Widget 未开启）、unavailable（服务器不存在）、QQ 群、微信群各一个。
- 测试容器与 app 共享网络命名空间，访问 `http://localhost:8080`，因此页面是安全上下文，可以读回剪贴板内容。
- 每个用例都自动监听 `securitypolicyviolation`（包括 report-only）、控制台错误和未捕获异常。另有一个对照用例主动插入内联样式，证明监听有效。
- 304：Chromium 把重新验证的文档报告为 200，所以用 Navigation Timing 判断（重新加载时 `transferSize` 只有响应头大小）。路由拦截会关闭 HTTP 缓存，所以 CSP 用例不拦截 `/media/p/`。
- 覆盖：首次加载与 304 重新加载均无 CSP 违规；axe 在 375 / 1440 px、浅色 / 深色下无 serious / critical 问题（已知问题除外）；微信 / QQ UA 点击 Discord「加入」出现引导遮罩，`/go/discord` 返回引导页（不 302）；手机 UA 的「在电脑上打开」弹层与复制；QQ 群号复制；`/c/{slug}` 置顶；404 页；320 px + 130% 字号无横向溢出。

本地调试：`docker compose -f e2e/compose.yaml -f e2e/compose.debug.yaml up -d app` 会把 app 暴露在 `127.0.0.1:8080`、stub 暴露在 `127.0.0.1:8090`（这个叠加文件会接入外网，CI 不使用）。

## 6. M2a 跟进

- 后台：站点信息、内容块、社区、链接、二维码、上传的增删改查，替代 seed 的一次性导入；settings bundle 导入导出。
- 本地账号登录、`hash-password` CLI、会话与每请求重校验、CrossOriginProtection、限流与退避、审计、角色。
- Webhook 通知框架（Discord 10006 邀请失效、二维码将过期等事件的出口）。
- `/go` 改读内存快照，媒体路由加未命中缓存和并发上限（见第 4 节「数据库开销」）。
- 多实例的成员列表：把 `LiveStore` 的数据写入数据库或在实例间同步。
- E2E 扩展到后台流程（13.3 节：配置管理员 → 登录 → 添加社区 → 公开页；删除管理员或改哈希后旧会话失效），以及 WebKit。
- M0 遗留：`make web-dist` 和 Dockerfile 只复制 `assets/` 和 `.vite/`；Dockerfile / compose 的基础镜像改为 digest 固定（Renovate `docker:pinDigests`）；首次发布 v0.1.0 时验证 release.yml 并把 GHCR 包设为 Public。
