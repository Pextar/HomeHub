// Flat ESLint config for the Svelte 5 + TypeScript frontend.
// Layered: JS recommended → typescript-eslint recommended → Svelte
// recommended, with eslint-config-prettier last so formatting concerns are
// left entirely to Prettier and never reported as lint errors.
import js from "@eslint/js";
import tseslint from "typescript-eslint";
import svelte from "eslint-plugin-svelte";
import globals from "globals";
import prettier from "eslint-config-prettier";
import svelteConfig from "./svelte.config.js";

export default tseslint.config(
	{
		// Generated and vendored output is never linted.
		ignores: ["dist/", "dev-dist/", "node_modules/"],
	},
	js.configs.recommended,
	...tseslint.configs.recommended,
	...svelte.configs.recommended,
	prettier,
	...svelte.configs.prettier,
	{
		languageOptions: {
			globals: { ...globals.browser, ...globals.node },
		},
	},
	{
		// Point the Svelte parser at the TS parser so `<script lang="ts">`
		// blocks type-resolve, and hand it the project's svelte.config.js.
		files: ["**/*.svelte", "**/*.svelte.ts", "**/*.svelte.js"],
		languageOptions: {
			parserOptions: {
				parser: tseslint.parser,
				svelteConfig,
			},
		},
		rules: {
			// Bare identifier references inside `$effect(() => { dep; ... })`
			// register reactive dependencies — idiomatic Svelte 5, not a
			// genuinely unused expression.
			"@typescript-eslint/no-unused-expressions": "off",
			// TypeScript resolves DOM lib types (e.g. CanvasImageSource);
			// eslint's no-undef can't see them and only false-positives here.
			"no-undef": "off",
		},
	},
	{
		// Pragmatic relaxations for an app codebase: the catch-all `any`
		// escape hatch and intentionally-unused args (prefixed `_`) are
		// allowed rather than blocking the build.
		rules: {
			"@typescript-eslint/no-explicit-any": "off",
			"@typescript-eslint/no-unused-vars": [
				"warn",
				{
					argsIgnorePattern: "^_",
					varsIgnorePattern: "^_",
					caughtErrorsIgnorePattern: "^_",
				},
			],
			// Introduced on an existing codebase: these surface real
			// best-practice gaps (keyed {#each}, Svelte-reactive Map/Set/Date,
			// dead assignments) but shouldn't block CI on day one. Kept as
			// warnings so they're visible and can be cleared incrementally;
			// new violations of the stricter default rules still fail the build.
			"svelte/require-each-key": "warn",
			"svelte/prefer-svelte-reactivity": "warn",
			"svelte/no-unused-svelte-ignore": "warn",
			"no-useless-assignment": "warn",
			"no-constant-binary-expression": "warn",
			// Empty catch blocks are a deliberate "best effort, ignore
			// failure" idiom here (e.g. releasePointerCapture).
			"no-empty": ["error", { allowEmptyCatch: true }],
		},
	},
);
