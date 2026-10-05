import { useCallback, useEffect, useRef, useState } from "react";
import { copyText } from "../../shared/clipboard";

export type CopyStatus = "idle" | "copied" | "failed";

/** How long "Copied" / "Copy failed" stays visible. */
export const COPY_FEEDBACK_MS = 2_000;

/** Copy with transient feedback; the status resets after COPY_FEEDBACK_MS. */
export function useCopy(copy: (text: string) => Promise<boolean> = copyText): [CopyStatus, (text: string) => void] {
  const [status, setStatus] = useState<CopyStatus>("idle");
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  const run = useCallback(
    (text: string) => {
      void copy(text).then((ok) => {
        clearTimeout(timer.current);
        setStatus(ok ? "copied" : "failed");
        timer.current = setTimeout(() => setStatus("idle"), COPY_FEEDBACK_MS);
      });
    },
    [copy],
  );
  return [status, run];
}
