// LinksPage topology C edge Worker (doc 2, 12.3; contract 9).

import type { Env } from "./env";
import { defaultDeps, handleRequest } from "./handler";

export default {
  fetch(request, env, ctx): Promise<Response> {
    return handleRequest(request, env, ctx, defaultDeps());
  },
} satisfies ExportedHandler<Env>;
