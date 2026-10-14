---
name: antfu-eslint-config
description: Configuring @antfu/eslint-config 9 - options, Vue support, formatters, rule overrides, plugin prefixes and VS Code integration
---

# @antfu/eslint-config

Handles both linting and formatting (no Prettier needed). Auto-detects TypeScript and Vue.

**Upstream default style**: single quotes, no semicolons, sorted imports, dangling commas. oagent overrides it to use semicolons (`stylistic.semi: true`) and adds the rules described in the skill's SKILL.md.

## Configuration Options

```js
import antfu from '@antfu/eslint-config'

export default antfu({
  // Project type: 'lib' for libraries, 'app' (default) for applications
  type: 'lib',

  // Global ignores (extends defaults, doesn't override)
  ignores: ['**/fixtures', '**/dist'],

  // Stylistic options
  stylistic: {
    indent: 2,        // 2, 4, or 'tab'
    quotes: 'single', // or 'double'
    semi: true,       // oagent: semicolons on
  },

  // Framework support (auto-detected, but can be explicit)
  typescript: true,
  vue: true,

  // Disable specific language support (oagent: YAML is linted by yamllint)
  yaml: false,
})
```

## Framework Support

### Vue

oagent enables `vue: true`. Vue accessibility is available but not enabled:

```js
export default antfu({
  vue: {
    a11y: true
  },
})
// Requires: pnpm add -D eslint-plugin-vuejs-accessibility
```

## Formatters (CSS, HTML, Markdown)

For files ESLint doesn't handle natively:

```js
export default antfu({
  formatters: {
    css: true,      // Format CSS, LESS, SCSS (uses Prettier)
    html: true,     // Format HTML (uses Prettier)
    markdown: 'prettier' // or 'dprint'
  }
})
// Requires: pnpm add -D eslint-plugin-format
```

## Rule Overrides

### Global overrides

```js
export default antfu(
  {
    // First argument: antfu config options
  },
  // Additional arguments: ESLint flat configs
  {
    rules: {
      'curly': ['error', 'all'],
    },
  }
)
```

### Per-integration overrides

```js
export default antfu({
  vue: {
    overrides: {
      'vue/operator-linebreak': ['error', 'before'],
    },
  },
  typescript: {
    overrides: {
      'ts/consistent-type-definitions': ['error', 'interface'],
    },
  },
})
```

### File-specific overrides

```js
export default antfu(
  { vue: true, typescript: true },
  {
    files: ['**/*.vue'],
    rules: {
      'vue/operator-linebreak': ['error', 'before'],
    },
  }
)
```

## Plugin Prefix Renaming

The config renames plugin prefixes for consistency:

| New Prefix | Original |
|------------|----------|
| `ts/*` | `@typescript-eslint/*` |
| `style/*` | `@stylistic/*` |
| `import/*` | `import-lite/*` |
| `node/*` | `n/*` |
| `yaml/*` | `yml/*` |
| `test/*` | `vitest/*` |
| `next/*` | `@next/next` |

Use the new prefix when overriding or disabling rules:

```ts
// eslint-disable-next-line ts/consistent-type-definitions
type Foo = { bar: 2 }
```

## Type-Aware Rules

Enable TypeScript type checking:

```js
export default antfu({
  typescript: {
    tsconfigPath: 'tsconfig.json',
  },
})
```

## Config Composer API

Chain methods for flexible composition:

```js
export default antfu()
  .prepend(/* configs before main */)
  .override('antfu/stylistic/rules', {
    rules: {
      'style/generator-star-spacing': ['error', { after: true, before: false }],
    }
  })
  .renamePlugins({
    'old-prefix': 'new-prefix',
  })
```

## VS Code Settings

oagent's `.vscode/settings.json` already turns off Prettier and format-on-save and runs `source.fixAll.eslint` on save. The upstream recommendation, for reference:

```jsonc
{
  "prettier.enable": false,
  "editor.formatOnSave": false,
  "editor.codeActionsOnSave": {
    "source.fixAll.eslint": "explicit",
    "source.organizeImports": "never"
  },
  "eslint.rules.customizations": [
    { "rule": "style/*", "severity": "off", "fixable": true },
    { "rule": "format/*", "severity": "off", "fixable": true },
    { "rule": "*-indent", "severity": "off", "fixable": true },
    { "rule": "*-spacing", "severity": "off", "fixable": true },
    { "rule": "*-spaces", "severity": "off", "fixable": true },
    { "rule": "*-order", "severity": "off", "fixable": true },
    { "rule": "*-dangle", "severity": "off", "fixable": true },
    { "rule": "*-newline", "severity": "off", "fixable": true },
    { "rule": "*quotes", "severity": "off", "fixable": true },
    { "rule": "*semi", "severity": "off", "fixable": true }
  ],
  "eslint.validate": [
    "javascript",
    "javascriptreact",
    "typescript",
    "typescriptreact",
    "vue",
    "html",
    "markdown",
    "json",
    "jsonc",
    "yaml",
    "toml",
    "xml",
    "astro",
    "svelte",
    "css",
    "less",
    "scss"
  ]
}
```

<!-- 
Source references:
- https://github.com/antfu/eslint-config
- https://raw.githubusercontent.com/antfu/eslint-config/refs/heads/main/README.md
-->
