#!/usr/bin/env bash
# Reames Agent installer for Linux, macOS, and other Unix-like systems.
#
# This installer is intentionally source-build based until public release
# artifacts are enabled. It installs one Go binary and can optionally install
# the social gateway as a user-level background service.

set -euo pipefail

REPO_URL="${REAMES_AGENT_REPO_URL:-https://github.com/Ebonyhtx/reames-agent.git}"
BRANCH="${REAMES_AGENT_BRANCH:-main}"
INSTALL_DIR="${REAMES_AGENT_INSTALL_DIR:-$HOME/.reames-agent/bin}"
BIN_NAME="reames-agent"
RUN_SETUP=1
INSTALL_GATEWAY=0
GATEWAY_CHANNELS=""
GATEWAY_DIR=""
DRY_RUN=0

usage() {
  cat <<'EOF'
Reames Agent installer

Usage:
  scripts/install.sh [options]

Options:
  --repo URL             Git repository to clone (default: official Reames repo)
  --branch NAME          Git branch/tag/ref to checkout (default: main)
  --install-dir PATH     Directory for the reames-agent binary (default: ~/.reames-agent/bin)
  --skip-setup           Do not run `reames-agent setup` after install
  --gateway              Install the gateway background service after building
  --channels LIST        Gateway channels for --gateway, e.g. feishu,weixin
  --gateway-dir PATH     Workspace directory passed to gateway service install
  --dry-run              Print planned commands without changing the host
  -h, --help             Show this help

Examples:
  curl -fsSL https://raw.githubusercontent.com/Ebonyhtx/reames-agent/main/scripts/install.sh | bash
  scripts/install.sh --gateway --channels feishu --gateway-dir /srv/reames-work
  scripts/install.sh --dry-run --gateway --channels feishu
EOF
}

quote() {
  printf '%q' "$1"
}

run() {
  if [ "$DRY_RUN" -eq 1 ]; then
    printf '+'
    for arg in "$@"; do
      printf ' %s' "$(quote "$arg")"
    done
    printf '\n'
    return 0
  fi
  "$@"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo)
      REPO_URL="${2:?--repo needs a value}"
      shift 2
      ;;
    --branch)
      BRANCH="${2:?--branch needs a value}"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="${2:?--install-dir needs a value}"
      shift 2
      ;;
    --skip-setup)
      RUN_SETUP=0
      shift
      ;;
    --gateway)
      INSTALL_GATEWAY=1
      shift
      ;;
    --channels)
      GATEWAY_CHANNELS="${2:?--channels needs a value}"
      shift 2
      ;;
    --gateway-dir)
      GATEWAY_DIR="${2:?--gateway-dir needs a value}"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [ "$DRY_RUN" -eq 0 ]; then
  if ! command -v git >/dev/null 2>&1; then
    echo "error: git is required" >&2
    exit 1
  fi
  if ! command -v go >/dev/null 2>&1; then
    echo "error: Go 1.25+ is required until release binaries are available" >&2
    exit 1
  fi
fi

WORK_DIR="${TMPDIR:-/tmp}/reames-agent-install-$$"
BIN_PATH="$INSTALL_DIR/$BIN_NAME"

echo "Installing Reames Agent"
echo "  repo:        $REPO_URL"
echo "  ref:         $BRANCH"
echo "  binary:      $BIN_PATH"

run rm -rf "$WORK_DIR"
run git clone --depth 1 --branch "$BRANCH" "$REPO_URL" "$WORK_DIR"
run mkdir -p "$INSTALL_DIR"
run env CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN_PATH" "$WORK_DIR/cmd/reames-agent"

if [ "$DRY_RUN" -eq 0 ]; then
  chmod +x "$BIN_PATH"
fi

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo "note: add $INSTALL_DIR to PATH, for example:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

if [ "$RUN_SETUP" -eq 1 ]; then
  run "$BIN_PATH" setup
fi

if [ "$INSTALL_GATEWAY" -eq 1 ]; then
  gateway_args=("$BIN_PATH" gateway install --start-now)
  if [ -n "$GATEWAY_CHANNELS" ]; then
    gateway_args+=(--channels "$GATEWAY_CHANNELS")
  fi
  if [ -n "$GATEWAY_DIR" ]; then
    gateway_args+=(--dir "$GATEWAY_DIR")
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    gateway_args+=(--dry-run)
  fi
  run "${gateway_args[@]}"
fi

run rm -rf "$WORK_DIR"
echo "Reames Agent install complete."
