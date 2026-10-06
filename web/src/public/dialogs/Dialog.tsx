import { type MouseEvent, type ReactNode, useCallback, useEffect, useId, useRef } from "react";
import { Glyph } from "../components/icons/glyphs";

interface DialogProps {
  title: string;
  /** Visually hide the title (it still labels the dialog). Overlays always hide it. */
  hideTitle?: boolean;
  closeLabel: string;
  onClose: () => void;
  /** "overlay" fills the viewport (open-in-browser guide). */
  variant?: "dialog" | "overlay";
  children: ReactNode;
}

/**
 * True when a mouse event hit the ::backdrop: its target is the <dialog>
 * itself (not a child) and the point lies outside the dialog box, so
 * clicks on the dialog's padding or scrollbar do not count.
 */
function onBackdrop(e: MouseEvent<HTMLDialogElement>): boolean {
  if (e.target !== e.currentTarget) return false;
  const r = e.currentTarget.getBoundingClientRect();
  return e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom;
}

/**
 * Modal built on <dialog>: showModal() gives the focus trap, inert
 * background and Esc handling. Without showModal (very old WebViews,
 * jsdom) it falls back to the `open` attribute with an Esc handler.
 */
export function Dialog({ title, hideTitle, closeLabel, onClose, variant = "dialog", children }: DialogProps) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (typeof el.showModal === "function") {
      if (!el.open) el.showModal();
    } else {
      el.setAttribute("open", "");
    }
  }, []);

  const requestClose = useCallback(() => {
    const el = ref.current;
    if (el && typeof el.close === "function" && el.open) el.close();
    else onClose();
  }, [onClose]);

  const overlay = variant === "overlay";
  // A press that started on the backdrop; a drag (text selection) that only
  // ends there must not close the dialog.
  const pressedBackdrop = useRef(false);
  const onPress = (e: MouseEvent<HTMLDialogElement>) => {
    pressedBackdrop.current = !overlay && onBackdrop(e);
  };
  const onClick = (e: MouseEvent<HTMLDialogElement>) => {
    const close = pressedBackdrop.current && onBackdrop(e);
    pressedBackdrop.current = false;
    if (close) requestClose();
  };

  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      className={overlay ? "lp-overlay" : "lp-dialog"}
      onClose={onClose}
      onMouseDown={onPress}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.preventDefault();
          requestClose();
        }
      }}
    >
      {overlay ? (
        <h2 id={titleId} className="sr-only">
          {title}
        </h2>
      ) : (
        <div className="flex items-start justify-between gap-4">
          <h2 id={titleId} className={hideTitle ? "sr-only" : "font-display text-2xl leading-tight"}>
            {title}
          </h2>
          <button
            type="button"
            onClick={requestClose}
            aria-label={closeLabel}
            className="lp-btn lp-btn-quiet -mt-2 -mr-2 ml-auto w-11 px-0 text-current"
          >
            <Glyph name="close" size={20} />
          </button>
        </div>
      )}
      {children}
      {overlay && (
        // The top-right corner belongs to the arrow; close sits under the content.
        <div className="mx-auto mt-4 max-w-[22rem]">
          <button type="button" onClick={requestClose} className="lp-btn lp-btn-quiet -ml-4 text-current">
            <Glyph name="close" size={18} />
            {closeLabel}
          </button>
        </div>
      )}
    </dialog>
  );
}
