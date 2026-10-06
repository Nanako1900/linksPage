/**
 * Copy text to the clipboard. Uses the async Clipboard API when available
 * (secure contexts) and falls back to a hidden textarea plus
 * document.execCommand("copy"), which older in-app WebViews still need.
 * Resolves to false when both fail; callers show a manual-copy hint.
 */
export async function copyText(text: string, doc: Document = document): Promise<boolean> {
  const clipboard = doc.defaultView?.navigator.clipboard;
  if (clipboard && typeof clipboard.writeText === "function") {
    try {
      await clipboard.writeText(text);
      return true;
    } catch {
      // Permission denied or not focused: try the legacy path.
    }
  }
  return legacyCopy(text, doc);
}

function legacyCopy(text: string, doc: Document): boolean {
  if (typeof doc.execCommand !== "function") return false;
  const area = doc.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  // Keep it out of view without display:none (which prevents selection).
  area.style.position = "fixed";
  area.style.top = "0";
  area.style.left = "0";
  area.style.opacity = "0";
  area.style.fontSize = "16px"; // avoids iOS zoom on focus
  const active = doc.activeElement;
  doc.body.append(area);
  try {
    area.select();
    area.setSelectionRange(0, text.length);
    return doc.execCommand("copy");
  } catch {
    return false;
  } finally {
    area.remove();
    if (active instanceof HTMLElement) active.focus();
  }
}
