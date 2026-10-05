import type { Bootstrap } from "../shared/bootstrap";

const palette = {
  bg: "#f9f7f1",
  fg: "#1a1c22",
  muted: "#5f6270",
  card: "#ffffff",
  border: "#e4e1d8",
  accent: "#5b4ad8",
  accentFg: "#ffffff",
};

export function sampleBootstrap(): Bootstrap {
  return {
    version: 3,
    page: { id: 1, slug: "default" },
    site: {
      defaultLocale: "zh-CN",
      locales: ["zh-CN", "en"],
      title: { "zh-CN": "我的社区", en: "My Communities" },
      description: { "zh-CN": "加入我们的社区。", en: "Join our communities." },
      appearance: "auto",
      theme: {
        preset: "signal-paper",
        light: palette,
        dark: palette,
        radius: "0.75rem",
        fontSans: "system",
        fontDisplay: "instrument-serif",
      },
    },
  };
}
