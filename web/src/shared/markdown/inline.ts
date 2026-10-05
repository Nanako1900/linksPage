/**
 * Inline part of the public Markdown subset (docs/m1/contract.md 5.4):
 * emphasis, strong, strikethrough, inline code, inline links
 * (https/http/mailto) and line breaks. Everything else (autolinks,
 * reference links, raw HTML) stays literal text, exactly like the server
 * renderer (internal/content); both are pinned by the shared corpus
 * src/test/fixtures/markdown-cases.json. The output is a plain tree
 * rendered with React elements, so nothing is ever interpreted as HTML.
 */
export type Inline =
  | { t: "text"; v: string }
  | { t: "br" }
  | { t: "code"; v: string }
  | { t: "em" | "strong" | "del"; c: Inline[] }
  | { t: "a"; href: string; c: Inline[] };

const MAX_URL_LENGTH = 2048;
const SAFE_SCHEME = /^(https?:\/\/|mailto:)/i;
const ESCAPABLE = /^[!-/:-@[-`{-~]$/;

/** Returns the URL when its scheme is allowed, otherwise null. */
export function safeHref(raw: string): string | null {
  const url = raw.trim();
  if (url.length === 0 || url.length > MAX_URL_LENGTH) return null;
  // biome-ignore lint/suspicious/noControlCharactersInRegex: rejecting control characters is the point
  if (/[\s\u0000-\u001f\u007f]/.test(url)) return null;
  return SAFE_SCHEME.test(url) ? url : null;
}

const isSpace = (ch: string | undefined): boolean => ch === undefined || /\s/.test(ch);
const isWordChar = (ch: string | undefined): boolean => ch !== undefined && /[\p{L}\p{N}]/u.test(ch);

/** Find the end of a backtick code span starting at `at`, or -1. */
function codeSpanEnd(s: string, at: number): { end: number; ticks: number } | null {
  let ticks = 0;
  while (s[at + ticks] === "`") ticks++;
  const fence = "`".repeat(ticks);
  let j = s.indexOf(fence, at + ticks);
  while (j !== -1) {
    let run = 0;
    while (s[j + run] === "`") run++;
    if (run === ticks) return { end: j, ticks };
    j = s.indexOf(fence, j + run);
  }
  return null;
}

/**
 * Find the closing delimiter `delim` for an opener ending at `from`. Code
 * spans, escapes and nested strong runs are skipped so "*a **b** c*" closes
 * on the last star.
 */
function findCloser(s: string, from: number, delim: string): number {
  const ch = delim.charAt(0);
  for (let j = from; j < s.length; j++) {
    const c = s[j];
    if (c === "\\") {
      j++;
      continue;
    }
    if (c === "`") {
      const span = codeSpanEnd(s, j);
      if (span) j = span.end + span.ticks - 1;
      continue;
    }
    if (c !== ch) continue;
    if (delim.length === 1 && s[j + 1] === ch) {
      const inner = findCloser(s, j + 2, ch + ch);
      if (inner !== -1) {
        j = inner + 1;
        continue;
      }
    }
    if (!s.startsWith(delim, j) || isSpace(s[j - 1]) || j === from) continue;
    if (ch === "_" && isWordChar(s[j + delim.length])) continue;
    return j;
  }
  return -1;
}

type Step = { node: Inline | Inline[]; next: number } | null;

function emphasis(s: string, at: number): Step {
  const ch = s[at] as string;
  if (ch === "_" && isWordChar(s[at - 1])) return null;
  const forms: Array<[string, "strong" | "em" | "del"]> =
    ch === "~"
      ? [
          ["~~", "del"],
          ["~", "del"],
        ]
      : [
          [ch + ch, "strong"],
          [ch, "em"],
        ];
  for (const [delim, t] of forms) {
    if (!s.startsWith(delim, at) || isSpace(s[at + delim.length])) continue;
    const close = findCloser(s, at + delim.length, delim);
    if (close === -1) continue;
    return { node: { t, c: parseInline(s.slice(at + delim.length, close)) }, next: close + delim.length };
  }
  return null;
}

function codeSpan(s: string, at: number): Step {
  const span = codeSpanEnd(s, at);
  if (!span) return null;
  let v = s.slice(at + span.ticks, span.end).replace(/\n/g, " ");
  if (v.length > 2 && v.startsWith(" ") && v.endsWith(" ") && v.trim() !== "") v = v.slice(1, -1);
  return { node: { t: "code", v }, next: span.end + span.ticks };
}

/** Index of the "]" matching the "[" at `at`, honoring nesting and escapes. */
function closingBracket(s: string, at: number): number {
  let depth = 0;
  for (let j = at; j < s.length; j++) {
    if (s[j] === "\\") j++;
    else if (s[j] === "[") depth++;
    else if (s[j] === "]" && --depth === 0) return j;
  }
  return -1;
}

const TITLE_CLOSE: Record<string, string> = { '"': '"', "'": "'", "(": ")" };
const MAX_PAREN_DEPTH = 32;

const skipSpace = (s: string, at: number): number => {
  let i = at;
  while (s[i] === " " || s[i] === "\t" || s[i] === "\n") i++;
  return i;
};

/** Unescape backslash-escaped ASCII punctuation, as CommonMark does in destinations. */
const unescapePunct = (v: string): string => v.replace(/\\([!-/:-@[-`{-~])/g, "$1");

/** `<...>` or a raw destination with balanced parentheses; returns [url, end]. */
function destination(s: string, at: number): [string, number] | null {
  if (s[at] === "<") {
    const close = s.indexOf(">", at + 1);
    const v = close === -1 ? "" : s.slice(at + 1, close);
    return close === -1 || /[<\n]/.test(v) ? null : [unescapePunct(v), close + 1];
  }
  let depth = 0;
  let i = at;
  for (; i < s.length; i++) {
    const c = s[i] as string;
    // biome-ignore lint/suspicious/noControlCharactersInRegex: control characters end a destination
    if (/[\s\u0000-\u001f\u007f]/.test(c)) break;
    if (c === "\\" && i + 1 < s.length) i++;
    else if (c === "(") depth++;
    else if (c === ")") {
      if (depth === 0) break;
      depth--;
    }
    if (depth > MAX_PAREN_DEPTH) return null;
  }
  return depth > 0 ? null : [unescapePunct(s.slice(at, i)), i];
}

/** Optional link title ("t", 't' or (t)); returns the index after it, or -1. */
function title(s: string, at: number): number {
  const close = TITLE_CLOSE[s[at] as string];
  if (!close) return at;
  for (let i = at + 1; i < s.length; i++) {
    if (s[i] === "\\") i++;
    else if (s[i] === close) return i + 1;
  }
  return -1;
}

/** Inline link tail `(dest "title")` starting at `at`; returns [url, end]. */
function linkTail(s: string, at: number): [string, number] | null {
  if (s[at] !== "(") return null;
  const d = destination(s, skipSpace(s, at + 1));
  if (!d) return null;
  let i = skipSpace(s, d[1]);
  if (i > d[1]) i = title(s, i);
  if (i === -1) return null;
  i = skipSpace(s, i);
  return s[i] === ")" ? [d[0], i + 1] : null;
}

/** `[label](url)`; `![alt](src)` renders only its alt text (no images). */
function link(s: string, at: number, image: boolean): Step {
  const open = image ? at + 1 : at;
  const close = closingBracket(s, open);
  if (close === -1) return null;
  const tail = linkTail(s, close + 1);
  if (!tail) return null;
  const label = parseInline(s.slice(open + 1, close));
  const href = image ? null : safeHref(tail[0]);
  return { node: href ? { t: "a", href, c: label } : label, next: tail[1] };
}

function escapeOrBreak(s: string, at: number): Step {
  const next = s[at + 1];
  if (next === "\n") return { node: { t: "br" }, next: at + 2 };
  if (next !== undefined && ESCAPABLE.test(next)) return { node: { t: "text", v: next }, next: at + 2 };
  return null;
}

function step(s: string, at: number): Step {
  switch (s[at]) {
    case "\\":
      return escapeOrBreak(s, at);
    case "`":
      return codeSpan(s, at);
    case "*":
    case "_":
    case "~":
      return emphasis(s, at);
    case "[":
      return link(s, at, false);
    case "!":
      return s[at + 1] === "[" ? link(s, at, true) : null;
    case "\n":
      return { node: { t: "br" }, next: at + 1 };
    default:
      return null;
  }
}

function pushText(out: Inline[], v: string): void {
  const last = out[out.length - 1];
  if (last?.t === "text") out[out.length - 1] = { t: "text", v: last.v + v };
  else out.push({ t: "text", v });
}

/** Parse inline Markdown. Newlines are hard breaks (contract: "换行"). */
export function parseInline(src: string): Inline[] {
  const s = src.replace(/[ \t]+\n/g, "\n");
  const out: Inline[] = [];
  let i = 0;
  while (i < s.length) {
    const r = step(s, i);
    if (!r) {
      pushText(out, s[i] as string);
      i++;
      continue;
    }
    for (const n of Array.isArray(r.node) ? r.node : [r.node]) {
      if (n.t === "text") pushText(out, n.v);
      else out.push(n);
    }
    i = r.next;
  }
  return out;
}
