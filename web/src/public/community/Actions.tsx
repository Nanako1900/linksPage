import type { MouseEvent } from "react";
import { CopyButton } from "../components/CopyButton";
import { Glyph } from "../components/icons/glyphs";
import { useEnv } from "../context";
import type { CardAction } from "./model";

interface ActionProps {
  action: CardAction;
  primary: boolean;
  /** Community display name (dialog titles). */
  name: string;
}

function ActionButton({ action, primary, name }: ActionProps) {
  const { t, openDialog } = useEnv();
  const cls = `lp-btn ${primary ? "lp-btn-primary" : ""}`;
  switch (action.kind) {
    case "join": {
      const onClick = action.intercept
        ? (e: MouseEvent<HTMLAnchorElement>) => {
            e.preventDefault();
            openDialog({ kind: "browser", url: action.copyUrl });
          }
        : undefined;
      return (
        <a href={action.href} onClick={onClick} className={cls}>
          {t(action.label)}
          <span className="lp-arrow" aria-hidden="true">
            →
          </span>
        </a>
      );
    }
    case "copy":
      return <CopyButton value={action.value} label={t(action.label)} primary={primary} />;
    case "qr":
      return (
        <button type="button" className={cls} onClick={() => openDialog({ kind: "qr", title: name, qr: action.qr })}>
          <Glyph name="qr" />
          {t("qrCode")}
        </button>
      );
    case "qrGenerate":
      return (
        <button
          type="button"
          className={cls}
          onClick={() => openDialog({ kind: "qrGenerate", title: name, url: action.url })}
        >
          <Glyph name="qr" />
          {t("qrCode")}
        </button>
      );
    case "desktop":
      return (
        <button
          type="button"
          className="lp-btn lp-btn-quiet"
          onClick={() => openDialog({ kind: "desktop", url: action.url })}
        >
          <Glyph name="monitor" />
          {t("openOnDesktopAction")}
        </button>
      );
  }
}

const actionKey = (a: CardAction): string => (a.kind === "copy" ? `copy-${a.label}` : a.kind);

interface ActionsProps {
  primary: CardAction | null;
  secondary: CardAction[];
  name: string;
}

export function Actions({ primary, secondary, name }: ActionsProps) {
  if (!primary && secondary.length === 0) return null;
  return (
    <div className="mt-4 flex flex-wrap gap-2">
      {primary && <ActionButton action={primary} primary name={name} />}
      {secondary.map((a) => (
        <ActionButton key={actionKey(a)} action={a} primary={false} name={name} />
      ))}
    </div>
  );
}
