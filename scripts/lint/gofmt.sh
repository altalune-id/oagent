#!/usr/bin/env sh
# Fails when a given Go file is not gofmt-formatted; gofmt -l alone always exits 0.
set -eu
out=$(gofmt -l "$@")
if [ -z "$out" ]; then
  exit 0
fi
echo "gofmt: these files need formatting (run make fmt):" >&2
echo "$out" >&2
exit 1
