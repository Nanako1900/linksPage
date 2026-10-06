/**
 * Small built-in icons. UI glyphs are stroked; the builtin platform marks
 * (IconView kind "builtin") are filled monochrome letterforms, not the
 * vendors' logos.
 */
import type { ReactNode } from "react";

export type GlyphName = "copy" | "qr" | "monitor" | "close" | "chevron";

const GLYPHS: Readonly<Record<GlyphName, string>> = {
  copy: "M9 9h11v11H9zM5 15V5a1 1 0 0 1 1-1h9",
  qr: "M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h2v2h-2zM18 18h2v2h-2zM18 14h2M14 18v2",
  monitor: "M3 4h18v12H3zM8 20h8M12 16v4",
  close: "M6 6l12 12M18 6 6 18",
  chevron: "M6 9l6 6 6-6",
};

/** Filled 24×24 marks for IconView kind "builtin". */
export const BUILTIN_MARKS: Readonly<Record<string, string>> = {
  kook: "M5 3h3.2v7.6L14.6 3h3.9l-6.6 7.7L19 21h-3.9l-5.4-7.9-1.5 1.7V21H5z",
  revolt: "M6 3h7a5 5 0 0 1 1.7 9.7L18.2 21h-3.4l-3.2-8H9.2v8H6zm3.2 3v4H13a2 2 0 0 0 0-4z",
  link: "M10.6 13.4a1 1 0 0 1 0-1.4l3.5-3.5a1 1 0 1 1 1.4 1.4L12 13.4a1 1 0 0 1-1.4 0zM8.5 19.8a4.3 4.3 0 0 1-3-7.3l2.1-2.1A1 1 0 1 1 9 11.8l-2.1 2.1a2.3 2.3 0 0 0 3.2 3.2l2.1-2.1a1 1 0 1 1 1.4 1.4l-2.1 2.1a4.3 4.3 0 0 1-3 1.3zm7.9-5.4a1 1 0 0 1-.7-1.7l2.1-2.1a2.3 2.3 0 0 0-3.2-3.2l-2.1 2.1A1 1 0 1 1 11 8.1l2.1-2.1a4.3 4.3 0 0 1 6.1 6.1l-2.1 2.1a1 1 0 0 1-.7.2z",
};

interface SvgProps {
  size: number;
  className?: string;
  children: ReactNode;
  stroke?: boolean;
}

export function Svg({ size, className, children, stroke }: SvgProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      aria-hidden="true"
      focusable="false"
      className={className}
      fill={stroke ? "none" : "currentColor"}
      stroke={stroke ? "currentColor" : undefined}
      strokeWidth={stroke ? 1.75 : undefined}
      strokeLinecap={stroke ? "round" : undefined}
      strokeLinejoin={stroke ? "round" : undefined}
    >
      {children}
    </svg>
  );
}

export function Glyph({ name, size = 16, className }: { name: GlyphName; size?: number; className?: string }) {
  return (
    <Svg size={size} className={className} stroke>
      <path d={GLYPHS[name]} />
    </Svg>
  );
}
