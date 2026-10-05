/**
 * Surface an unexpected error without console logging: window.reportError
 * reaches window.onerror and any error-monitoring hook.
 */
export function reportFailure(error: unknown): void {
  if (typeof window.reportError === "function") window.reportError(error);
}
