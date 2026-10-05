/**
 * Block part of the public Markdown subset: paragraphs, ordered and
 * unordered lists (one level) and blockquotes. Headings, tables, code
 * blocks, thematic breaks and raw HTML are not supported and stay literal
 * paragraph text, matching the server renderer (internal/content).
 */
import { type Inline, parseInline } from "./inline";

export type MdBlock =
  | { t: "p"; c: Inline[] }
  | { t: "ul"; items: Inline[][]; loose: boolean }
  | { t: "ol"; start: number; items: Inline[][]; loose: boolean }
  | { t: "quote"; c: MdBlock[] };

const QUOTE = /^ {0,3}> ?(.*)$/;
const BULLET = /^ {0,3}[-*+][ \t]+(.*)$/;
const ORDERED = /^ {0,3}(\d{1,9})[.)][ \t]+(.*)$/;
const MAX_QUOTE_DEPTH = 8;

const isBlank = (line: string | undefined): boolean => line === undefined || line.trim() === "";
const startsBlock = (line: string): boolean => QUOTE.test(line) || BULLET.test(line) || ORDERED.test(line);

interface Cursor {
  lines: readonly string[];
  i: number;
}

/**
 * A blockquote. A plain line right after quoted paragraph text continues
 * that paragraph ("lazy continuation"), as in CommonMark.
 */
function quote(cur: Cursor, depth: number): MdBlock {
  const inner: string[] = [];
  while (cur.i < cur.lines.length) {
    const line = cur.lines[cur.i] as string;
    const m = QUOTE.exec(line);
    const lazy = !m && !isBlank(line) && !startsBlock(line) && !isBlank(inner[inner.length - 1]);
    if (!m && !lazy) break;
    inner.push(m ? (m[1] ?? "") : line);
    cur.i++;
  }
  if (depth >= MAX_QUOTE_DEPTH) return { t: "p", c: parseInline(inner.join("\n")) };
  return { t: "quote", c: blocks(inner, depth + 1) };
}

/**
 * One list (all items share the marker kind of the first item). Items
 * separated by a blank line make the list loose: like CommonMark, each
 * item's text is then wrapped in a paragraph.
 */
function list(cur: Cursor, ordered: boolean): MdBlock {
  const marker = ordered ? ORDERED : BULLET;
  const first = marker.exec(cur.lines[cur.i] as string);
  const start = ordered ? Number(first?.[1] ?? 1) : 1;
  const items: string[][] = [];
  let loose = false;
  while (cur.i < cur.lines.length) {
    const line = cur.lines[cur.i] as string;
    const m = marker.exec(line);
    if (m) {
      items.push([(ordered ? m[2] : m[1]) ?? ""]);
    } else if (isBlank(line)) {
      const next = cur.lines[cur.i + 1];
      if (next === undefined || !marker.test(next)) break;
      loose = true;
    } else if (startsBlock(line)) {
      break;
    } else {
      items[items.length - 1]?.push(line.trim());
    }
    cur.i++;
  }
  const parsed = items.map((it) => parseInline(it.join("\n")));
  return ordered ? { t: "ol", start, items: parsed, loose } : { t: "ul", items: parsed, loose };
}

function paragraph(cur: Cursor): MdBlock {
  const text: string[] = [];
  while (cur.i < cur.lines.length) {
    const line = cur.lines[cur.i] as string;
    if (isBlank(line) || (text.length > 0 && startsBlock(line))) break;
    text.push(line.trim());
    cur.i++;
  }
  return { t: "p", c: parseInline(text.join("\n")) };
}

function blocks(lines: readonly string[], depth: number): MdBlock[] {
  const cur: Cursor = { lines, i: 0 };
  const out: MdBlock[] = [];
  while (cur.i < lines.length) {
    const line = lines[cur.i] as string;
    if (isBlank(line)) cur.i++;
    else if (QUOTE.test(line)) out.push(quote(cur, depth));
    else if (BULLET.test(line)) out.push(list(cur, false));
    else if (ORDERED.test(line)) out.push(list(cur, true));
    else out.push(paragraph(cur));
  }
  return out;
}

/** Parse the public Markdown subset into a block tree. */
export function parseMarkdown(src: string): MdBlock[] {
  return blocks(src.replace(/\r\n?/g, "\n").split("\n"), 0);
}
