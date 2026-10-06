// Browser compatibility gate for the public page (doc 8.1, `pnpm lint:css`).
// Only compatibility rules are enabled: formatting is Biome's job.
//
// Checked: the public CSS sources, the Go-rendered stylesheets
// (critical CSS of the fallback markup, the /go guide page) and the built
// public CSS. The browserslist query is passed explicitly because the Go
// stylesheets live outside web/ and would not pick up .browserslistrc.
import { readFileSync } from "node:fs";

const browsers = readFileSync(new URL("./.browserslistrc", import.meta.url), "utf8")
  .split("\n")
  .map((line) => line.replace(/#.*/, "").trim())
  .filter(Boolean)
  .join(", ");

// Features reported by doiuse that are safe here:
const IGNORED_FEATURES = [
  // Unprefixed text-size-adjust is ignored by Safari; the -webkit- form
  // (written next to it, and in Tailwind's preflight) is what applies.
  "text-size-adjust",
  // ui-monospace / ui-sans-serif only appear inside stacks with fallbacks.
  "extended-system-fonts",
  // Flagged for the multi-value shorthand; only `underline` plus
  // text-decoration-thickness / text-underline-offset are used.
  "text-decoration",
  // dvh/svh fallbacks are checked precisely by linkspage/dynamic-viewport-fallback.
  "viewport-unit-variants",
];

// Violations tracked in docs/m1/notes.md (已知问题); the rule reports
// entries that no longer reproduce. Empty since the dialog/overlay dvh
// fallbacks landed.
const KNOWN_VIEWPORT_ISSUES = [];

const compat = (ignorePartialSupport, ignore) => [true, { browsers, ignore, ignorePartialSupport, severity: "error" }];

export default {
  plugins: ["stylelint-no-unsupported-browser-features", "./stylelint/dynamic-viewport-fallback.mjs"],
  rules: {
    "plugin/no-unsupported-browser-features": compat(false, IGNORED_FEATURES),
    "linkspage/dynamic-viewport-fallback": [true, { known: KNOWN_VIEWPORT_ISSUES }],
  },
  overrides: [
    {
      // Built output: Tailwind's preflight adds partially supported
      // properties (text-indent, columns, clip-path) and textarea resize
      // that degrade harmlessly; full support gaps are still errors.
      files: ["dist/**/*.css"],
      rules: {
        "plugin/no-unsupported-browser-features": compat(true, [...IGNORED_FEATURES, "css-resize"]),
      },
    },
  ],
};
