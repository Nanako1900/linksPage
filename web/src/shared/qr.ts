/**
 * Client-side QR codes as PNG data URLs (doc 3.2: uqr, dynamically
 * imported; WeChat/QQ long-press recognition only works on <img>).
 */

export interface QrImage {
  src: string;
  /** Pixel width = height of the PNG. */
  size: number;
}

type Encoder = (text: string, opts: { ecc: "M"; border: number }) => { size: number; data: boolean[][] };

export interface QrDeps {
  load: () => Promise<{ encode: Encoder }>;
  doc: Document;
}

const defaultDeps = (): QrDeps => ({ load: () => import("uqr"), doc: document });

/** Quiet zone in modules (the spec minimum is 4; 2 is plenty on a white tile). */
const MARGIN = 2;

/** Render `text` as a black-on-white QR PNG with `scale` pixels per module. */
export async function qrPng(text: string, scale = 8, deps: QrDeps = defaultDeps()): Promise<QrImage> {
  const { encode } = await deps.load();
  const qr = encode(text, { ecc: "M", border: 0 });
  const size = (qr.size + MARGIN * 2) * scale;
  const canvas = deps.doc.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("qr: 2D canvas is unavailable");
  ctx.fillStyle = "#ffffff";
  ctx.fillRect(0, 0, size, size);
  ctx.fillStyle = "#000000";
  qr.data.forEach((row, y) => {
    row.forEach((dark, x) => {
      if (dark) ctx.fillRect((x + MARGIN) * scale, (y + MARGIN) * scale, scale, scale);
    });
  });
  return { src: canvas.toDataURL("image/png"), size };
}
