import assert from "node:assert/strict";
import { test } from "node:test";
import stylelint from "stylelint";
import plugin, { dynamicUnit, ruleName } from "./dynamic-viewport-fallback.mjs";

async function lint(code, options = true) {
  const { results } = await stylelint.lint({
    code,
    config: { plugins: [plugin], rules: { [ruleName]: options } },
  });
  return results[0].warnings.map((w) => w.text);
}

test("dynamicUnit finds dynamic viewport units only", () => {
  const cases = [
    ["100dvh", "dvh"],
    ["calc(100svh - 2rem)", "svh"],
    ["min(50lvw, 10rem)", "lvw"],
    ["1.5dvmin", "dvmin"],
    ["100vh", null],
    ["var(--dvh)", null],
    ["url(a-100dvh.png)", null],
    ["10px", null],
  ];
  for (const [value, want] of cases) assert.equal(dynamicUnit(value), want, value);
});

test("rule accepts fallbacks and rejects bare dynamic units", async () => {
  const cases = [
    { name: "earlier fallback", code: "a{height:100vh;height:100dvh}", warnings: 0 },
    {
      name: "fallback with another prop between",
      code: "a{min-height:100vh;color:red;min-height:100dvh}",
      warnings: 0,
    },
    { name: "@supports guard", code: "@supports (min-height:100dvh){a{min-height:100dvh}}", warnings: 0 },
    {
      name: "nested @supports guard",
      code: "@layer x{@supports (height:1svh){a{height:calc(100svh - 1rem)}}}",
      warnings: 0,
    },
    { name: "no fallback", code: "a{height:100dvh}", warnings: 1 },
    { name: "fallback after", code: "a{height:100dvh;height:100vh}", warnings: 1 },
    { name: "fallback is dynamic too", code: "a{height:100svh;height:100dvh}", warnings: 2 },
    { name: "unrelated @supports", code: "@supports (display:grid){a{height:100dvh}}", warnings: 1 },
    { name: "other property", code: "a{min-height:100vh;height:100dvh}", warnings: 1 },
  ];
  for (const c of cases) assert.equal((await lint(c.code)).length, c.warnings, c.name);
  const [text] = await lint("a{max-height:calc(100dvh - 2rem)}");
  assert.match(text, /"max-height" uses dvh without a fallback/);
});

test("known selectors are tolerated until they are fixed", async () => {
  const known = [true, { known: [".lp-overlay"] }];
  assert.deepEqual(await lint(".lp-overlay{height:100dvh}", known), []);
  assert.deepEqual(await lint(".other{color:red}", known), []);
  const fixed = await lint(".lp-overlay{height:100vh;height:100dvh}", known);
  assert.equal(fixed.length, 1);
  assert.match(fixed[0], /"\.lp-overlay" now has a fallback: remove it/);
  assert.equal((await lint(".lp-overlay{height:100dvh}.x{height:1dvh}", known)).length, 1);
});

test("invalid options are rejected", async () => {
  const bad = await lint("a{height:100vh}", [true, { known: [1] }]);
  assert.equal(bad.length, 0);
  const { results } = await stylelint.lint({
    code: "a{}",
    config: { plugins: [plugin], rules: { [ruleName]: [true, { known: [1] }] } },
  });
  assert.ok(results[0].invalidOptionWarnings.length > 0);
});
