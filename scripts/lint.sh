#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

LINT_BIN="${GOLANGCI_LINT_BIN:-golangci-lint}"
BOOTSTRAP_VERSION="${GOLANGCI_LINT_VERSION:-v2.11.4}"
BOOTSTRAP_BIN="${ROOT_DIR}/.tmp/golangci-lint-${BOOTSTRAP_VERSION}"

export GOCACHE="${GOCACHE:-${ROOT_DIR}/.tmp/go-build-cache}"
export GOLANGCI_LINT_CACHE="${GOLANGCI_LINT_CACHE:-${ROOT_DIR}/.tmp/golangci-lint-cache}"
mkdir -p "$GOCACHE" "$GOLANGCI_LINT_CACHE"

bootstrap_golangci_lint() {
  local version_no_v os arch
  version_no_v="${BOOTSTRAP_VERSION#v}"
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m | tr '[:upper:]' '[:lower:]')"

  case "$os" in
    linux|darwin|windows) ;;
    mingw*|msys*|cygwin* ) os="windows" ;;
    *)
      echo "unsupported OS for bootstrap: ${os}" >&2
      return 1
      ;;
  esac

  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *)
      echo "unsupported architecture for bootstrap: ${arch}" >&2
      return 1
      ;;
  esac

  if [[ -z "${HTTP_PROXY:-}${http_proxy:-}${HTTPS_PROXY:-}${https_proxy:-}${ALL_PROXY:-}${all_proxy:-}" ]]; then
    export HTTP_PROXY="http://127.0.0.1:7890"
    export http_proxy="$HTTP_PROXY"
    export HTTPS_PROXY="$HTTP_PROXY"
    export https_proxy="$HTTP_PROXY"
    export ALL_PROXY="$HTTP_PROXY"
    export all_proxy="$HTTP_PROXY"
  fi

  export GOPROXY="direct"
  export GOSUMDB="off"

  mkdir -p "${ROOT_DIR}/.tmp"
  GOBIN="${ROOT_DIR}/.tmp" GOOS="$os" GOARCH="$arch" go install "github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
  cp "${ROOT_DIR}/.tmp/golangci-lint" "${BOOTSTRAP_BIN}"
  chmod +x "${BOOTSTRAP_BIN}"

  echo "bootstrapped golangci-lint ${BOOTSTRAP_VERSION} to ${BOOTSTRAP_BIN}" >&2
}

run_lint() {
  local bin="$1"
  if [[ "$bin" == *" "* ]]; then
    eval "$bin fmt --diff -c .golangci.yml" && eval "$bin run -c .golangci.yml ./..."
  else
    "$bin" fmt --diff -c .golangci.yml && "$bin" run -c .golangci.yml ./...
  fi
}

is_compatibility_error() {
  case "$1" in
    *"command not found"*|\
    *"not recognized as an internal or external command"*|\
    *"No such file or directory"*|\
    *"unknown command \"fmt\""*|\
    *"unknown command \"run\""*|\
    *"unknown flag"*|\
    *"no such flag"*|\
    *"unsupported version of the configuration"*|\
    *"can't load config"*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

# v2 separates formatters from linters; enforce both in one entrypoint.
if lint_output="$(run_lint "$LINT_BIN" 2>&1)"; then
  [[ -n "$lint_output" ]] && echo "$lint_output"
  exit 0
fi

if [[ -n "${GOLANGCI_LINT_BIN:-}" ]]; then
  echo "$lint_output" >&2
  echo "lint failed with explicit GOLANGCI_LINT_BIN=${GOLANGCI_LINT_BIN}; skip auto-bootstrap." >&2
  exit 1
fi

if ! is_compatibility_error "$lint_output"; then
  echo "$lint_output" >&2
  exit 1
fi

echo "default golangci-lint appears incompatible; bootstrapping ${BOOTSTRAP_VERSION}..." >&2
if [[ ! -x "${BOOTSTRAP_BIN}" ]]; then
  bootstrap_golangci_lint
fi

if lint_output="$(run_lint "${BOOTSTRAP_BIN}" 2>&1)"; then
  [[ -n "$lint_output" ]] && echo "$lint_output"
  exit 0
fi

echo "$lint_output" >&2
exit 1
