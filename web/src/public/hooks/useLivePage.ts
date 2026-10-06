import { useEffect, useRef, useState } from "react";
import { type Fetch, fetchPage } from "../../shared/bootstrap";
import { reconcileLive, startLivePoller } from "../../shared/live";
import type { LiveDTO, PublicPage } from "../../shared/types/public";

export interface LivePageDeps {
  fetch: Fetch;
  doc: Document;
  now: () => number;
}

const defaultDeps = (): LivePageDeps => ({ fetch: (i, init) => fetch(i, init), doc: document, now: Date.now });

/**
 * Keep the page's live data fresh (contract section 8): merge
 * /api/v1/public/live responses; reload the full bootstrap when the
 * revision changed. Reload failures keep the current page and are retried
 * on the next poll (the poller does not store the ETag of that response).
 */
export function useLivePage(initial: PublicPage, deps: LivePageDeps = defaultDeps()): PublicPage {
  const [page, setPage] = useState(initial);
  const pageRef = useRef(page);
  useEffect(() => {
    pageRef.current = page;
  }, [page]);
  const depsRef = useRef(deps);
  useEffect(() => {
    const { fetch, doc, now } = depsRef.current;
    // Resolves true when the new revision was loaded; a failure keeps the
    // current page and makes the poller retry on its next tick.
    const reload = (): Promise<boolean> =>
      fetchPage(fetch).then(
        (next) => {
          setPage(next);
          return true;
        },
        () => false,
      );
    const onLive = (dto: LiveDTO): boolean | Promise<boolean> => {
      const out = reconcileLive(pageRef.current, dto);
      if (out.kind === "reload") return reload();
      if (out.page !== pageRef.current) setPage(out.page);
      return true;
    };
    return startLivePoller({ fetch, doc, now, onLive });
  }, []);
  return page;
}
