import createClient from "openapi-fetch";
import type { components, paths } from "./schema";

// Typed client for the hub's HTTP API. schema.d.ts is generated from the
// hub's OpenAPI document: run `make generate` after changing the API.
export const api = createClient<paths>({ baseUrl: "" });

export type Readiness = components["schemas"]["Readiness"];
export type VersionInfo = components["schemas"]["Info"];
