// Stylelint rule: dynamic viewport units (dvh, svh, lvh, dvw, …) need a
// fallback for Chrome 99–107 WebViews (doc 8.6), either
//   - an earlier declaration of the same property without them, or
//   - an enclosing @supports that tests a dynamic unit.
// The built CSS is checked too: the Tailwind optimizer may merge a
// "height: 100vh; height: 100dvh" pair, while @supports blocks survive.
//
// Secondary option `known`: selectors of tracked violations (see
// docs/m1/notes.md). A known selector that has a fallback again is
// reported, so the entry is removed together with the fix.
import stylelint from "stylelint";

const {
  createPlugin,
  utils: { report, ruleMessages, validateOptions },
} = stylelint;

export const ruleName = "linkspage/dynamic-viewport-fallback";

export const messages = ruleMessages(ruleName, {
  rejected: (prop, unit) =>
    `"${prop}" uses ${unit} without a fallback (add an earlier vh/vw declaration or wrap it in @supports)`,
  fixed: (selector) => `"${selector}" now has a fallback: remove it from the "known" option`,
});

const DYNAMIC_UNIT = /(?<![\w-])[+-]?(?:\d*\.)?\d+(?:[dsl]v(?:min|max|h|w|i|b))\b/i;

/** The dynamic unit used in `value`, or null. */
export function dynamicUnit(value) {
  const m = DYNAMIC_UNIT.exec(value);
  return m ? m[0].replace(/^[+-]?(?:\d*\.)?\d+/, "").toLowerCase() : null;
}

function insideSupportsTest(node) {
  for (let p = node.parent; p; p = p.parent) {
    if (p.type === "atrule" && p.name.toLowerCase() === "supports" && dynamicUnit(p.params)) return true;
  }
  return false;
}

function hasEarlierFallback(decl) {
  const prop = decl.prop.toLowerCase();
  for (let prev = decl.prev(); prev; prev = prev.prev()) {
    if (prev.type === "decl" && prev.prop.toLowerCase() === prop && !dynamicUnit(prev.value)) return true;
  }
  return false;
}

const isString = (v) => typeof v === "string";

const rule = (primary, secondary) => (root, result) => {
  const valid = validateOptions(
    result,
    ruleName,
    { actual: primary, possible: [true] },
    { actual: secondary, possible: { known: [isString] }, optional: true },
  );
  if (!valid) return;
  const known = new Set(secondary?.known ?? []);
  const knownSeen = new Map();

  root.walkDecls((decl) => {
    const unit = dynamicUnit(decl.value);
    if (!unit) return;
    const selector = decl.parent?.type === "rule" ? decl.parent.selector : "";
    const ok = insideSupportsTest(decl) || hasEarlierFallback(decl);
    if (known.has(selector)) {
      const state = knownSeen.get(selector) ?? { node: decl.parent, bad: false };
      knownSeen.set(selector, { ...state, bad: state.bad || !ok });
      return;
    }
    if (!ok) report({ result, ruleName, node: decl, message: messages.rejected(decl.prop, unit) });
  });

  for (const [selector, { node, bad }] of knownSeen) {
    if (!bad) report({ result, ruleName, node, message: messages.fixed(selector) });
  }
};

rule.ruleName = ruleName;
rule.messages = messages;
rule.meta = { url: "https://github.com/Nanako1900/linksPage/blob/main/docs/m1/notes.md" };

export default createPlugin(ruleName, rule);
