#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEST_TAG="release-test-$(date +%Y%m%d%H%M%S)-$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo nogit)"
MANIFEST_PATH="$ROOT_DIR/.release/manifests/${TEST_TAG}.json"

cleanup() {
  rm -f "$MANIFEST_PATH"
}
trap cleanup EXIT

cd "$ROOT_DIR"

output="$(sh ops/local/release.sh --tag "$TEST_TAG" --services backend,quant --build-only --dry-run)"
printf '%s\n' "$output"

printf '%s' "$output" | grep -q 'Resolved release plan'
printf '%s' "$output" | grep -q 'Building backend'
printf '%s' "$output" | grep -q 'Building quant'
[ -f "$MANIFEST_PATH" ]

grep -q '"requested_services": \["backend" ,"quant"\]' "$MANIFEST_PATH"
grep -q 'pumpkin-pro-backend' "$MANIFEST_PATH"
grep -q 'pumpkin-pro-quant' "$MANIFEST_PATH"

echo 'release script dry-run test passed'
