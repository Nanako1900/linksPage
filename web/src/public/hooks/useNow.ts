import { useEffect, useState } from "react";

/** Current time in ms, refreshed every `intervalMs` (default one minute). */
export function useNow(initial: number, intervalMs = 60_000): number {
  const [now, setNow] = useState(initial);
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}
