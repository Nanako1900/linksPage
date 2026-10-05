import "../styles/public.css";
import { installPreloadRecovery } from "../shared/preloadRecovery";
import { start } from "./start";

installPreloadRecovery({
  addEventListener: (type, fn) => window.addEventListener(type, fn),
  storage: () => window.sessionStorage,
  reload: () => window.location.reload(),
});

const root = document.getElementById("root");
if (root) void start(root);
