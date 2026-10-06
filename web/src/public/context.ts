import { createContext, useContext } from "react";
import type { Translator } from "../shared/i18n/t";
import type { LocalizedText, PlatformView, PublicSite, QRView } from "../shared/types/public";
import type { UAClass } from "../shared/ua";

/** Dialogs shown by the single host at the app root. */
export type DialogSpec =
  /** WeChat/QQ: guide to "open in browser" with a copy-link fallback. */
  | { kind: "browser"; url: string }
  /** Phone: show the page address for a desktop browser. */
  | { kind: "desktop"; url: string }
  /** Uploaded QR image. */
  | { kind: "qr"; title: string; qr: QRView }
  /** Client-generated QR of `url`. */
  | { kind: "qrGenerate"; title: string; url: string };

/** Everything components need about the page and the visitor. */
export interface Env {
  locale: string;
  site: PublicSite;
  platforms: Readonly<Record<string, PlatformView>>;
  t: Translator;
  ua: UAClass;
  /** Current time (ms), refreshed every minute for "updated N minutes ago". */
  now: number;
  /** Resolve LocalizedText with locale → default → en. */
  pick: (text: LocalizedText | null | undefined) => string;
  openDialog: (spec: DialogSpec) => void;
}

export const EnvContext = createContext<Env | null>(null);

export function useEnv(): Env {
  const env = useContext(EnvContext);
  if (!env) throw new Error("useEnv must be used inside EnvContext");
  return env;
}
