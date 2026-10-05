# Spike：Discord 与 KOOK 免认证接口实测（M0）

- 日期：2026-10-05（UTC 13:47–13:50）
- 出口：英国（Discord 返回的 `cf-ray` 后缀为 `LHR`），未经代理
- 工具：`curl -sS -D <headers> -o <body>`，不跟随重定向
- 请求总数：Discord 9 次，KOOK 9 次，间隔 1–2 秒。没有为了触发 429 而刷接口。
- Fixture 位置：
  - `internal/provider/discord/testdata/`
  - `internal/provider/kook/testdata/`
  - `*.headers` 是 curl `-D` 格式（状态行 + 头）。只保留和实现相关的头；`set-cookie`、`report-to`、`nel`、带 Sentry key 的 `content-security-policy` 已删除。每个文件都有一行 `X-Fixture-Note` 说明来源。
  - 以 `_synthetic` 结尾的 fixture 是按文档手写的，**不是实测**。

对应代码（只用标准库，不做网络请求）：

| 包 | 内容 |
|---|---|
| `internal/provider/discord` | widget / invite 的类型定义；`ParseWidget`、`ParseInvite` 把非 200 响应映射为 `*APIError`（10004/50004/10006 → `ErrUnknownGuild`/`ErrWidgetDisabled`/`ErrUnknownInvite`）或 `*RateLimitError`；`ErrorCode` 生成 `Snapshot.ErrCode`；CDN URL 构造（`IconURL`/`BannerURL`/`SplashURL`）；`ImageHosts`；输入校验 `ValidGuildID`、`ExtractInviteCode`；端点构造 `WidgetURL`、`InviteURL` |
| `internal/provider/kook` | `BadgeURL`；`ParseBadgeResponse`（校验 3xx 状态并解析 Location）；`ParseBadgeLocationForStyle`（按请求的 style 严格解释，provider 应使用这个）；`ParseBadgeLocation`（不知道 style 时自动识别） |
| `internal/provider/providertest` | 读取 `*.headers` fixture 的测试辅助 |

---

## 1. Discord

### 1.1 端点与结果

| # | 请求 | 状态 | 关键响应头 | Body | Fixture |
|---|---|---|---|---|---|
| 1 | `GET https://discord.com/api/guilds/1114391825336250432/widget.json` | 200 | `cache-control: public, max-age=300, s-maxage=300`；`last-modified`；`cf-cache-status: EXPIRED`；`x-discord-features: guild-widgets`；无 `access-control-*`；无 `x-ratelimit-*` | 正常 widget | `widget_ok.*` |
| 2 | `GET https://discord.com/api/v10/invites/KwdRuAkT?with_counts=true` | 200 | **没有** `cache-control`；`cf-cache-status: BYPASS`；`x-discord-features: invites`；无 `x-ratelimit-*` | 正常 invite | `invite_ok.*` |
| 3 | `GET .../api/guilds/100000000000000000/widget.json` | **404** | `cache-control: public, max-age=300, s-maxage=300` | `{"message": "Unknown Guild", "code": 10004}` | `widget_unknown_guild.*` |
| 4 | `GET .../api/guilds/662267976984297473/widget.json`（Midjourney，大型公开服务器） | **403** | `cache-control: public, max-age=300, s-maxage=300` | `{"message": "Widget Disabled", "code": 50004}` | `widget_disabled.*` |
| 5 | `GET .../api/v10/invites/lpUnknownInvite0x?with_counts=true` | **404** | 无 `cache-control` | `{"message": "Unknown Invite", "code": 10006}` | `invite_unknown.*` |
| 6 | 同 1，附加 `Origin: https://example.com` | 200 | `access-control-allow-origin: https://example.com`；`access-control-allow-credentials: true` | — | `widget_ok_with_origin.headers` |
| 7 | 同 2，附加 `Origin: https://example.com` | 200 | 同上 | — | `invite_ok_with_origin.headers` |
| 8 | `GET https://cdn.discordapp.com/icons/{guild}/{hash}.png?size=256` | 200 | `content-type: image/png`；`cache-control: public, max-age=31536000`；`access-control-allow-origin: *` | PNG 83 KB | 未保存 |
| 9 | `GET https://cdn.discordapp.com/widget-avatars/{a}/{b}`（无扩展名） | 200 | `content-type: image/png`；`cache-control: public, max-age=31536000`；`access-control-allow-origin: *` | PNG 24 KB | 未保存 |
| — | 429 | 未观测 | — | — | `ratelimited_synthetic.*`、`ratelimited_global_synthetic.*`、`ratelimited_cloudflare_synthetic.*`（均为**手写**） |

### 1.2 观察要点

**widget.json**
- 字段：`id`、`name`、`instant_invite`、`channels[]{id,name,position}`、`members[]`、`presence_count`。和文档一致，不含图标和总人数。
- `members[]` 字段：`id`（widget 内部编号 `"0"`…`"N"`，不是真实用户 ID）、`username`、`discriminator`（固定 `"0000"`）、`avatar`（固定 null）、`status`（实测有 `online`、`idle`）、`avatar_url`（`cdn.discordapp.com/widget-avatars/...`，无扩展名）、可选 `game{name}`。本次 16 人中 5 人有 `game`。成员列表里包含 Bot 账号。
- `presence_count` = 16，与成员数一致，也与 invite 的 `approximate_presence_count` 一致。
- 本次 `instant_invite` 对应的邀请 `expires_at` 是拉取时间 + 24 小时（临时邀请），印证文档「`instant_invite` 是临时的」。
- 服务器名含 U+2018（`‘`）。清洗只能去掉 bidi 控制符和零宽字符，不能动普通标点。

**invite（`with_counts=true`）**
- 有用字段：`code`、`expires_at`（临时邀请有值，永久邀请为 null）、`guild{id,name,icon,splash,banner,description,features,vanity_url_code,...}`、`channel{id,type,name}`、`approximate_member_count`（125）、`approximate_presence_count`（16）。
- **文档未提及：** 响应里还有一个 `profile` 对象（`member_count`、`online_count`、`icon_hash`、`brand_color_primary`、`badge_*` 等）。这是非公开文档字段，**不要依赖**；类型定义里没有收录。
- `guild.features` 含 `ANIMATED_ICON`，但本次 `icon` 哈希没有 `a_` 前缀，所以图标扩展名仍按哈希前缀判断，不能看 features。
- `expires_at` 格式 `2026-10-06T13:47:48+00:00`，`time.Time` 的 RFC 3339 解析可以直接处理。

**错误码映射**（实测，补充 HTTP 状态码）

| code | HTTP | 含义 | 卡片状态（文档 5.3） |
|---|---|---|---|
| 10004 | 404 | Unknown Guild | `unavailable` |
| 50004 | 403 | Widget Disabled | `static` |
| 10006 | 404 | Unknown Invite | `degraded` |

- widget 的错误响应（403/404）**同样带 `max-age=300`**。管理员刚打开 Widget 后，最多可能还要等 5 分钟（CF 边缘缓存）才能拉到数据。后台提示「请开启 Widget」时应说明这一点。
- invite 错误响应没有 `cache-control`。

**限流**
- 无认证请求的 200/4xx 响应**都没有** `x-ratelimit-*` 头，所以无法提前看到剩余额度。invite 的限流规则仍是 **UNVERIFIED**。
- 429 没有实测。fixture 按 Discord 公开文档的格式手写：
  - JSON body：`{"message","retry_after"(浮点秒),"global"}`
  - 头：`Retry-After`、`X-RateLimit-*`、`X-RateLimit-Scope`、`X-RateLimit-Global`
  - 另有 Cloudflare 层封禁（1015）的情况：返回 HTML，不带 JSON 和限流头。
- 解析器的处理：
  - 优先用 body 的 `retry_after`，其次用 `Retry-After` 头。
  - 时间按毫秒取整，上限 1 小时；不接受 HTTP-date。
  - body 不是 JSON 时标记 `Cloudflare=true`，`RetryAfter=0`，由调用方用自己的退避策略。
- 建议（M1）：widget 每 5 分钟拉一次（与 `max-age=300` 一致），invite 每 15 分钟拉一次。遇到 429 时按 `RetryAfter` 和指数退避取较大值。

**CORS（与文档不一致）**
- 文档 5.3 写「没有 CORS 头」。实测：
  - 请求**不带** `Origin` 时，确实没有 `access-control-*`。
  - **带** `Origin` 时，widget 和 invite 都会回显该 Origin，并带 `access-control-allow-credentials: true`。
- 结论：浏览器理论上可以直接跨域请求。但 `discord.com` 在大陆被墙，而且我们需要统一缓存和清洗，所以仍然由服务端拉取。**这个结论不影响架构，只需修正文档里的说法。**
- CDN（`cdn.discordapp.com`）返回 `access-control-allow-origin: *`，`max-age=31536000`。

**其他**
- Discord 会下发 `_cfuvid`、`__dcfduid`、`__sdcfduid` Cookie。出站 client 不设 CookieJar，媒体代理不转发 `Set-Cookie`（文档 5.2 已有此规则）。
- widget-avatars 没有扩展名，但上游 `content-type` 是 `image/png`，可以按文档 5.2 补扩展名。

### 1.3 脱敏说明
- 成员 `username` 依次替换为 `user1`…`user16`，`avatar_url` 统一替换为 `https://cdn.discordapp.com/widget-avatars/REDACTED/REDACTED`。
- `game.name` 替换为 `Game 1`…`Game 5`。
- 字段结构、类型和顺序保持不变。
- 服务器名、ID、频道和邀请码都保留（站长自己的公开服务器；邀请码是 24 小时临时邀请，2026-10-06 过期）。

---

## 2. KOOK

### 2.1 测试对象
- **公开服务器：** `5417470909511807`（「猎杀对决」中文玩家社区）。这个 ID 来自 KOOK 官方仓库 `kaiheila/api-docs` 中的 badge 示例。
- **非公开 / 不存在：** `8474959284287105`。这个 ID 是 KOOK 开发者文档里「如何获取 guild_id」的示例，实测不是公开服务器。

### 2.2 端点与结果

请求格式：`GET https://www.kookapp.cn/api/v3/badge/guild?guild_id=<id>&style=<n>`，不跟随重定向。

| # | 参数 | 状态 | Location 中解码后的 `label` / `message` | Fixture |
|---|---|---|---|---|
| 1 | 公开，`style=0` | 302 | `「猎杀对决」中文玩家社区` / `JOIN` | `badge_public_style0.headers` |
| 2 | 公开，`style=1` | 302 | `10350 ONLINE` / `JOIN` | `badge_public_style1.headers` |
| 3 | 公开，`style=2` | 302 | `10350/107345 ONLINE` / `JOIN` | `badge_public_style2.headers` |
| 4 | 公开，`style=3`（文档没有这个值），带 `Origin` | 302 | 与 `style=0` 相同（服务器名） | `badge_public_style3_with_origin.headers` |
| 5 | 非公开，`style=0` | 302 | `服务器不存在或非公开` / `404` | `badge_not_public_style0.headers` |
| 6 | 非公开，`style=1`、`style=2` | 302 | 同上 | `badge_not_public_style2.headers` |
| 7 | `guild_id=abc` | 302 | 同上 | `badge_invalid_guild_id.headers` |
| 8 | 不带任何参数 | 302 | 同上 | `badge_missing_guild_id.headers` |

所有响应的共同点：
- 都是 `HTTP/2 302`，`server: nginx/1.20.1`，`content-type: application/json; charset=UTF-8`，body 是字面量 `null`。
- **没有 `cache-control`，没有 CORS 头，没有任何限流头。**
- 会下发负载均衡 Cookie `tgw_l7_route`（有效期约 1 分钟），忽略即可。
- Location 固定为 `https://img.shields.io/static/v1?label=…&message=…&color=000000&labelColor=87EB00&style=flat&logo=data:image/svg+xml;base64,…`。其中 `logo` 是约 1.7 KB 的 base64 SVG，整个 Location 约 2.1 KB，在 Go 默认头大小限制之内。
- 查询串编码方式：空格编码为 `+`，`/` 编码为 `%2F`，中文按 UTF-8 百分号编码。必须用 `url.ParseQuery` 解码。

### 2.3 与文档的差异 / 补充
- 文档 5.4 只写了 `style=0|2`。实测：
  - `style=1` 返回 `<在线> ONLINE`。
  - 不认识的 style 值会按 `style=0` 处理。
  - `BadgeURL` 只允许 0–2。
- 非法 ID 或缺少参数时**不会返回 4xx**，同样是 302 + 「服务器不存在或非公开」。所以本地必须先用正则（`^\d{1,20}$`）校验，否则会把输入错误误判成「非公开」。
- 成功时 `message` 固定为 `JOIN`，失败时为 `404`，可以和 label 关键字一起作为判断依据（解析器两者任一命中即判为 `ErrNotPublic`）。
- 名称和计数格式的歧义：服务器名本身可能就是 `5 ONLINE` 这种形式。provider 必须调用 `ParseBadgeLocationForStyle`，按**请求时的 style** 解释 label。`ParseBadgeLocation` 的自动识别只适合调试。
- 计数校验：`在线 > 总数`、数字超过 9 位，都按格式错误处理（降级为 `static` 并告警，与文档一致）。
- 限流：本次 9 次请求没有观察到任何限制或限流头。仍记为 **UNVERIFIED**。建议 M1 与 Discord widget 保持同一节奏（每 5 分钟拉一次；style 0 和 2 各一次）。
- 上游不给缓存头，由我们自己的 TTL 控制。

### 2.4 解析器行为（`kook.ParseBadgeLocationForStyle`）
1. Location 为空或无法解析 → `ErrMalformed`。
2. 必须是 `https://img.shields.io/static/v1`：不能带端口和 userinfo，不能用相似域名 → 否则 `ErrMalformed`。
3. label 含「服务器不存在或非公开」，或 `message=404` → `ErrNotPublic`（卡片状态 `unavailable`，后台提示「请把服务器设为公开」）。
4. label 为空、不是合法 UTF-8、或超过 512 字节 → `ErrMalformed`。
5. 按 style 解释：
   - `0` → 名称。
   - `1` → `^\d{1,9} ONLINE$`。
   - `2` → `^\d{1,9}/\d{1,9} ONLINE$`，且在线数 ≤ 总数。
   - 不匹配 → `ErrMalformed`。
6. 名称的截断（100 字符）和 bidi / 零宽字符清洗属于快照清洗环节（文档 5.1），解析器不处理。

---

## 3. 测试结果

```
go vet ./internal/provider/...                       # 通过
go test -race -cover ./internal/provider/...
  internal/provider/discord        coverage: 100.0%
  internal/provider/kook           coverage: 100.0%
  internal/provider/providertest   coverage: 95.5%
```

## 4. 建议修改 tech-selection.md 的地方
1. **5.3 widget 行：** 把「没有 CORS 头」改为「不带 Origin 时没有 CORS 头；带 Origin 时会回显并允许 credentials。不影响服务端拉取的结论」。另外补充：错误响应（403/404）同样 `max-age=300`。
2. **5.3 错误码表：** 补充 HTTP 状态码（50004 → 403，10004 / 10006 → 404），并注明 50004 和 10006 已于 2026-10-05 实测。
3. **5.3 invite 行：** 补充「无 `cache-control`；无认证请求不返回 `x-ratelimit-*`」。
4. **5.4：** 补充：
   - `style=1` 的格式；
   - 非法或缺失 `guild_id` 时同样返回 302 + 非公开文案，必须先在本地校验；
   - 上游无缓存头、无限流头。
