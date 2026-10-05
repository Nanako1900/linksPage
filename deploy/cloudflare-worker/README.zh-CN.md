# LinksPage 边缘前端 Worker（拓扑 C）

本目录是 `docs/tech-selection.md` 第 2 节「拓扑 C」和 12.3 节的 Worker 实现：前端（`web/dist`）托管在 Cloudflare Workers Static Assets，公开 HTML 由 Worker 用 HTMLRewriter 注入源站渲染结果；API、跳转、图片仍然直接回源。

## 1. 请求如何流转

| 请求 | 谁处理 | 说明 |
|---|---|---|
| `/`、`/privacy`、`/c/{slug}` | Worker（`run_worker_first`） | 取静态 `index.html` + 源站 `GET /api/v1/public/render`，改写后返回 |
| `/assets/*` 等静态文件 | Static Assets（不经过 Worker，免费不限量） | 不存在的 `/assets/*`、`/fonts/*`、`/ext/*` 由 Worker 返回纯文本 404（`no-store`） |
| 其他未知路径（如 `/foo`） | Worker | 渲染可定制的 404 页（状态码 404） |
| `/api/*`、`/go/*`、`/media/*`、`/healthz`、`/readyz`、`/favicon.ico`、`/robots.txt`、`/site.webmanifest`、`/admin*` | **None 路由，直接回源** | 万一请求仍然到达 Worker（例如还没配 None 路由），Worker 会原样转发给源站，不跟随重定向（`/go` 的 302 原样交给访客） |

### 改写步骤（契约第 9 节）

1. `<html lang data-appearance>` 设为渲染结果的 `lang`、`appearance`；
2. 删掉静态壳中的 `<title>` 和 `<meta name="description">`，把 `head` 片段追加到 `<head>` 末尾；
3. 用 `fallback` 替换 `#root` 的内容；
4. `data` 非 null 时在 `<body>` 末尾追加 `<script id="lp-data" type="application/json">{"data":…}</script>`，其中 `<`、`>`、`&`、U+2028、U+2029 转义为 `\uXXXX`；
5. 响应头：渲染结果给出的 `Content-Security-Policy` / `Content-Security-Policy-Report-Only`、`Cache-Control: no-cache, no-transform`、`Vary: Accept-Language`、`X-Content-Type-Options`、`Referrer-Policy`、`Permissions-Policy`、`Cross-Origin-Opener-Policy`。状态码取渲染结果的 `status`（200/404）。
6. 状态码 200 时带弱 ETag（源站 ETag + 静态壳 ETag），`If-None-Match` 命中返回 304。

### 缓存

- 渲染结果放在 `caches.default` 中 **60 秒**，键为「源站 + path + `?lang=` + 规范化后的 Accept-Language」。规范化：最多取 q 值最高的 3 个语言范围、小写、丢弃非法项和 `q=0`；转发给源站的也是这个规范化值，因此缓存内容与源站看到的请求一致。
- 不可能是页面的路径（如 `/wp-login.php`）统一用 `path=/404` 请求源站，只占一个缓存项。
- 源站 settings 的 `version` 只有拿到渲染结果后才知道，无法放进缓存键；内容更新最多延迟 60 秒（每个 Cloudflare 机房各自缓存）。

### 源站不可用时

源站超时（默认 4 秒）、返回非 200、响应不是合法 JSON 或校验失败时，Worker 返回**原样的静态壳**：状态码 200（明显不是页面的路径为 404），使用通用 CSP（只允许同源脚本和样式，`frame-src https://discord.com`），不带 `#lp-data`。前端找不到 `#lp-data` 时会自己请求 `/api/v1/public/bootstrap`。Workers Logs 中记录 `render_unavailable` 事件和原因。

## 2. 前置条件

- 域名已接入 Cloudflare，且源站已按拓扑 A（橙云直连）或 B（Tunnel）接入**同一个主机名**（例如 `links.example.com`）。Worker 用路由（Route）挂在这个主机名上，**不要**用 Custom Domain（那样 Worker 本身就是源站，回源请求会打到自己）。
- **不要使用 `*.workers.dev`**，大陆无法访问；`wrangler.jsonc` 已设置 `workers_dev: false`、`preview_urls: false`。
- Node.js ≥ 22.18，`corepack enable`（pnpm 版本由 `package.json` 的 `packageManager` 固定）。
- 前端产物：在 `web/` 执行 `pnpm install --frozen-lockfile && pnpm build`，或把同一个 release tag 的 `web-dist.tar.gz` 解压到 `web/dist`。**镜像和 Worker 必须来自同一个 tag。** 使用 release 附件时先校验来源（release 流水线为附件生成了 build provenance 证明，`SHA256SUMS` 只能发现传输损坏）：

  ```sh
  gh attestation verify web-dist.tar.gz --repo Nanako1900/linksPage
  ```

- `web/dist/.assetsignore`（来自 `web/public/`，构建时自动复制）让 Static Assets 不上传 `.vite/`（Vite manifest，只给 Go 源站用）和 `admin/`（后台壳，始终回源）。`assets/` 下的后台 chunk 仍会上传，源站返回的后台页面要从边缘加载它们。

## 3. 配置

### 3.1 `wrangler.jsonc`

| 键 | 说明 |
|---|---|
| `assets.directory` | `../../web/dist` |
| `assets.run_worker_first` | `["/", "/privacy", "/c/*", "/admin", "/admin/*"]`。只有公开 HTML 先进 Worker；`/admin` 也列进来，是为了不让边缘返回 `web/dist/admin` 里那份过期的后台壳（后台应走 None 路由回源） |
| `vars.LP_ORIGIN` | 留空 = 访客访问的主机名（推荐，依赖 None 路由回源）。也可以填单独的回源地址，如 `https://origin.example.com`（必须无路径、无查询）。**必须是 https**：渲染子请求携带共享密钥，明文 `http://` 只允许 `localhost`、`127.0.0.1`、`[::1]`（本地 `wrangler dev`），其他 http 地址视为配置非法。即使配置合法，渲染地址不是 https（例如留空时访客用 http 访问）时 Worker 也不发送密钥 |
| `vars.LP_RENDER_TIMEOUT_MS` | 渲染请求超时，250–15000，默认 4000 |
| `routes` | 按需添加，例如 `"routes": [{ "pattern": "links.example.com/*", "zone_name": "example.com" }]`；也可以在控制台配置 |
| `env.test` | 只给测试用（指向 `test/fixtures/assets`），部署时用 `--env=""` 选择顶层配置 |

配置非法（例如 `LP_ORIGIN` 带路径、`LP_PROXY_AUTH` 太短）时 Worker 对所有请求返回 500 并记录 `config_invalid`，部署后立刻能发现。

### 3.2 共享密钥 `LP_PROXY_AUTH`

源站配置了 `edge.proxy_auth` 时，`/api/v1/public/render` 要求请求头 `X-LP-Proxy-Auth` 与之相等，否则 403（此时 Worker 会走「源站不可用」的回退）。带正确密钥的渲染请求不受每 IP 限流（边缘出口 IP 是共享的），而是所有边缘请求共用源站上一个每分钟 1200 次的桶，防止大量不同的 `?lang=` / 路径绕过 Worker 缓存后无限回源；超出时源站返回 429，Worker 同样回退到静态壳，由前端自己加载数据。

```sh
# 生成一次，两边使用同一个值（≥32 字节、可打印 ASCII、无空格）
openssl rand -hex 32 > secrets/edge_proxy_auth

# Worker 端
pnpm exec wrangler secret put LP_PROXY_AUTH < secrets/edge_proxy_auth

# 源站端（compose 中挂载为 secret 文件）
LP_EDGE__PROXY_AUTH_FILE=/run/secrets/edge_proxy_auth
```

本地 `wrangler dev` 可以把它写进 `.dev.vars`（已在 `.gitignore` 中）。

### 3.3 Workers Routes（关键）

在 Cloudflare 控制台 → 域名 → Workers Routes 中添加（`links.example.com` 换成你的主机名）：

| 路由 | Worker |
|---|---|
| `links.example.com/*` | `linkspage-edge` |
| `links.example.com/api/*` | **None** |
| `links.example.com/go/*` | **None** |
| `links.example.com/media/*` | **None** |
| `links.example.com/healthz` | **None** |
| `links.example.com/readyz` | **None** |
| `links.example.com/favicon.ico` | **None** |
| `links.example.com/robots.txt` | **None** |
| `links.example.com/site.webmanifest` | **None** |
| `links.example.com/admin*` | **None** |

- Cloudflare 按「最具体的路由」匹配，所以上面的 None 路由会优先于 `/*`。None 路由的请求不调用 Worker、不计入 Worker 额度，并且真实访客 IP 不受影响。
- **`/c/*` 不要设成 None 路由。** 这与 `tech-selection.md` 12.3 的旧写法不同：契约（`docs/m1/contract.md` 第 9 节）规定 `/c/{slug}` 也由 Worker 渲染。
- 用 API 创建 None 路由（不带 `script` 字段即为 None）：

  ```sh
  curl -X POST "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/workers/routes" \
    -H "Authorization: Bearer $CF_API_TOKEN" -H "Content-Type: application/json" \
    --data '{"pattern":"links.example.com/api/*"}'
  ```

- `LP_ORIGIN` 留空时，Worker 对同主机名的子请求会命中 None 路由直接回源。如果配错导致子请求又回到 Worker，Worker 会识别自己加的 `X-LP-Edge-Hop` 头并返回 **508 Loop Detected**（日志事件 `edge_loop`），而不是无限循环。

### 3.4 其他 Cloudflare 设置

- 继续遵守 12.3 的通用检查清单：**永远不要**对 HTML 配置 Cache Everything；`/media/*` 缓存规则照旧。
- 12.3 中「可选：`/api/v1/public/*` 可缓存」的规则**不要覆盖 `/api/v1/public/render`**：源站按 `Accept-Language` 选择语言，而 CF 默认缓存键不含该头；Worker 自己已经缓存 60 秒。
- 免费套餐每天 10 万次 Worker 请求（约 10 万 PV）；超额后 `run_worker_first` 匹配的请求会直接返回 429。
- 静态资源的缓存头由 Static Assets 决定（不经过 Worker）；如需 `/assets/*` 一年 `immutable`，由前端在 `web/public/_headers` 中声明。

## 4. 部署

```sh
cd deploy/cloudflare-worker
pnpm install --frozen-lockfile
pnpm check:deploy        # wrangler deploy --dry-run，不需要凭据，检查打包和 assets 目录
export CLOUDFLARE_API_TOKEN=...   # 需要 Workers Scripts: Edit 权限
export CLOUDFLARE_ACCOUNT_ID=...
pnpm run deploy          # wrangler deploy --env=""
```

部署后检查：

```sh
curl -sI https://links.example.com/ | grep -iE 'content-security-policy|cache-control|vary|etag'
# CSP 中应出现 sha256-… 哈希（不是通用回退 CSP）
curl -s https://links.example.com/ | grep -c 'id="lp-data"'     # 应为 1
curl -sI https://links.example.com/api/v1/public/live            # 来自源站（None 路由）
curl -sI https://links.example.com/go/<slug>                     # 302 或引导页，Cache-Control: no-store
```

Workers Logs（已开启 `observability`）中的错误事件：`render_unavailable`、`shell_unavailable`、`origin_unreachable`、`config_invalid`、`edge_loop`。

## 5. 开发与测试

```sh
pnpm test             # vitest + @cloudflare/vitest-pool-workers（workerd 中运行）
pnpm test:coverage    # istanbul 覆盖率，阈值见 vitest.config.ts
pnpm typecheck
pnpm lint
pnpm dev              # wrangler dev（顶层配置，需要先构建 web/dist）
```

本地联调真实源站：`pnpm exec wrangler dev --env test --var LP_ORIGIN:http://127.0.0.1:8080`（源站需未配置 `edge.proxy_auth`，或在 `.dev.vars` 中提供密钥）。

测试用的静态壳在 `test/fixtures/assets/`，PublicPage 样例直接引用契约文件 `web/src/test/fixtures/public-page.json`。

版本说明：`@cloudflare/vitest-pool-workers@0.22.0` 只支持 vitest 4（peer `^4.1.0`），因此这里固定 vitest 4.1.11，而 `web/` 使用 vitest 5。`compatibility_date` 不能晚于 pool 自带的 workerd（1.20260815）。
