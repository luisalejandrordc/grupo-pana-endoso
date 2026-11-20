import js from "@eslint/js";
import globals from "globals";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["**/dist", "**/node_modules", "packages/api/src/schema.d.ts"] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      ecmaVersion: 2022,
      globals: { ...globals.browser, ...globals.node },
    },
    plugins: {
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "react-refresh/only-export-components": [
        "warn",
        { allowConstantExport: true },
      ],
      "@typescript-eslint/consistent-type-imports": [
        "error",
        { fixStyle: "inline-type-imports" },
      ],
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
    },
  },
  // ---- the dependency rules of the workspace: apps → packages, never the other way, never sideways ----
  boundary(
    ["packages/ui/**/*.{ts,tsx}"],
    ["@endosoflow/*"],
    "The design system depends on no other EndosoFlow package.",
  ),
  boundary(
    ["packages/api/**/*.{ts,tsx}"],
    ["@endosoflow/*"],
    "The API package depends on no other EndosoFlow package.",
  ),
  boundary(
    ["apps/web/**/*.{ts,tsx}"],
    ["@endosoflow/portal", "@endosoflow/portal/*"],
    "Apps never import each other: move what they share into a package.",
  ),
  boundary(
    ["apps/portal/**/*.{ts,tsx}"],
    ["@endosoflow/web", "@endosoflow/web/*"],
    "Apps never import each other: move what they share into a package.",
  ),
);

/** Forbids importing the given packages from the given files. */
function boundary(files, group, message) {
  return {
    files,
    rules: {
      "no-restricted-imports": ["error", { patterns: [{ group, message }] }],
    },
  };
}
