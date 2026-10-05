# M1 契约：公开页 MVP

> 日期：2026-10-05　分支：`feat/m1-public-page`　范围：`docs/tech-selection.md` 第 14 节 M1。
> 本文固定 M1 所有并行构建者共用的接口。**与 tech-selection.md 冲突时以本文为准**（偏差见第 15 节）；与 `docs/spikes/providers.md` 的实测结论冲突时以实测为准。
> 契约的机器可读部分：Go 类型（`internal/site`、`internal/provider` 等骨架包）、TS 镜像 `web/src/shared/types/public.ts`、样例 `web/src/test/fixtures/public-page.json`。三者由测试互相钉住：
> - `internal/site/contract_test.go`：样例严格解码（`DisallowUnknownFields`）、`PublicPage.Validate()` 通过、重新编码后与样例逐字段相等。
> - `web/src/shared/types/guards.test.ts`：样例通过 `isPublicPage`。
>
> 需要修改契约文件时，在报告里写出具体改动，由契约负责人统一修改。

---

## 1. 文件归属

| 构建者 | 拥有（可自由修改） | 说明 |
|---|---|---|
| **content** | `internal/site/**`、`internal/seed/**`、`internal/content/**`、`config/seed.example.yaml`；`internal/config/**` 中只限 seed 相关的键（目前没有遗留项） | 页面组装（`site.Builder`）、seed 导入、Markdown/URL/文本清洗。`internal/site` 中的 DTO 类型、`Validate()` 和 `contract_test.go` 属于契约：改字段需走契约变更 |
| **providers** | `internal/provider/**`（含 `platforms.yaml`、`discord/`、`kook/`）、`internal/jobs/**`、`internal/imgproxy/**` | `provider.JoinTarget`、`provider.State`、`Snapshot` 的 JSON 形状属于契约 |
| **media** | `internal/media/**` | |
| **golink** | `internal/golink/**`、`internal/uaclass/**` | |
| **webui** | `internal/webui/**` | 含模板、关键 CSS、boot 脚本 |
| **frontend** | `web/**`，**除了** `web/src/shared/types/public.ts` 和 `web/src/test/fixtures/public-page.json`（契约所有，可提变更请求） | `web/src/shared/types/guards.ts` 是契约给出的初版，归 frontend 维护 |
| **worker** | `deploy/cloudflare-worker/**` | CI 中 Worker 相关 job 的改动写进报告，由集成者合入 `.github/workflows/ci.yml` |
| **集成者** | `cmd/linkspage/**`、`internal/httpapi/**`（含 `router.go`、`api.go`）、`internal/store/**`（迁移、查询、生成代码）、`go.mod`/`go.sum`、`web/openapi.json`、`web/src/shared/api/gen/**`、`.github/**`、`Makefile`、`Dockerfile`、`compose*.yaml` | 构建者**不得**修改 `cmd/linkspage` 和 `internal/httpapi/router.go`；需要的接线写进报告 |

其他约定：
- 依赖已全部预先加入 `go.mod`（见第 14 节）。构建者**不要**执行 `go get`；确实需要新依赖时写进报告。
- 需要新的 SQL 查询时，在报告里给出完整的 `-- name:` 查询文本，由集成者加入 `internal/store/queries/` 并重新生成。
- 骨架函数返回各包的 `ErrNotImplemented`；实现时删掉该返回即可，签名不要改。确实需要改签名时写进报告。
- 骨架阶段 golangci-lint 会报 16 个 `unused`（骨架里尚未使用的字段和 `runSafe`），实现后自然消失。

---

## 2. 集成者接线清单

### 2.1 `cmd/linkspage/cmd_serve.go`

按顺序构造（全部依赖 `cfg`、`logger`、`pool`）：

1. `mediaKey, _ := cfg.DeriveKey(config.KeyInfoMediaProxy, 32)`
2. `client, _ := provider.NewHTTPClient(provider.ClientOptions{ProxyURL: cfg.Providers.HTTPProxy.Reveal(), UserAgent: provider.UserAgent(version)})`
3. `q := dbq.New(pool)`
4. `registrar` 依赖 registry 的 ImageHosts，而 Discord provider 依赖 registrar，所以先用 `discord.ImageHosts()` 构造：
   `registrar, _ := imgproxy.NewRegistrar(q, mediaKey, map[string]map[string][]string{"discord": discord.ImageHosts(), "kook": {}})`
5. `dp, _ := discord.NewProvider(discord.Options{APIBase: cfg.Providers.Discord.APIBase, Client: client, Images: registrar})`
   `kp, _ := kook.NewProvider(kook.Options{APIBase: cfg.Providers.KOOK.APIBase, Client: client})`
   `registry, _ := provider.NewRegistry(dp, kp)`
6. `live := provider.NewLiveStore()`；`catalog, _ := provider.LoadPresets()`（自定义平台由 Builder 每次重建时读取并 `WithCustom`）
7. `sem := media.NewSemaphore(media.MemoryBudget)`；`mstore, _ := media.NewLocalStore(filepath.Join(cfg.DataDir, media.UploadsDir))`；`proc := media.NewProcessor(mstore, sem)`；`gen := media.NewGenerator(mstore, sem)`
8. `builder, _ := site.NewBuilder(site.BuilderDeps{Queries: q, Live: live, Platforms: catalog, Assets: gen, BaseURL: cfg.BaseURL, Logger: logger})`
9. `importer, _ := seed.NewImporter(seed.Deps{DB: pool, Media: proc, Logger: logger, MaxUploadBytes: cfg.Uploads.MaxBytes})`
10. 启动步骤（`dbStartupSteps`）在迁移之后依次加入：
    - `cfg.SeedFile != ""` 时 `importer.Import(ctx, cfg.SeedFile)`；`errors.Is(err, seed.ErrInvalidSeed)` → `errFatalStartup`，其他错误按瞬时错误重试。
    - 用 `builder.Rebuild(ctx, holder)` 取代 `site.LoadSnapshot`（`LoadSnapshot` 保留给测试或删除，由集成者决定）。
11. Jobs：
    ```go
    refresh, _ := jobs.NewRefreshJob(jobs.RefreshDeps{Store: q, Registry: registry, Live: live,
        OnChange: func(ctx context.Context) { _ = builder.Rebuild(ctx, holder) }, Logger: logger})
    sched, _ := jobs.NewScheduler(jobs.NewPGLeaderLock(pool, jobs.LeaderLockKey), logger,
        refresh, // LeaderOnly, Interval jobs.RefreshTick
        jobs.Job{Name: "page-boundary", Interval: 5 * time.Second, Run: func(ctx context.Context) error { return builder.RebuildIfDue(ctx, holder) }},
        jobs.Job{Name: "page-rebuild", Interval: time.Minute, Run: func(ctx context.Context) error { return builder.Rebuild(ctx, holder) }},
    )
    ```
    `sched.Run(runCtx)` 在 readiness 变为 ready 后用 goroutine 启动；关闭步骤在 `startup` 之后、`database pool` 之前插入 `{Name: "scheduler", ...}`（取消并等待 `Run` 返回）。
12. Handler：
    - `goHandler, _ := golink.NewHandler(golink.HandlerOptions{Resolver: golink.NewResolver(<golink 提供的 DB Source>, nil), Snapshot: holder.Current, BaseURL: cfg.BaseURL, Hook: golink.NoopClickHook{}, Logger: logger})`
    - `proxyHandler, _ := imgproxy.NewHandler(imgproxy.HandlerOptions{Store: q, Client: client, Hosts: imageHosts(), Logger: logger})`
    - `media.UploadsHandler(mstore, q, logger)`、`media.QRHandler(mstore, q, logger)`
    - `media.SiteFilesHandler(media.FileFavicon|FileManifest|FileRobots, func() *media.SiteFiles { return holder.Current().Files })`
    - 全部传给 `httpapi.Deps`（新增字段）。

### 2.2 `internal/httpapi`

`router.go` 第 2、3 步（在 `/assets/*` 之后、`/healthz` 之前）挂载：

```go
r.Get("/favicon.ico", d.Favicon.ServeHTTP)
r.Get("/robots.txt", d.Robots.ServeHTTP)
r.Get("/site.webmanifest", d.Manifest.ServeHTTP)
r.Get("/go/{slug}", d.GoLink.ServeHTTP)
r.Get("/media/u/{key}", d.Uploads.ServeHTTP)
r.Get("/media/p/{file}", d.ImgProxy.ServeHTTP)
r.Get("/media/q/{id}", d.QR.ServeHTTP)
```

- `/go/*`、`/media/*` 加入 4.12 的「公开路由」极简 access log；`/go` 不加任何会返回 429 的限流。
- Huma 操作（`api.go`）：
  - `getPublicBootstrap`：输出改为 `{"data": site.PublicPage}`（`holder.Current().Public`），`Cache-Control: no-cache`。
  - 新增 `getPublicLive`：`GET /api/v1/public/live` → `{"data": site.LiveDTO}`（`Public.Live()`）；强 ETag = `"` + hex(sha256(JSON(LiveDTO 去掉 generatedAt)))[:32] + `"`；`If-None-Match` 命中返回 304；`Cache-Control: public, max-age=0, s-maxage=30`。
  - 新增 `getPublicRender`：`GET /api/v1/public/render?path=&lang=` → `{"data": site.RenderDTO}`（`webui.Renderer.RenderDTO`）；配置了 `edge.proxy_auth` 时要求 `X-LP-Proxy-Auth` 常量时间比较相等，否则 problem+json 403 `code=forbidden`；`Cache-Control: public, max-age=0, s-maxage=60`。始终带 `Vary: Accept-Language`。限流：带正确 `X-LP-Proxy-Auth` 的边缘请求共用一个每分钟 1200 次的桶（`EdgeRenderLimit`），其余请求按公开读 API 的每 IP 限额；桶满时 Worker 回退到静态壳。
  - 公开读 API 限流 120/min/IP（4.11）由集成者加。
- 重新生成：`go run ./cmd/linkspage openapi > web/openapi.json && cd web && pnpm gen:api`。前端公开页**不使用**生成的类型，改用 `public.ts`。

---

## 3. URL 格式

| 形式 | 说明 | 缓存 |
|---|---|---|
| `/media/u/{key}` | 上传和生成的图片。`key = hex(sha256(文件字节)[:16]) + "." + (webp\|png\|jpg)`，正则 `^[0-9a-f]{32}\.(webp\|png\|jpg)$` | `public, max-age=31536000, immutable` |
| `/media/p/{key}.{ext}` | 外部图片代理。`key = base64url(HMAC-SHA256(K_media, 规范化 URL))[:22]`，正则 `^[A-Za-z0-9_-]{22}$`；`ext ∈ png\|jpg\|webp\|gif`，须与登记时一致，否则 404 | 图标/横幅 `public, max-age=86400, stale-while-revalidate=604800`；头像 `public, max-age=3600` |
| `/media/q/{uuid}` | 二维码。格式错误 404；格式正确但不存在（已替换/删除）→ **410** | 200 与 410 都是 `public, max-age=300` |
| `/go/{slug}` | 加入/外链跳转，slug 与 `/c/{slug}` 同正则 `^[a-z0-9][a-z0-9-]{0,63}$`。社区和链接共用一个命名空间（DB 触发器保证） | `no-store` |
| `/c/{slug}` | 单社区分享页（HTML） | HTML 规则 |
| `/favicon.ico` | 32px PNG（`image/png`） | `public, max-age=3600` + ETag |
| `/site.webmanifest`、`/robots.txt` | 内存生成 | 同上 |
| `https://discord.com/widget?id={guildId}` | `EmbedView.src`，前端追加 `&theme=light\|dark` | — |

DTO 中除 `site.baseUrl`、`links[].url`、`communities[].inviteUrl`、`embed.src` 外全部是以 `/` 开头的相对路径。

---

## 4. 数据库（`internal/store/migrations/00002_m1.sql`，只做 expand）

| 表 | 要点 |
|---|---|
| `media` | `key` PK（见上）；`kind ∈ avatar,icon,qr,og,favicon,background`；`content_type` 与扩展名必须一致；宽高 1–4096；`variants` 为对象（格式 → 同级行的 key） |
| `media_proxy` | `key` 22 位；`provider ∈ discord,kook`；`url` 必须 https、≤2048；`ext`；`kind ∈ icon,banner,splash,avatar`；`last_seen_at` 有索引（回收用） |
| `custom_platforms` | `id` 正则 `^[a-z][a-z0-9-]{1,31}$`；`name` 非空对象；`icon`（`si:`/`builtin:`）与 `icon_key`（FK media，SET NULL）二选一 |
| `communities` | `slug` 唯一；`provider ∈ discord,kook,static`；discord 的 `external_id` 17–20 位数字，kook 1–20 位，static 必须为 NULL；`display` 必须含非空 `name` 对象；`icon_key` FK media（SET NULL）；`invite_url` 必须 https；`fallback_url` 必须 http(s)；`refresh_interval ≥ 1 min` |
| `community_qr_codes` | 每社区一行（`community_id` UNIQUE，CASCADE）；`media_key` FK media（**RESTRICT**，SQLSTATE 23001） |
| `provider_snapshots` | PK=`community_id`（CASCADE）；`data` 为对象且**不得含 `users` 键**；`state` 七值之一；`err_code` `^[a-z0-9_]{1,64}$`；`next_fetch_at` 有索引 |
| `links` | `slug` 唯一；`kind ∈ link,social`；`url` 以 `http://`、`https://` 或 `mailto:` 开头；`icon`/`icon_key` 二选一；`rel_me` |
| `page_blocks` | `kind` 五值；**`community_id` / `link_id` 两个真实 FK（CASCADE）**取代文档里的多态 `ref_id`，CHECK 保证只有对应 kind 设置；`data` 为对象；`visible_from < visible_to` |
| 触发器 `lp_check_go_slug` | `communities.slug` 与 `links.slug` 不得重复（`unique_violation` 23505），用事务级 advisory lock 串行化 |

### 4.1 jsonb 形状（Go 类型在 `internal/site/display.go`，用 `site.DecodeStrict` 严格解码）

- `communities.display` = `site.CommunityDisplay`：`name`（必填）、`description`、`memberDisplay`（默认 `avatars_names`）、`nameBlocklist`、`showChannels`/`showOnline`（缺省 true）、`memberLimit`（默认 30，上限 100）、`embed`、`qqGroupNumber`（qq-group 必填，`^\d{5,12}$`）、`contact {label, value}`、`unavailableText`。
- `communities.config`：M1 恒为 `{}`。
- `page_blocks.data`：heading `{"text": {...}, "showCount": bool}`；text `{"markdown": {...}}`；social_row `{"linkIds": [uuid…]}`；community/link `{}`。
- `community_qr_codes.note`：LocalizedText。
- `provider_snapshots.data` = `provider.Snapshot` 的 JSON（见 12.1）。
- `site_settings.data` = `site.Settings`（见 5.2）。

### 4.2 查询（`internal/store/queries/*.sql`，生成到 `internal/store/dbq`）

| 文件 | 查询 |
|---|---|
| `blocks.sql` | `ListVisibleBlocks(page_id, now)`、`NextBlockBoundary(page_id, now)`（无边界时 `Valid=false`）、`InsertBlock` |
| `communities.sql` | `ListCommunities(page_id)`、`GetCommunityBySlug`、`GetGoCommunity(slug)`（LEFT JOIN 快照与二维码）、`ListDueCommunities(now, max_rows)`（非 static，且无快照或 `next_fetch_at <= now`）、`InsertCommunity` |
| `snapshots.sql` | `ListProviderSnapshots(page_id)`、`GetProviderSnapshot`、`UpsertProviderSnapshot`、`EnsureProviderSnapshot`（插入 pending 行） |
| `links.sql` | `ListLinks(page_id)`、`GetLinkBySlug`、`InsertLink` |
| `platforms.sql` | `ListCustomPlatforms`、`InsertCustomPlatform` |
| `media.sql` | `GetMedia`、`ListMediaByKeys(keys[])`、`InsertMedia`（`ON CONFLICT DO NOTHING`） |
| `media_proxy.sql` | `UpsertMediaProxy`（冲突时只刷新 `last_seen_at`）、`GetMediaProxy`、`TouchMediaProxy(keys[])` |
| `qr.sql` | `GetQRCode(id)`（JOIN media）、`ListQRCodes(page_id)`、`InsertQRCode` |
| `seed.sql` | `IsContentEmpty`、`ReplaceSiteSettingsData`（version+1） |

集成测试：`internal/store/integration_m1_test.go`（testcontainers，覆盖约束、级联、可见性边界、到期查询）。

---

## 5. 公开 DTO

### 5.1 JSON 约定

- 字段名 camelCase；**所有字段总是出现**，可空字段写 `null`，不省略。唯一例外是 `blocks[]` 中按 kind 出现的字段。
- 数组和对象永不为 `null`（空就是 `[]` / `{}`）。`LocalizedText` 可以是 `{}`。
- 时间是 UTC 的 RFC 3339 字符串。
- 成功响应包在 `{"data": …}` 中（4.10）。`#lp-data` 的内容也是 `{"data": PublicPage}`。

### 5.2 `site.Settings`（存储，`site_settings.data`，**不直接下发**）

M0 字段之外新增：

| 字段 | 类型 | 默认 | 校验 |
|---|---|---|---|
| `displayName` | LocalizedText | `{}` | ≤300 字符；空时前端用 `title` |
| `bio` | LocalizedText（Markdown 子集） | `{}` | ≤4096 字节 |
| `avatarKey` | string | `""` | 媒体 key |
| `footer` | LocalizedText（Markdown 子集） | `{}` | ≤4096 字节 |
| `showPoweredBy` | bool | `true` | — |
| `og` | `{title, description: LocalizedText, imageKey: string}` | 空 | imageKey 为媒体 key；空时用自动合成的 OG 图 |
| `searchIndexing` | `"index" \| "noindex"` | `index` | 同时影响 meta robots 与 robots.txt |
| `notFound` | LocalizedText | `{}` | 404 页导语，≤300 字符 |
| `copy` | `locale → key → text` | `{}` | key 只能是 `openInBrowser`、`openOnDesktop`、`inviteUnavailable`、`communityUnavailable`；≤300 字符 |

文案回退顺序：`copy[当前语言][key]` → `copy[默认语言][key]` → `copy.en[key]` → 内置文案（`CopyOverrides.Get`）。

### 5.3 `PublicPage`

| 字段 | 类型 | 说明 |
|---|---|---|
| `version` | number | settings 版本 |
| `revision` | string | 除 live 数据以外任何内容变化都会变（由 Builder 计算，例如内容哈希）；LiveDTO 带同一个值 |
| `page` | `{id, slug}` | |
| `site` | PublicSite | 见 5.4 |
| `blocks` | Block[] | `generatedAt` 时刻可见的块，按顺序 |
| `communities` | `{[id]: CommunityView}` | 只含 blocks 引用到的社区，key = id |
| `links` | `{[id]: LinkView}` | 只含 blocks 引用到的链接 |
| `platforms` | `{[id]: PlatformView}` | 只含 communities 引用到的平台 |
| `generatedAt` | string | |
| `nextBoundary` | string \| null | 下一个可见性边界；到达后必须重建 |

### 5.4 `PublicSite`

`baseUrl`（config base_url，无结尾斜杠，用于「在电脑上打开」和分享地址）、`defaultLocale`、`locales`、`title`、`description`、`displayName`、`bio`（Markdown）、`avatar: ImageView|null`、`appearance`、`theme`（同 M0）、`footer`（Markdown）、`showPoweredBy`、`notFound`、`copy`。
OG、searchIndexing 等只在服务端 `<head>` 使用，放在 `site.Snapshot.Head`，不下发。

Markdown 子集（服务端 goldmark+bluemonday 与前端 `web/src/shared/markdown` 一致，由共享语料 `web/src/test/fixtures/markdown-cases.json` 钉住；段落内换行即 `<br>`，不支持 `<…>` 自动链接和引用式链接，单个 `~` 也是删除线）：段落、换行、强调、加粗、删除线、行内代码、链接（scheme 白名单 https/http/mailto，`rel="nofollow noopener noreferrer"`、新窗口）、有序/无序列表、引用。**不支持**原始 HTML、图片、标题、表格、裸 URL 自动链接。

### 5.5 `Block`（按 `kind` 区分的联合类型）

| kind | 专有字段 |
|---|---|
| `community` | `communityId` |
| `link` | `linkId` |
| `heading` | `text: LocalizedText`；`count?: number`（只有 `showCount` 时出现，等于到下一个 heading 之前的社区块数） |
| `text` | `markdown: LocalizedText` |
| `social_row` | `linkIds: string[]`（非空） |

### 5.6 `CommunityView`

| 字段 | 类型 | 说明 |
|---|---|---|
| `id`, `slug` | string | |
| `provider` | `discord\|kook\|static` | |
| `platform` | string | `platforms` 的 key |
| `card` | `discord\|kook\|qq-group\|wechat-group\|static` | 选择卡片组件 |
| `name` | LocalizedText | 管理员填写的显示名（必填），**不用**上游名称 |
| `description` | LocalizedText | 纯文本 |
| `icon` | ImageView\|null | 上传图标（`/media/u`）优先，其次 provider 图标（`/media/p`），都没有为 null（显示平台图标） |
| `sharePath` | string | `/c/{slug}` |
| `inviteUrl` | string\|null | 永久邀请绝对地址，用于「复制邀请」；未配置或已知失效（10006）时为 null |
| `memberDisplay` | `hidden\|avatars\|avatars_names` | 默认 `avatars_names`；不含 game |
| `embed` | `{kind:"discord", src}`\|null | 仅 discord 且 `display.embed` 时；点击后才加载 iframe |
| `qq` | `{groupNumber}`\|null | **恰好**在 qq-group 卡片上出现 |
| `qr` | `{url:"/media/q/{uuid}", width, height, note}`\|null | wechat-group 必有；qq-group 可选 |
| `contact` | `{label, value}`\|null | 兜底联系方式（一键复制） |
| `unavailableText` | LocalizedText | `{}` 表示用 copy/内置文案 |
| `live` | LiveView | 见 5.7 |

`ImageView = {url, width, height}`（必须渲染宽高）。`IconView = {kind: simple|builtin|media, name, url}`：`simple` 是 simple-icons slug（已核对 16.34.0 中存在 discord、telegram、qq、wechat、bilibili、steam、teamspeak、matrix、github、x）；`builtin` 是前端自带图标（至少 `kook`、`revolt`、`link`）；`media` 时 `url = /media/u/{name}`，否则 `url = null`。

### 5.7 `LiveView`（同时由 `/api/v1/public/live` 下发）

| 字段 | 说明 |
|---|---|
| `state` | 卡片状态，见第 6 节 |
| `online`, `members` | number\|null；未知或管理员隐藏时为 null |
| `onlineSource` | `invite\|widget\|badge\|null`；**恰好**在 `online` 为 null 时为 null |
| `channels` | `[{id, name}]`；未知或隐藏时 `[]` |
| `users` | `[{name, avatarUrl, status}]`，服务端已按展示档位、屏蔽词、上限处理：`hidden` → `[]`；`avatars` → `name` 全为 null；`avatars_names` → `name` 必有。`avatarUrl` 为 `/media/p/...` 或 null；`status ∈ online\|idle\|dnd` |
| `updatedAt` | 最近一次成功拉取时间（`last_ok_at`），从未成功为 null；stale 时显示「N 分钟前更新」 |
| `joinUrl` | `"/go/{slug}"` 或 null。**等于** `provider.JoinTarget(...) != ""`（第 7 节），所以按钮与 `/go` 永远一致；unavailable 与 wechat-group 恒为 null |

### 5.8 `LinkView` / `PlatformView`

`LinkView = {id, slug, kind: link|social, label, url（绝对目标）, href, icon, relMe}`：`href` 是 `<a href>` 的值，普通链接为 `/go/{slug}`；`relMe` 为 true 时 `href = url`（rel=me 校验要求直链），并加 `rel="me"`。
`PlatformView = {id, name, icon, needsExternalBrowser}`。

### 5.9 内部快照 `site.Snapshot`

M0 字段之外：`Public *PublicPage`（永不为 nil）、`Head HeadMeta{OGTitle, OGDescription, OGImage *ImageView(相对路径), Robots, Icons map[size]path}`、`Files *media.SiteFiles`（首次构建前为 nil）、`NextBoundary()`。`site.Builder`（content 实现）负责：读 DB → 组装 → `PublicPage.Validate()` 应在测试中通过 → 调 `media.Generator` → 发布到 `Holder`；只有 `revision` 或 live 数据变化时才替换快照（保持 ETag 稳定）。

---

## 6. 卡片状态机

| state | 产生条件（provider / 构建者） | 卡片显示 | 主按钮（`joinUrl`） |
|---|---|---|---|
| `pending` | 尚未拉取 | 管理员名称 + 骨架屏 | 有目标就显示 |
| `live` | 最近一次成功 | 实时数据 | 加入 → |
| `stale` | 拉取失败，沿用 `last_ok_at` 的数据 | 实时数据 +「N 分钟前更新」 | 加入 → |
| `degraded` | Discord 永久邀请返回 10006（`inviteInvalid=true`） | 数据正常，`inviteUrl=null` | 有 instant invite / fallback 时显示，否则显示 `inviteUnavailable` 文案 |
| `static` | static 平台（含 QQ 群）；Discord 50004（Widget 未开启）；KOOK 徽章无法解析 | 管理员内容；QQ 群显示群号（复制）和可选二维码 | 有目标时「加入/加群」，QQ 群无链接时主按钮是「复制群号」 |
| `qr-only` | wechat-group | 内联二维码（PNG `<img>`，≥240px，长按识别）+ 说明 + 兜底联系方式 | 无；次要按钮「复制微信号」 |
| `unavailable` | Discord 10004；KOOK 非公开（`ErrNotPublic`） | 灰显 + `unavailableText`/`communityUnavailable` | 隐藏 |

provider 的错误分类（`Provider.Fetch` 约定）：逻辑结果（50004、10004、10006、KOOK 非公开、解析失败）返回快照 + 对应 state + `ErrCode`，error 为 nil；瞬时错误（网络、5xx、429、畸形响应）返回 error → refresh job 把上次数据标为 stale（pending 保持 pending）、`fail_count+1`、按 `max(RetryAfter, 指数退避)` 推迟，退避上限 1 小时；连续 3 次失败打 WARN。live/degraded 在 `max(3×refresh_interval, 15min)` 内没有成功时，由 Builder 显示为 stale（每分钟重建页面时复查）；widget 的 instant invite 以首次看到的时间 + 23h 作为 `inviteExpiresAt`（再次看到同一邀请不会延长）。`ErrCode` 只存错误码（`discord.ErrorCode` 格式，如 `discord_404_10004`），不存上游响应体。

---

## 7. `/go/{slug}` 决策表

目标选择（`provider.JoinTarget`，页面按钮与 `/go` 共用，已实现并有测试）：

| 条件 | 目标 |
|---|---|
| `state = unavailable` 或 `card = wechat-group` | 无 |
| static 平台（含 QQ 群） | `invite_url` → `fallback_url` |
| provider 平台（discord/kook） | 永久邀请（`inviteInvalid` 为 false 时）→ 未过期的 widget `instant_invite` → `fallback_url` |

UA 识别（`uaclass.Classify`，规则写死，fixture 表驱动测试，前端用同一规则）：
- 微信：含 `MicroMessenger`。
- QQ：含 `QQ/`（QQ App 内 WebView）。只有 `MQQBrowser` 而无 `QQ/` 的是 QQ 浏览器，按普通浏览器处理。
- 移动端：含 `Mobi`、`Android`、`iPhone`、`iPad` 等。

只解析当前已发布的社区和链接（可见且在显示时段内的块；链接也可通过 social_row 发布），其余 slug 与未知 slug 一样返回 404。

响应（按顺序匹配第一条）：

| # | 条件 | 动作 | 响应 |
|---|---|---|---|
| 1 | slug 不合法或不存在 | `not-found` | 404 页 |
| 2 | slug 是链接 | `redirect` | 302 → `links.url`（不看 UA） |
| 3 | wechat-group | `qr-code` | 200：二维码 + 说明 + 兜底联系方式 |
| 4 | qq-group 且在微信内 | `qq-group` | 200：群号（复制）+ 二维码（如有）。不 302，微信会拦 `qm.qq.com` |
| 5 | 无目标（含 unavailable、QQ 群无链接） | QQ 群 → `qq-group`；其他 → `unavailable` | 200：`inviteUnavailable` 文案 + 其他社区列表；**绝不 302 到已知失效的链接** |
| 6 | 平台 `needs_external_browser` 且在微信或 QQ 内 | `open-in-browser` | 200：「点击右上角 ··· → 在浏览器打开」遮罩（箭头指向右上角）+「复制链接」按钮（复制 `base_url/go/{slug}`）。在浏览器里打开后会再次命中本表并 302 |
| 7 | 其他 | `redirect` | 302 → 目标 |

- 所有响应 `Cache-Control: no-store`、`X-Robots-Tag: noindex`；引导页是自包含 HTML（html/template、hash CSP、无外部脚本），文案来自 `site.Snapshot`（copy 覆盖 + golink 内置中英文案），语言按 `?lang=` → `Accept-Language` → 默认语言。
- **永不返回 429。** M1 不计数：调用 `ClickHook.OnClick`，集成时传 `golink.NoopClickHook{}`；M3 换成统计实现。
- 前端遮罩规则与表中 6 相同，作用于「加入」「打开邀请」「加载 iframe」三个动作；手机上（含微信/QQ 内）的 Discord 卡片另有次要操作「在电脑上打开」：弹层大字显示 `site.baseUrl + sharePath`（或首页 `baseUrl`）和复制按钮，文案 `openOnDesktop`。

---

## 8. LiveDTO 与轮询

`GET /api/v1/public/live` → `{"data": {revision, communities: {[id]: LiveView}, generatedAt}}`。

- 前端只在 `document.visibilityState === "visible"` 时每 **60 秒**轮询一次；页面重新可见且距上次轮询超过 60 秒时立即补一次。
- 请求带 `If-None-Match`；未变化返回 **304**。200 响应带强 ETag（不含 `generatedAt`）和 `Cache-Control: public, max-age=0, s-maxage=30`。
- `revision` 与当前页面的 `PublicPage.revision` 不同时，前端重新请求 `/api/v1/public/bootstrap` 并整体替换数据。
- map 中缺失的社区保持原状态。网络错误静默忽略，下一轮再试。

---

## 9. RenderDTO（拓扑 C Worker）

`GET /api/v1/public/render?path=&lang=` → `{"data": RenderDTO}`：

| 字段 | 说明 |
|---|---|
| `status` | 200 或 404 |
| `lang` | 解析后的语言 |
| `appearance` | 写入 `<html data-appearance>` |
| `head` | `<head>` 片段：title、description、robots、canonical、hreflang、og/twitter/itemprop、theme-color、favicon/manifest 链接，以及三个内联块（`lp-critical` 样式、`lp-theme` 样式、boot 脚本）。**不含** charset、viewport、入口 script/link（静态壳里已有） |
| `fallback` | `#root` 内的兜底 markup |
| `data` | PublicPage\|null（404 时为 null） |
| `csp` / `cspReportOnly` | 响应头的值（hash 与 `head` 中的内联块对应；有 Discord iframe 时含 `frame-src https://discord.com`） |
| `etag` | 源站弱 ETag |

`path` 只接受 `/`、`/privacy`、`/c/{slug}`，其他或未知 slug 返回 404 渲染（`data=null`）。Worker 的处理步骤见 `internal/site/live.go` 中 `RenderDTO` 的注释：设置 `<html>` 属性 → 删掉静态壳中的 `<title>` 和 description → 追加 `head` → 替换 `#root` → `data` 非 null 时追加 `<script id="lp-data" type="application/json">{"data":…}</script>`（`<`、`>`、`&`、U+2028、U+2029 转义为 `\uXXXX`）→ 用 `status` 和 CSP 响应，`Cache-Control: no-cache, no-transform`。Worker 按 `path+lang` 缓存 60 秒；配置了共享密钥时带 `X-LP-Proxy-Auth`。

webui 需提供：`(*Renderer).RenderDTO(path, lang, acceptLanguage string) (site.RenderDTO, error)`，以及 `PublicCSP(bootHash, criticalHash, themeHash string, discordFrame bool) string`（签名变更，由 webui 修改其测试）。HTML 渲染也要读 `Accept-Language`（M0 遗留项）。

---

## 10. seed.yaml

`config: seed_file` 指向它；**只在 `page_blocks`、`communities`、`links` 全空时导入**，所有行在一个事务中写入，事务内再查一次 `IsContentEmpty`，所以重复启动是幂等的。图片路径相对 seed 文件所在目录，不能越出该目录；图片先经 `media.Processor` 处理再开事务。文件本身的错误都包装 `seed.ErrInvalidSeed`，错误信息给出 YAML 路径（如 `communities[2].guild_id`）。完整示例：`config/seed.example.yaml`（`internal/seed/seed_test.go` 会解析它）。

```yaml
# 示意（`a / b: {…}` 表示多个同类键），完整可解析的例子见 config/seed.example.yaml
version: 1                      # 必须为 1；未知键报错
site:                           # 全部可选，缺省用内置默认值
  default_locale: zh-CN
  locales: [zh-CN, en]
  title / description / display_name / bio / footer / not_found: { zh-CN: …, en: … }
  avatar: images/avatar.png     # → media(kind avatar)
  show_powered_by: true
  appearance: auto              # light | dark | auto | visitor-choice
  search_indexing: index        # index | noindex
  og: { title: {…}, description: {…}, image: images/og.jpg }
  copy: { zh-CN: { openInBrowser: …, openOnDesktop: …, inviteUnavailable: …, communityUnavailable: … } }
platforms:                      # 自定义平台
  - { id: heybox, name: { zh-CN: 黑盒语音 }, icon: builtin:heybox | si:<slug> | images/x.png,
      url_pattern: '^https://', needs_external_browser: false }
communities:
  - slug: discord               # 必填，slug 正则
    platform: discord           # 预设或自定义平台 id；provider 由平台推出
    name: { zh-CN: …, en: … }   # 必填
    description: {…}
    icon: images/x.png
    guild_id: "1114391825336250432"   # discord(17–20 位)/kook(1–20 位) 必填，必须加引号
    invite: https://discord.gg/xxxx   # 永久邀请 / 官方加群链接，必须 https 且匹配平台 url_pattern
    fallback_url: https://…
    qq_group: "123456789"             # qq-group 必填
    qr: { image: images/qr.png, note: {…} }     # wechat-group 必填
    contact: { label: {…}, value: my_wechat_id }
    members: avatars_names            # hidden | avatars | avatars_names
    name_blocklist: [spam]
    show_channels: true
    show_online: true
    member_limit: 30                  # ≤100
    embed: false                      # Discord iframe facade
    refresh_interval: 10m             # 不低于 provider.MinInterval
    unavailable_text: {…}
links:
  - { slug: blog, kind: link | social, label: {…}, url: https://…, icon: si:github | builtin:link | images/x.png, rel_me: false }
blocks:                         # 每项恰好一个主键；省略整个 blocks 时用默认布局
  - { heading: {…}, show_count: true }
  - { community: discord }
  - { link: blog }
  - { text: { zh-CN: "**Markdown**" } }
  - { social_row: [github, mastodon] }
  - { community: kook, visible: true, visible_from: 2026-10-10T00:00:00+08:00, visible_to: … }
```

默认布局：标题「社区 / Communities」（`show_count`）→ 按文件顺序的全部社区 → 全部 `kind: link` 的链接 → 一个包含全部 `kind: social` 链接的 social_row（没有则省略）。

---

## 11. platforms.yaml

嵌入二进制（`internal/provider/platforms.yaml`），严格解码（未知键报错）：

```yaml
version: 1
platforms:
  - id: discord                  # ^[a-z][a-z0-9-]{1,31}$，唯一
    name: { en: Discord, zh-CN: Discord }   # 至少 en
    icon: si:discord             # si:<simple-icons slug> | builtin:<name> | ""
    provider: discord            # discord | kook | ""(static)
    card: discord                # discord | kook | qq-group | wechat-group | static
    url_pattern: '^https://…'    # RE2；invite/fallback URL 必须匹配；"" = 只做 content.SafeURL 校验
    needs_external_browser: true # 微信/QQ 内引导到浏览器
```

M1 预设：discord、kook、telegram、qq-group、qq-channel、wechat-group、bilibili、steam-group、teamspeak、matrix、revolt、link（Guilded 已删除）。`needs_external_browser` 为 true 的：discord、telegram、matrix、revolt。自定义平台（`custom_platforms`）一律 `card: static`、`provider: ""`，id 不得与预设重复（`Catalog.WithCustom` 检查）。

---

## 12. Go 接口一览

### 12.1 `internal/provider`

- `State`（7 值，`States()`、`Valid()`）；`Source*` 常量；`Card*` 常量（与 `site.CardKind` 同值）。
- `Snapshot`：`name, iconPath, bannerPath, online, members, onlineSource, channels, instantInviteUrl, inviteExpiresAt, inviteInvalid, inviteFetchedAt` 写入 `provider_snapshots.data`；`Users` 只在内存（`json:"-"`）；`State/ErrCode/FetchedAt` 对应表的列（`json:"-"`）。图片字段存已登记的 `/media/p/...` 路径。
- `Provider{Kind, Capabilities, ValidateConfig(ConfigInput), Fetch(ctx, FetchInput), MinInterval, ImageHosts}`；`FetchInput{Config, Previous, Now}`（与文档 5.1 的差异：传入上次快照，用于 invite 15 分钟节奏）。
- `RetryAfterError` / `RetryAfter(err)`；`ImageRegistrar.Register(ctx, providerKind, imageKind, rawURL) (path, error)`；`Image*` 常量。
- `Registry`（`NewRegistry`、`Get`、`Kinds`、`ImageHosts`）；`LiveSource` / `LiveStore`（内存快照，含 users，读写都复制）。
- `Platform`、`Catalog`（`LoadPresets`、`ParsePlatforms`、`NewCatalog`、`WithCustom`、`Get`、`All`）。
- `ClientOptions{ProxyURL, UserAgent, Timeout}`、`NewHTTPClient`（不跟随重定向、无 CookieJar、不读环境代理）、`UserAgent(version)` = `LinksPage/<ver> (+https://github.com/Nanako1900/linksPage)`、`DefaultDiscordAPIBase`、`DefaultKOOKAPIBase`。
- `SanitizeSnapshot`：名称 100、用户名 32、频道名 100 字符；频道最多 50、用户最多 100；用 `content.CleanText`。
- `JoinInput` / `JoinTarget`（已实现）。
- `discord.NewProvider(Options{APIBase, Client, Images})`、`kook.NewProvider(Options{APIBase, Client})`；现有 `discord.WidgetURL`/`InviteURL`、`kook.BadgeURL` 写死了主机，providers 需要增加按 `APIBase` 构造的版本（旧函数可保留）。

### 12.2 `internal/jobs`

`Job{Name, Interval, Timeout, LeaderOnly, Run}`、`LeaderLock{TryAcquire, Release}`、`NewPGLeaderLock(pool, LeaderLockKey)`、`NewScheduler(lock, logger, jobs...)`、`(*Scheduler).Run(ctx)`、`runSafe`（已实现：recover + 超时 + slog）、`NewRefreshJob(RefreshDeps)`、常量 `RefreshTick=30s`、`RefreshBatch=10`、`MaxBackoff=1h`、`NotifyAfterFailures=3`。

### 12.3 `internal/imgproxy`

`NewRegistrar(store, mediaKey, hosts)`、`(*Registrar).Register`（实现 `provider.ImageRegistrar`）、`Key(mediaKey, url)`、`FileRe`、`NewHandler(HandlerOptions{Store, Client, Hosts, Logger})`（`Hosts` 必填，与 registrar 使用同一份 `imageHosts()` 列表，取图前对已登记行再次校验）；限制常量 `MaxUpstreamBytes=2MiB`、`UpstreamTimeout=5s`、`UpstreamConcurrency=4`、`NegativeCacheTTL=10m`、`CacheBytes=16MiB`。规范化规则：`url.URL` 重组、`path.Clean`、拒绝 `%`、`\`、`..`，再按 host + 路径前缀校验；无扩展名的 Discord widget-avatars 按 `png` 登记。

### 12.4 `internal/media`

`Store`（`Put`、`Open`）/ `LocalStore`（`os.OpenRoot`）/ `KeyFor`；`Semaphore`（`NewSemaphore(MemoryBudget)`、`Acquire(ctx, weight, AcquireWait)` 超时 → `ErrBusy`）/ `ImageWeight(w,h)=w*h*4*2`；`Processor.Ingest(ctx, r, ProcessOptions{Kind, MaxBytes, MaxSide}) → Result{Primary, Variants}`（调用方负责 `InsertMedia`）；`UploadsHandler`、`QRHandler`；`Generator.Generate(ctx, GenerateInput) → Generated{OGImage, Favicons, Files}`；`SiteFiles{FaviconICO, Manifest, RobotsTxt, ETag}`、`SiteFilesHandler`。错误：`ErrTooLarge`(413)、`ErrUnsupportedType`(415)、`ErrDimensions`/`Err16BitPNG`(422)、`ErrBusy`(503 + Retry-After)、`ErrNotFound`。SVG 一律拒绝。测试默认不带 `nodynamic` 标签运行，gen2brain/webp 会自动回退到 WASM；发布构建带 `-tags nodynamic`（已验证 `CGO_ENABLED=0` 下编码、解码和交叉编译均可用）。

### 12.5 `internal/content`

`NewMarkdown()` / `(*Markdown).Render(src) (string, error)`、`MaxMarkdownBytes=4096`、`ErrMarkdownTooLong`；`SafeURL(raw)`、`HTTPSURL(raw, hosts...)`、`AllowedSchemes`、`MaxURLLength=2048`、`ErrUnsafeURL`；`CleanText(s, maxRunes)`（已实现并测试）。

### 12.6 `internal/uaclass` / `internal/golink`

`uaclass.Classify(ua) Class{InWeChat, InQQ, Mobile}`、`Class.InApp()`。
`golink.Source{Community(ctx, slug), Link(ctx, slug)}`（找不到返回 `ErrNotFound`；DB 实现由 golink 基于 `dbq.GetGoCommunity`、`GetLinkBySlug` 和平台目录编写）、`CommunityTarget{…, Join provider.JoinInput, …}`、`NewResolver(src, now)`、`Resolve(ctx, slug, uaclass.Class) (Decision, error)`、`Action*` 常量、`NewHandler(HandlerOptions{Resolver, Snapshot, BaseURL, Hook, Logger})`、`ClickHook` / `NoopClickHook` / `ClickEvent`。

### 12.7 `internal/seed`

`Parse(data)`（已实现，严格解码）、`NewImporter(Deps{DB, Media, Logger, MaxUploadBytes})`、`Import(ctx, path) (Result, error)`、`ErrInvalidSeed`、schema 类型见 `schema.go`。

### 12.8 `internal/site`

DTO 类型（第 5 节）、`Settings` 扩展、`CommunityDisplay` 等存储类型、`DecodeStrict`、`PublicSiteFrom`、`EmptyPublicPage`、`(*PublicPage).Live()` / `NeedsDiscordFrame()` / `Validate()`、`Snapshot.Public/Head/Files`、`BuildQuerier`、`AssetGenerator`、`NewBuilder(BuilderDeps)`、`Build` / `Rebuild` / `RebuildIfDue`。

---

## 13. 配置新增（`internal/config`，已实现并测试）

| 键 | 环境变量 | 默认 | 校验 |
|---|---|---|---|
| `seed_file` | `LP_SEED_FILE` | `""`（不导入） | `.yaml`/`.yml` |
| `uploads.max_bytes` | `LP_UPLOADS__MAX_BYTES` | 5242880 | 64 KiB–20 MiB |
| `providers.discord.api_base` | `LP_PROVIDERS__DISCORD__API_BASE` | `https://discord.com` | 绝对 http(s)，无凭据、查询、片段、结尾斜杠；可带路径前缀（测试用） |
| `providers.kook.api_base` | `LP_PROVIDERS__KOOK__API_BASE` | `https://www.kookapp.cn` | 同上 |
| `edge.proxy_auth` / `edge.proxy_auth_file` | `LP_EDGE__PROXY_AUTH(_FILE)` | 空 | ≥32 字节；`*_file` 层级规则同 `db.password_file` |

另有 `(*Config).DeriveKey(info, size)`（HKDF-SHA256，info 常量 `KeyInfoMediaProxy`、`KeyInfoVisitor`、`KeyInfoDevice`）。`config/config.example.yaml` 已加注释示例。

---

## 14. 已加入的依赖（构建者不要再 `go get`）

| 模块 | 版本 | 用途 / 引用位置 |
|---|---|---|
| `golang.org/x/image` | v0.46.0 | `draw.CatmullRom`（media）；webp 解码由 gen2brain 提供 |
| `github.com/gen2brain/webp` | v0.6.4 | WebP 编解码（`-tags nodynamic` 已验证纯 Go） |
| `golang.org/x/sync` | v0.23.0 | `semaphore`、`singleflight`、`errgroup` |
| `github.com/hashicorp/golang-lru/v2` | v2.0.7 | `expirable.LRU`（imgproxy） |
| `go.yaml.in/yaml/v3` | v3.0.5 | seed.yaml、platforms.yaml（koanf 已在用） |
| `github.com/yuin/goldmark` | v1.8.6 | Markdown |
| `github.com/microcosm-cc/bluemonday` | v1.0.27 | HTML 清洗 |

前端依赖（markdown-to-jsx、simple-icons、uqr 等）由 frontend 自行加入 `web/package.json`，加入前先确认版本能解析。

---

## 15. 与 tech-selection.md 的偏差

| 项目 | 文档 | 契约 | 原因 |
|---|---|---|---|
| `page_blocks.ref_id` | 多态 `ref_id` | `community_id`、`link_id` 两个 FK（CASCADE） | 删除社区或链接时自动删块，有引用完整性 |
| 图标 | `communities.display`/`links.icon` 文本 | `icon`（`si:`/`builtin:`）+ `icon_key`（FK media）二选一 | 媒体回收能找到引用 |
| `/go` slug | 社区与链接各自 UNIQUE | 额外用触发器保证两表不重复 | `/go/{slug}` 共用一个命名空间 |
| `Provider.Fetch` | `Fetch(ctx, cfg any)` | `Fetch(ctx, FetchInput{Config, Previous, Now})`；`ValidateConfig(ConfigInput)` | invite 与 widget 节奏不同；校验需要行字段 |
| `Snapshot.IconKey` | `*Key` | `IconPath`/`BannerPath`/`AvatarPath` 存完整 `/media/p/...` 路径 | DTO 直接使用，避免各处重复拼接扩展名 |
| unavailable 的 `/go` | 未规定 | 无目标（不跳 fallback），显示「邀请暂不可用」页 | 与「主按钮隐藏」一致 |
| `/media/q` 410 | 替换/删除后 410 | 格式合法但不存在的 UUID 一律 410 | 不需要墓碑表 |
| 头像 PNG 回退 | 另存 PNG | DTO 只给 WebP（Processor 仍可存 PNG 变体用于生成 favicon） | 基线浏览器全部支持 WebP |
| `media.key` | `sha256[:16].<ext>` | 前 16 **字节**的 hex（32 字符） | 明确长度 |
| Discord CORS | 无 CORS 头 | 以 providers.md 实测为准 | — |

---

## 16. 构建者须知

- 覆盖率：自己负责的每个包 ≥80%（不含生成代码）；Go 用表驱动测试，Web 用 Vitest。不得削弱已有测试。
- `PublicPage.Validate()` 是契约检查：content 的 Builder 测试、webui 的渲染测试应对构建出的页面调用它。
- 不要提交（commit/push），也不要在任何文件中添加 AI 署名行。
- 注释用英文，文档用中文。
