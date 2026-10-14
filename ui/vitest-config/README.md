# @oagent/vitest-config

Shared Vitest presets. Every `ui/*` package with tests extends one of these, so repo-wide test settings live in one place.

## Presets

| Entry                           | Environment                                                |
| ------------------------------- | ---------------------------------------------------------- |
| `@oagent/vitest-config/base`    | none: base config only (10 s timeouts, CI reporter toggle) |
| `@oagent/vitest-config/node`    | Node: pure TS helpers (most tests)                         |
| `@oagent/vitest-config/browser` | happy-dom: tests that need the DOM                         |

## Usage

```ts
// ui/foo/vitest.config.ts
export { default } from '@oagent/vitest-config/node';
```

Or extend it:

```ts
import base from '@oagent/vitest-config/node';
import { defineConfig, mergeConfig } from 'vitest/config';

export default mergeConfig(base, defineConfig({
  test: { setupFiles: ['./test/setup.ts'] },
}));
```

Run every package's tests with `make ui-test` (`pnpm -r --if-present test`).

## Peer deps

`vitest` is a peer; add it to the consuming package. The `browser` preset also needs `happy-dom`: add it only to a package that uses that preset.
