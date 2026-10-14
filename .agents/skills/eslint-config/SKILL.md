---
name: eslint-config
description: oagent's ESLint setup (@antfu/eslint-config 9 on ESLint 10, one root eslint.config.mjs, @nuxt/eslint in ui/console) and the JS/TS coding style it enforces. Use when `pnpm lint` or the lint-staged pre-commit hook fails, when fixing a style/*, curly, vue/* or pnpm/* lint error, when adding an ignore or rule override, when a file type is not linted as expected, or when writing new TS/Vue code that must pass lint on the first try.
metadata:
  version: "2026.09.30"
  source: Adapted from antfu/skills@feb0f3a (MIT), generated from https://github.com/antfu/eslint-config
---

# ESLint config

## In oagent

- One root `eslint.config.mjs` exports `oagent()`, built on `@antfu/eslint-config` 9 with ESLint 10. It lints every JS/TS/Vue/JSON/Markdown/CSS file in the repo.
- `ui/console/eslint.config.mjs` wraps it: `withNuxt(oagent())` adds the `@nuxt/eslint` rules (`eslint: { config: { standalone: false } }` in `nuxt.config.ts`). It needs `.nuxt/` (run `nuxt prepare`, done by `postinstall`).
- YAML is not ESLint's job (`yaml: false`); yamllint checks it. Go, shell, Dockerfiles and protos have their own linters (see lint-staged in the root `package.json`).
- Commands: `pnpm lint` (`eslint .`), `pnpm lint:fix` (`eslint . --fix`), `make lint-js`. The husky pre-commit hook runs `lint-staged`, which runs `eslint --fix --no-warn-ignored` on staged files.
- pnpm 10 workspace: `pnpm/json-enforce-catalog` is off, because the `catalog:` holds only versions shared by several `ui/*` packages.
- The user commits; never commit for them and never add AI attribution. See docs/howto/README.md.

## What the config enforces

The `oagent()` factory in the root `eslint.config.mjs`:

```js
antfu(
  {
    type: 'app',
    vue: true,
    typescript: true,
    yaml: false,
    stylistic: { indent: 2, quotes: 'single', semi: true },
    formatters: { css: true, html: true, markdown: 'prettier' },
    ignores: [/* gen, .nuxt, .output, dist, storybook-static, coverage, testdata, embedded static output, docs/superpowers, .agents, .claude, ... */],
  },
  { files: ['**/package.json'], rules: { 'pnpm/json-enforce-catalog': 'off' } },
  { files: ['**/*.{js,mjs,cjs,ts,mts,vue}'], rules: { /* curly, brace-style, member-delimiter-style */ } },
  { files: [/* pages, layouts, components/ui, app.vue, error.vue */], rules: { 'vue/multi-word-component-names': 'off' } },
);
```

Style that follows from it:

- 2-space indent, single quotes, semicolons, trailing commas on multiline literals, sorted imports (type imports first).
- `curly: all` and `style/brace-style: 1tbs` without single-line blocks: no one-line `if`. Always:

  ```ts
  if (!org) {
    return;
  }
  ```

- `interface` fields end with `;`. Inline object types (props, parameters, type literals) separate members with `,`:

  ```ts
  export interface Org {
    id: string;
    slug: string;
  }

  const { orgId, projectId } = defineProps<{ orgId: string, projectId: string }>();
  ```

- Single-word component names are allowed only in pages, layouts, `components/ui/`, `app.vue` and `error.vue`.
- Markdown, CSS and HTML are formatted through `eslint-plugin-format` (Prettier for Markdown); code blocks inside Markdown are linted too.

## Working with it

- Run `pnpm lint:fix` first; most `style/*` errors fix themselves. Fix the rest by hand.
- Do not silence a rule with `eslint-disable` to get past a failure. If a rule truly does not fit a file group, add a scoped override in `eslint.config.mjs` with a one-line reason comment.
- New generated or vendored output goes into `ignores` in the root config, not into per-file disables.
- Use the renamed prefixes in overrides and disables: `ts/`, `style/`, `import/`, `node/`, `test/` (see the reference).
- Do not add Prettier, a second ESLint config, or `antislop`/UnoCSS integrations; they are not part of oagent's setup.

## Coding practices

- **Single responsibility**: each file has one clear purpose; split a file when it handles too many concerns.
- **Explicit imports**: oagent narrows Nuxt auto-imports (`components.dirs: []`, `imports.scan: false`); import components, stores and own composables from their files. Use the aliases already configured (`~`, `~~`, `@shared/*`); do not invent new ones.
- **TypeScript**: extract complex inline types into a named `type` or `interface`; declare return types on exported functions when inference is not obvious.
- **Comments**: explain why, not what; follow the comment-discipline skill.
- **Tests (Vitest)**: `foo.ts` gets `foo.test.ts` in the same folder; use `describe`/`it` (not `test`), as the existing tests do.

## References

| Topic | Description | Reference |
|-------|-------------|-----------|
| @antfu/eslint-config | Options, Vue support, formatters, rule overrides, plugin prefixes, composer API, VS Code | [antfu-eslint-config](references/antfu-eslint-config.md) |
