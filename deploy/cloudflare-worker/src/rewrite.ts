// HTMLRewriter pipeline over the static index.html (contract 9, steps 1-4).

import type { RenderDTO } from "./dto";
import { serializeScriptJSON } from "./json";

/** Mirrors web/src/shared/bootstrap.ts BOOTSTRAP_ELEMENT_ID. */
export const DATA_ELEMENT_ID = "lp-data";

const removeElement: HTMLRewriterElementContentHandlers = {
  element(el) {
    el.remove();
  },
};

const removeComments: HTMLRewriterDocumentContentHandlers = {
  comments(comment) {
    comment.remove();
  },
};

export function dataScript(data: Readonly<Record<string, unknown>>): string {
  return `<script id="${DATA_ELEMENT_ID}" type="application/json">${serializeScriptJSON({ data })}</script>`;
}

/**
 * Rewrites the shell with the origin rendering. A 404 rendering (data null)
 * also drops the SPA entry and its preloads, like the origin's own 404
 * page: the server-rendered page is final and must not boot the app.
 */
export function rewriteShell(shell: Response, dto: RenderDTO): Response {
  const rewriter = new HTMLRewriter();
  if (dto.data === null) {
    rewriter.on('script[type="module"]', removeElement).on('link[rel="modulepreload"]', removeElement);
  }
  return rewriter
    .onDocument(removeComments)
    .on("html", {
      element(el) {
        el.setAttribute("lang", dto.lang);
        el.setAttribute("data-appearance", dto.appearance);
      },
    })
    .on("head title", removeElement)
    .on('head meta[name="description"]', removeElement)
    .on(`script#${DATA_ELEMENT_ID}`, removeElement)
    .on("head", {
      element(el) {
        el.append(dto.head, { html: true });
      },
    })
    .on("#root", {
      element(el) {
        el.setInnerContent(dto.fallback, { html: true });
      },
    })
    .on("body", {
      element(el) {
        if (dto.data !== null) {
          el.append(dataScript(dto.data), { html: true });
        }
      },
    })
    .transform(shell);
}

/** Shell served as-is (minus build comments) when the origin is unavailable. */
export function plainShell(shell: Response): Response {
  return new HTMLRewriter().onDocument(removeComments).transform(shell);
}
