# LinksPage

一个可自托管的社区聚合分享页：把你的 Discord、KOOK、QQ 群、微信群等社区汇总到**一个链接**里，实时展示服务器名称、在线人数、频道与在线成员，并提供加入入口。

> 状态：设计阶段，尚无可运行代码。技术选型见 [docs/tech-selection.md](docs/tech-selection.md)。

## 计划特性

- 访客无需登录；管理员通过配置文件中的 OAuth2/OIDC 白名单或本地账号登录
- Discord（widget.json + invite 接口 + 官方 iframe）、KOOK（免 token 徽章解析）、QQ 群 / 微信群二维码卡片
- 在微信 / QQ 内置浏览器中打开 Discord 时引导到系统浏览器
- 无 Cookie、不存原始 IP 的访问统计
- 所有个性化内容（标题、头像、主题、字体、社区、页脚、OG 图、语言……）均可由管理员修改
- 技术栈：Vite + React + Tailwind CSS / Go / PostgreSQL，Docker Compose 部署，前置 Cloudflare

## License

[MIT](LICENSE)
