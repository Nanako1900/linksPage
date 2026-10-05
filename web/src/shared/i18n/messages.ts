/** Built-in public UI strings. Content fields come from the database. */
const en = {
  communities: "Communities",
  comingSoon: "Communities will appear here once they are configured.",
  privacy: "Privacy",
  privacyBody: "This page will describe how visits are counted.",
  communityLead: "Community page",
  notFound: "This page does not exist.",
  backHome: "Back to home",
  language: "Language",
  loadError: "The page data could not be loaded. Please refresh.",
  poweredBy: "Powered by LinksPage",
} as const;

export type MessageKey = keyof typeof en;

/** Every built-in locale must translate every key (checked by the compiler). */
export type Catalog = Readonly<Record<MessageKey, string>>;

const zhCN: Catalog = {
  communities: "社区",
  comingSoon: "配置社区后会显示在这里。",
  privacy: "隐私",
  privacyBody: "这里将说明访问统计的方式。",
  communityLead: "社区页",
  notFound: "页面不存在。",
  backHome: "返回首页",
  language: "语言",
  loadError: "页面数据加载失败，请刷新重试。",
  poweredBy: "由 LinksPage 驱动",
};

export const messages: Readonly<Record<"en" | "zh-CN", Catalog>> = { en, "zh-CN": zhCN };
