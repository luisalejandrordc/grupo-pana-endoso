import type { components, operations } from "./schema.d";

/** Types generated from backend/api/openapi.yaml (run `npm run api:types` after the contract changes). */
export type Schemas = components["schemas"];

/** The generated description of every operation, for the shape of its query and its answers. */
export type Operations = operations;
