#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR"

if command -v golangci-lint >/dev/null 2>&1 || [[ -n "${GOLANGCI_LINT_BIN:-}" ]]; then
  ./scripts/lint.sh
else
  echo "[check-local-gates] golangci-lint not available; skipping lint step" >&2
fi

./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
npm run build --prefix webui