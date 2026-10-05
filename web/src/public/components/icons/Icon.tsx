import { useEffect, useState } from "react";
import type { IconView } from "../../../shared/types/public";
import { BUILTIN_MARKS, Svg } from "./glyphs";

type IconMap = Readonly<Record<string, string>>;

let simpleIcons: Promise<IconMap> | null = null;

/** Load the simple-icons chunk once; failures resolve to an empty map. */
export function loadSimpleIcons(
  load: () => Promise<{ SIMPLE_ICONS: IconMap }> = () => import("./simpleIcons"),
): Promise<IconMap> {
  simpleIcons ??= load().then(
    (m) => m.SIMPLE_ICONS,
    () => ({}),
  );
  return simpleIcons;
}

/** Test hook: forget the cached chunk. */
export function resetSimpleIcons(): void {
  simpleIcons = null;
}

/** undefined while loading, null for unknown slugs. */
function useSimplePath(slug: string | null): string | null | undefined {
  const [path, setPath] = useState<string | null | undefined>(undefined);
  useEffect(() => {
    if (slug === null) return;
    let live = true;
    void loadSimpleIcons().then((icons) => {
      if (live) setPath(icons[slug] ?? null);
    });
    return () => {
      live = false;
    };
  }, [slug]);
  return slug === null ? null : path;
}

function Monogram({ label, size, className }: { label: string; size: number; className?: string }) {
  const letter = [...label.trim()][0]?.toUpperCase() ?? "·";
  return (
    <span
      aria-hidden="true"
      className={`lp-monogram ${className ?? ""}`}
      style={{ width: size, height: size, fontSize: Math.round(size * 0.55) }}
    >
      {letter}
    </span>
  );
}

interface IconProps {
  icon: IconView | null;
  /** Used for the monogram fallback. */
  label: string;
  size: number;
  className?: string;
}

/** Platform or link icon: simple-icons path, builtin mark, uploaded image or monogram. */
export function Icon({ icon, label, size, className }: IconProps) {
  const path = useSimplePath(icon?.kind === "simple" ? icon.name : null);
  if (icon?.kind === "media" && icon.url) {
    return (
      <img src={icon.url} width={size} height={size} alt="" loading="lazy" decoding="async" className={className} />
    );
  }
  if (icon?.kind === "simple" && path !== null) {
    return (
      <Svg size={size} className={className}>
        {path && <path d={path} />}
      </Svg>
    );
  }
  const mark = icon?.kind === "builtin" ? BUILTIN_MARKS[icon.name] : undefined;
  if (mark) {
    return (
      <Svg size={size} className={className}>
        <path d={mark} fillRule="evenodd" />
      </Svg>
    );
  }
  return <Monogram label={label} size={size} className={className} />;
}
