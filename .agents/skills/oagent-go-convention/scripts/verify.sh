#!/usr/bin/env bash
# Runs oagent's gates in order and says what each failure usually means.
#
#   verify.sh                 regenerates code, then runs every unit gate
#   verify.sh --check         read-only: never writes a file (use during review)
#   verify.sh --integration   also runs make cover (integration tests + 85% per package)
#   verify.sh --ui            also runs the UI gates (typecheck, tests, build, Storybook)
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "$root" || exit 1
log="$(mktemp -d)"
fail=0
readonly_mode=0
integration=0
ui=0
for a in "$@"; do
  case "$a" in
    --check) readonly_mode=1 ;;
    --integration) integration=1 ;;
    --ui) ui=1 ;;
    *) printf 'unknown flag: %s\n' "$a" >&2; exit 2 ;;
  esac
done

step() { printf '\n=== %s\n' "$1"; }
why() { printf '    -> %s\n' "$1"; fail=1; }
run() { local name="$1"; shift; "$@" >"$log/$name.log" 2>&1; }

if [ "$readonly_mode" -eq 1 ]; then
  step "config examples match the config structs"
  run config make config-check || why "a config struct tag changed: make config-examples"
  printf '    not checked read-only: make generate (run make generate-check on a clean tree)\n'
else
  step "generated code"
  run generate make generate || { tail -15 "$log/generate.log"; why "make generate failed"; }
  run configex make config-examples || why "make config-examples failed"
  git status --short -- gen ui/console/gen '*_templ.go' scope apperror .env.reference config.reference.yml | head -10
fi

step "make check (vet, golangci-lint, ESLint, config linters, unit tests)"
if ! run check make check; then
  grep -E "^\S+\.(go|ts|vue|md|ya?ml):[0-9]+|FAIL|error" "$log/check.log" | head -20
  why "depguard: an import breaks the layer rules (AGENTS.md); TestEveryRPCHasAPermission: add the RPC to internal/permission; TestEveryRequestFieldHasARule: add protovalidate rules. Full log: $log/check.log"
fi

step "comment discipline"
if ! run comment make comment-check; then
  tail -10 "$log/comment.log"
  why "keep one-sentence docs on exported symbols and NOTE/SECURITY/TODO markers only"
fi

if [ "$integration" -eq 1 ]; then
  step "integration tests and coverage (make cover)"
  if ! run cover make cover; then
    grep -E "^(FAIL|--- FAIL|cover:)" "$log/cover.log" | head -10
    why "a failing integration test or a package under 85%. Full log: $log/cover.log"
  fi
fi

if [ "$ui" -eq 1 ]; then
  step "UI: typecheck, tests, build, Storybook"
  run typecheck pnpm -r --if-present typecheck || { tail -10 "$log/typecheck.log"; why "TypeScript errors"; }
  run uitest make ui-test || { tail -10 "$log/uitest.log"; why "a failing UI test or coverage under the threshold"; }
  run uibuild make ui || { tail -10 "$log/uibuild.log"; why "the embedded UI does not build"; }
  run storybook pnpm --filter @oagent/console build-storybook --quiet || why "Storybook does not build"
fi

printf '\n'
if [ "$fail" -eq 0 ]; then
  printf 'all gates passed.\n\nStill on you:\n'
  printf '  - open what you changed (console page, CLI command, MCP tool); a green build is not proof it works\n'
  printf '  - for every guard you added (permission, reach, validation), break it once and see a test fail\n'
  exit 0
fi
printf 'gates failed; see above. Logs: %s\n' "$log"
exit 1
