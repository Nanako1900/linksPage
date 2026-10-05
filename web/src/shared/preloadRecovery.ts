const RELOAD_FLAG = "lp_preload_reload";

interface RecoveryEnv {
  addEventListener: (type: "vite:preloadError", fn: (e: Event) => void) => void;
  storage: () => Pick<Storage, "getItem" | "setItem">;
  reload: () => void;
}

/**
 * After a release, old chunks disappear; reload once when a dynamic import
 * fails to preload. A session flag prevents reload loops, so without
 * usable storage (some in-app browsers throw) no reload is attempted.
 */
export function installPreloadRecovery(env: RecoveryEnv): void {
  env.addEventListener("vite:preloadError", (event) => {
    try {
      const storage = env.storage();
      if (storage.getItem(RELOAD_FLAG)) return;
      storage.setItem(RELOAD_FLAG, "1");
    } catch {
      return;
    }
    event.preventDefault();
    env.reload();
  });
}
