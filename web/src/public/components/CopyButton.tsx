import { useEnv } from "../context";
import { useCopy } from "../hooks/useCopy";
import { Glyph } from "./icons/glyphs";

interface CopyButtonProps {
  value: string;
  label: string;
  primary?: boolean;
  className?: string;
}

/** Copy `value` with visible and announced feedback ("已复制" / manual-copy hint). */
export function CopyButton({ value, label, primary, className }: CopyButtonProps) {
  const { t } = useEnv();
  const [status, copy] = useCopy();
  const text = status === "copied" ? t("copied") : status === "failed" ? t("copyFailed") : label;
  return (
    <button
      type="button"
      onClick={() => copy(value)}
      data-status={status}
      className={`lp-btn ${primary ? "lp-btn-primary" : ""} ${className ?? ""}`}
    >
      <Glyph name="copy" />
      <span aria-live="polite">{text}</span>
    </button>
  );
}
