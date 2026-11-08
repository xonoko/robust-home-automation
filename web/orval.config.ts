import { defineConfig } from "orval";

// Generates the typed fetch client from the hub's OpenAPI document
// (openapi.json, written by `hub openapi`). Run `make generate` after
// changing the API.
export default defineConfig({
  hub: {
    input: "./openapi.json",
    output: {
      target: "./src/api/generated.ts",
      client: "fetch",
      mode: "single",
      baseUrl: "",
    },
  },
});
