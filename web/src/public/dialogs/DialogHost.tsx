import { useEffect, useState } from "react";
import { type QrImage, qrPng } from "../../shared/qr";
import { CopyButton } from "../components/CopyButton";
import { type DialogSpec, useEnv } from "../context";
import { Dialog } from "./Dialog";

interface HostProps {
  spec: DialogSpec | null;
  onClose: () => void;
  /** Injectable for tests. */
  makeQr?: (url: string) => Promise<QrImage>;
}

/** Open-in-browser guide for WeChat/QQ (doc 5.7): arrow to the top-right menu. */
function BrowserOverlay({ url, onClose }: { url: string; onClose: () => void }) {
  const { t } = useEnv();
  return (
    <Dialog title={t("openInBrowserTitle")} closeLabel={t("close")} onClose={onClose} variant="overlay">
      <svg
        className="lp-overlay-arrow absolute top-[calc(0.5rem+env(safe-area-inset-top))] right-6"
        width="96"
        height="96"
        viewBox="0 0 96 96"
        fill="none"
        stroke="currentColor"
        strokeWidth="3"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
      >
        <path d="M14 86C30 50 52 28 84 12" />
        <path d="M60 10l24 2-6 22" />
      </svg>
      <div className="mx-auto mt-28 flex max-w-[22rem] flex-col gap-6">
        <p className="font-display text-3xl leading-snug">{t("openInBrowser")}</p>
        <p className="break-all font-mono text-sm opacity-80">{url}</p>
        <CopyButton value={url} label={t("copyLink")} primary className="self-start" />
      </div>
    </Dialog>
  );
}

function DesktopDialog({ url, onClose }: { url: string; onClose: () => void }) {
  const { t } = useEnv();
  return (
    <Dialog title={t("openOnDesktopAction")} closeLabel={t("close")} onClose={onClose}>
      <p className="mt-3 text-sm text-muted">{t("openOnDesktop")}</p>
      <p className="my-5 select-all break-all font-mono text-2xl leading-snug tracking-tight">{url}</p>
      <CopyButton value={url} label={t("copyLink")} primary />
    </Dialog>
  );
}

function GeneratedQr({ url, makeQr }: { url: string; makeQr: (url: string) => Promise<QrImage> }) {
  const { t } = useEnv();
  const [img, setImg] = useState<QrImage | null | "error">(null);
  useEffect(() => {
    let live = true;
    makeQr(url).then(
      (r) => live && setImg(r),
      () => live && setImg("error"),
    );
    return () => {
      live = false;
    };
  }, [url, makeQr]);
  if (img === "error") return <p className="mt-4 text-sm text-muted">{t("qrUnavailable")}</p>;
  if (!img) return <span className="lp-skeleton mt-4 block h-60 w-60" aria-hidden="true" />;
  return <img src={img.src} width={img.size} height={img.size} alt={t("qrCode")} className="lp-qr-img mt-4" />;
}

const defaultMakeQr = (url: string): Promise<QrImage> => qrPng(url);

/** The single dialog outlet at the app root. */
export function DialogHost({ spec, onClose, makeQr = defaultMakeQr }: HostProps) {
  const { t, pick } = useEnv();
  if (!spec) return null;
  switch (spec.kind) {
    case "browser":
      return <BrowserOverlay url={spec.url} onClose={onClose} />;
    case "desktop":
      return <DesktopDialog url={spec.url} onClose={onClose} />;
    case "qr": {
      const note = pick(spec.qr.note);
      return (
        <Dialog title={spec.title} closeLabel={t("close")} onClose={onClose}>
          <img
            src={spec.qr.url}
            width={spec.qr.width}
            height={spec.qr.height}
            alt={t("qrCode")}
            className="lp-qr-img mt-4"
          />
          <p className="mt-3 text-sm text-muted">{note ? `${t("qrLongPress")} · ${note}` : t("qrLongPress")}</p>
        </Dialog>
      );
    }
    case "qrGenerate":
      return (
        <Dialog title={spec.title} closeLabel={t("close")} onClose={onClose}>
          <GeneratedQr url={spec.url} makeQr={makeQr} />
          <p className="mt-3 text-sm text-muted">{t("qrScanToJoin")}</p>
        </Dialog>
      );
  }
}
