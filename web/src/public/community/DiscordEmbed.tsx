import { useState } from "react";
import { absoluteUrl } from "../../shared/format";
import type { EmbedView } from "../../shared/types/public";
import { useEnv } from "../context";

/** Theme for the Discord widget, following the resolved page appearance. */
export function widgetTheme(doc: Document = document): "light" | "dark" {
  return doc.documentElement.getAttribute("data-appearance") === "dark" ? "dark" : "light";
}

interface EmbedProps {
  embed: EmbedView;
  name: string;
  sharePath: string;
  /** WeChat/QQ: show the open-in-browser overlay instead of loading. */
  intercept: boolean;
}

/**
 * Click-to-load facade for the Discord widget iframe: nothing is requested
 * from discord.com until the visitor asks for it.
 */
export function DiscordEmbed({ embed, name, sharePath, intercept }: EmbedProps) {
  const { t, site, openDialog } = useEnv();
  const [theme, setTheme] = useState<"light" | "dark" | null>(null);
  if (theme) {
    return (
      <iframe
        src={`${embed.src}&theme=${theme}`}
        title={name}
        width={350}
        height={500}
        loading="lazy"
        sandbox="allow-popups allow-popups-to-escape-sandbox allow-same-origin allow-scripts"
        className="mt-4 block w-full max-w-[350px] rounded-card border border-border"
      />
    );
  }
  const onClick = () => {
    if (intercept) openDialog({ kind: "browser", url: absoluteUrl(site.baseUrl, sharePath) });
    else setTheme(widgetTheme());
  };
  return (
    <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1">
      <button type="button" onClick={onClick} className="lp-btn lp-btn-quiet -ml-3 underline underline-offset-4">
        {t("loadWidget")}
      </button>
      <span className="text-xs text-muted">{t("widgetNotice")}</span>
    </div>
  );
}
