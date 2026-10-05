import { defineConfig } from "orval";

// Typed fetch client generated from the Go server's OpenAPI document
// (`linkspage openapi > web/openapi.json`, see `make gen`).
export default defineConfig({
  linkspage: {
    input: { target: "./openapi.json" },
    output: {
      mode: "split",
      client: "fetch",
      target: "./src/shared/api/gen/linkspage.ts",
      schemas: "./src/shared/api/gen/model",
      clean: true,
      override: {
        fetch: { includeHttpResponseReturnType: true },
      },
    },
  },
});
