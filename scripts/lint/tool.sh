#!/usr/bin/env sh
# Runs a lint or release tool: the installed binary if there is one, else its pinned Docker image,
# else it prints a note and skips. TOOL_STRICT=1 (set in CI) turns the skip into a failure.
# usage: scripts/lint/tool.sh <actionlint|hadolint|yamllint|shellcheck|goreleaser> [args...]
set -eu

tool=${1:?usage: scripts/lint/tool.sh <tool> [args...]}
shift

case $tool in
  actionlint) image=rhysd/actionlint:1.7.12 ;;
  hadolint) image=hadolint/hadolint:v2.15.1 ;;
  yamllint) image=pipelinecomponents/yamllint:0.35.27 ;;
  shellcheck) image=koalaman/shellcheck:v0.11.0 ;;
  goreleaser) image=goreleaser/goreleaser:v2.18.3 ;;
  *)
    echo "tool.sh: unknown tool $tool" >&2
    exit 2
    ;;
esac

if [ "${TOOL_DOCKER:-0}" != 1 ] && command -v "$tool" >/dev/null 2>&1; then
  exec "$tool" "$@"
fi

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  # the repo is mounted at the same path, so absolute paths (lint-staged passes them) resolve in the container
  set -- --rm --user "$(id -u):$(id -g)" -e HOME=/tmp -v "$PWD:$PWD" -w "$PWD" --entrypoint "$tool" "$image" "$@"
  if [ "$tool" = goreleaser ]; then
    # docker run -e NAME passes NAME through only when it is set
    set -- -e GHCR_OWNER -e GITHUB_TOKEN -e GITHUB_REPOSITORY "$@"
  fi
  exec docker run "$@"
fi

if [ "${TOOL_STRICT:-0}" = 1 ]; then
  echo "tool.sh: $tool is not installed and Docker is not available" >&2
  exit 1
fi
echo "note: skipping $tool (not installed, no Docker); see .husky/README.md to enable it" >&2
