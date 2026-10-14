---
name: oagent-go-review
description: Use when reviewing a change in oagent (altalune.id/oagent) — a checkpoint during a plan, a re-review after fixes, a final pass before the user commits, or a pull request — or when asked whether code follows oagent's conventions (public ids, permission table, reach, protovalidate rules, unit of work, layering, comment discipline) or is maintainable and free of duplication.
license: Proprietary
metadata:
  pairs-with: oagent-go-convention (how to build it); this skill checks it
---

# Reviewing oagent changes

A review answers two questions about every change: **does it follow oagent's conventions**, and **when it changes
next time, is that one edit in one place?** Findings are reported and ranked; nothing is fixed until the user picks.

## 1. Pick the scope

```sh
.agents/skills/oagent-go-review/scripts/scope.sh [mode] [--patch]
```

| When | Mode |
| --- | --- |
| checkpoint during a plan, or re-review after fixes | `delta` (since the last `mark`) |
| final pass before the user commits | `branch` (merge-base with main → working tree) |
| only what is uncommitted | `wip` |
| one past commit | `commit <sha>` |
| everything after a tag or PR base | `since <ref>` |

No mode means `delta` when a mark exists for the current HEAD, else `branch`. The user may name files or a PR instead.

## 2. Load the standards

Read the repo's rules first; they are the source of truth:

- `AGENTS.md`
- the `docs/howto/` recipe for each kind of change in the diff, especially its **Review** checklist
- the plan or spec that drove the change, if any (`docs/superpowers/`); a change that does not do what the plan says is a finding

Then the skills, by what the diff touches:

| Always | Read |
| --- | --- |
| `go` | core principles, interfaces, errors, testing |
| `go-proverb` | the "In review, flag" lines |
| `comment-discipline` | Keep, Delete, Traps |

| Only if the diff touches | Read |
| --- | --- |
| a goroutine, channel, `select`, `sync`, `errgroup`, a worker | `go-concurrency` |
| `cmd/` or `internal/cli/` | `cobra-viper` |
| an exported symbol in a root package | `go-release` (breaking changes) |
| `ui/console` | `vue`, `pinia`, `nuxt`, `vitest` |
| ESLint, Vite or the UI build | `eslint-config`, `vite` |

## 3. Review in four passes

Read each changed file in full, not only its hunks: duplication hides outside the hunk.

1. **Bugs a user hits.** Wrong result, crash, data visible to the wrong org or project, blocked flow, broken boot. The only P0 candidates.
2. **Convention.** Work through [references/convention.md](references/convention.md).
3. **Design.** One source of truth; reuse an existing helper instead of a second copy; the cost of the next change; no abstraction for a single caller.
4. **Gates.** Run `.agents/skills/oagent-go-convention/scripts/verify.sh --check`. It is read-only and never writes a file.
   - Add `--integration` when a store, migration or boot wiring changed.
   - Add `--ui` when `ui/` changed.
   - A red gate is a finding.

For a large diff with a subagent tool, give passes 2 and 3 to parallel reviewers with the scope command; merge and dedupe.

## 4. Verify before reporting

Every finding needs evidence: a `file:line`, the second copy it duplicates, the failing command, or the input that breaks it. A finding you cannot show is a question. Drop anything a gate already enforces unless the gate is red.

## 5. Report

| Level | Meaning |
| --- | --- |
| P0 | critical, or a user or deployer hits it right away and is blocked |
| P1 | correctness or security, not yet reachable by a user |
| P2 | maintainability, reuse, a missing guard or test |
| P3 | cosmetic, naming, docs |

```markdown
## Review: <scope mode>: <N> findings (P0 a · P1 b · P2 c · P3 d)

| # | Sev | Category | Where | Finding | Fix (one line) |
| --- | --- | --- | --- | --- | --- |
| 1 | P0 | correctness | internal/x/y.go:42 | <what is wrong + evidence> | <smallest fix> |

**Questions**: ambiguities for the author.
**Gates**: each gate run, pass or fail.
```

Categories: `correctness`, `security`, `public-id`, `permission`, `convention`, `duplication`, `reuse`, `test`, `comment`, `docs`.

Then run `scope.sh mark`, so the next `delta` covers only what changes after this review.

## 6. Ask, then act

Group the findings: **blockers and user- or deployer-facing problems** (fix now) versus **nice-to-haves** (skip unless the user asks). Ask the user which to fix; do not edit code before the answer. P0 is never parked. After fixes, review again with `delta`.

## Common mistakes

- Reviewing hunks only, and missing the existing helper three files away.
- Trusting a green suite: check that a new guard has a test that fails without it.
- Proposing an abstraction for one caller.
- Fixing before asking, or committing. The user commits.
