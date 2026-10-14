import antfu from '@antfu/eslint-config';

// One config for every JS/TS/Vue/JSON/Markdown/CSS file in the repo; ui/console adds the Nuxt rules on top
// (ui/console/eslint.config.mjs). YAML belongs to yamllint, so antfu's yaml support is off.
export function oagent() {
  return antfu(
    {
      type: 'app',
      vue: true,
      typescript: true,
      yaml: false,
      stylistic: {
        indent: 2,
        quotes: 'single',
        semi: true,
      },
      formatters: {
        css: true,
        html: true,
        markdown: 'prettier',
      },
      ignores: [
        '**/gen/**',
        '**/.nuxt/**',
        '**/.output/**',
        '**/dist/**',
        '**/storybook-static/**',
        '**/coverage/**',
        '**/testdata/**',
        'internal/web/static/**',
        'internal/web/console/public/**',
        'internal/mcp/ui/assets/**',
        'docs/superpowers/**',
        '.superpowers/**',
        '.agents/**',
        '.claude/**',
        '.playwright-mcp/**',
        'AUTH*.md',
        'OAGENT-NOTES.md',
        '*_BACKLOG.md',
      ],
    },
    {
      // The catalog holds only versions shared by several ui/* packages.
      files: ['**/package.json'],
      rules: {
        'pnpm/json-enforce-catalog': 'off',
      },
    },
    {
      // No one-line if: every block gets braces on its own lines.
      files: ['**/*.{js,mjs,cjs,ts,mts,vue}'],
      rules: {
        'curly': ['error', 'all'],
        'style/brace-style': ['error', '1tbs', { allowSingleLine: false }],
        // Interface fields end with semicolons; inline object types (parameters, type literals) use commas.
        'style/member-delimiter-style': ['error', {
          multiline: { delimiter: 'comma', requireLast: true },
          singleline: { delimiter: 'comma', requireLast: false },
          overrides: {
            interface: {
              multiline: { delimiter: 'semi', requireLast: true },
              singleline: { delimiter: 'semi', requireLast: true },
            },
          },
        }],
      },
    },
    {
      files: [
        '**/pages/**/*.vue',
        '**/layouts/**/*.vue',
        '**/components/ui/**/*.vue',
        '**/app.vue',
        '**/error.vue',
      ],
      rules: {
        'vue/multi-word-component-names': 'off',
      },
    },
  );
}

export default oagent();
