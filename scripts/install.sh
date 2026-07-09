#!/usr/bin/env bash
# Reames Agent installer for Linux, macOS, and other Unix-like systems.
#
# This installer defaults to source-build mode until stable public release
# artifacts are enabled. It can also install an explicit GitHub Release artifact
# with SHA256 verification when --binary-source release --version vX.Y.Z is used.

set -euo pipefail

REPO_URL="${REAMES_AGENT_REPO_URL:-https://github.com/Ebonyhtx/reames-agent.git}"
BRANCH="${REAMES_AGENT_BRANCH:-main}"
RELEASE_BASE_URL="${REAMES_AGENT_RELEASE_BASE_URL:-https://github.com/Ebonyhtx/reames-agent/releases/download}"
BINARY_SOURCE="${REAMES_AGENT_BINARY_SOURCE:-source}"
VERSION="${REAMES_AGENT_VERSION:-}"
INSTALL_DIR="${REAMES_AGENT_INSTALL_DIR:-$HOME/.reames-agent/bin}"
AGENT_HOME="${REAMES_AGENT_HOME:-$HOME/.reames-agent}"
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
  --binary-source MODE   source or release (default: source; release requires --version)
  --version VERSION      Release tag used with --binary-source release, e.g. v0.1.0
  --release-base-url URL Release download base URL
  --install-dir PATH     Directory for the reames-agent binary (default: ~/.reames-agent/bin)
  --home PATH            Reames Agent home for config, credentials, and gateway services
  --skip-setup           Do not run `reames-agent setup` after install
  --gateway              Install the gateway background service after building
  --channels LIST        Gateway channels for --gateway, e.g. feishu,weixin
  --gateway-dir PATH     Workspace directory passed to gateway service install
  --dry-run              Print planned commands without changing the host
  -h, --help             Show this help

Examples:
  curl -fsSL https://raw.githubusercontent.com/Ebonyhtx/reames-agent/main/scripts/install.sh | bash
  scripts/install.sh --binary-source release --version v0.1.0
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

detect_release_target() {
  local os arch machine
  case "$(uname -s)" in
    Linux) os="linux" ;;
    Darwin) os="darwin" ;;
    *)
      echo "error: release artifacts are only supported by this installer on Linux and macOS; use source mode or install.ps1 on Windows" >&2
      exit 1
      ;;
  esac
  machine="$(uname -m)"
  case "$machine" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *)
      echo "error: unsupported architecture for release artifact: $machine" >&2
      exit 1
      ;;
  esac
  printf '%s %s' "$os" "$arch"
}

verify_release_checksum() {
  local archive checksum asset expected
  archive="$1"
  checksum="$2"
  asset="$3"
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "+ verify SHA256SUMS contains $asset and matches downloaded archive"
    return 0
  fi
  expected="$(awk -v asset="$asset" '$2 == asset { print; exit }' "$checksum")"
  if [ -z "$expected" ]; then
    echo "error: SHA256SUMS does not contain $asset" >&2
    exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$(dirname "$archive")" && printf '%s\n' "$expected" | sha256sum -c -)
  elif command -v shasum >/dev/null 2>&1; then
    (cd "$(dirname "$archive")" && printf '%s\n' "$expected" | shasum -a 256 -c -)
  else
    echo "error: sha256sum or shasum is required to verify release artifacts" >&2
    exit 1
  fi
}

install_from_release() {
  local target os arch asset release_url archive checksum extract_dir
  if [ -z "$VERSION" ]; then
    echo "error: --binary-source release requires --version vMAJOR.MINOR.PATCH" >&2
    exit 2
  fi
  target="$(detect_release_target)"
  os="${target%% *}"
  arch="${target##* }"
  asset="reames-agent-${os}-${arch}.tar.gz"
  release_url="${RELEASE_BASE_URL}/${VERSION}"
  archive="$WORK_DIR/$asset"
  checksum="$WORK_DIR/SHA256SUMS"
  extract_dir="$WORK_DIR/release"

  if [ "$DRY_RUN" -eq 0 ] && ! command -v curl >/dev/null 2>&1; then
    echo "error: curl is required for release artifact installs" >&2
    exit 1
  fi
  if [ "$DRY_RUN" -eq 0 ] && ! command -v tar >/dev/null 2>&1; then
    echo "error: tar is required for release artifact installs" >&2
    exit 1
  fi

  run mkdir -p "$WORK_DIR" "$INSTALL_DIR" "$extract_dir"
  run curl -fsSL -o "$archive" "$release_url/$asset"
  run curl -fsSL -o "$checksum" "$release_url/SHA256SUMS"
  verify_release_checksum "$archive" "$checksum" "$asset"
  run tar -xzf "$archive" -C "$extract_dir"
  run install -m 755 "$extract_dir/$BIN_NAME" "$BIN_PATH"
}

install_from_source() {
  if [ "$DRY_RUN" -eq 0 ]; then
    if ! command -v git >/dev/null 2>&1; then
      echo "error: git is required" >&2
      exit 1
    fi
    if ! command -v go >/dev/null 2>&1; then
      echo "error: Go 1.25+ is required for source installs; use --binary-source release --version vX.Y.Z after stable release artifacts are available" >&2
      exit 1
    fi
  fi

  run git clone --depth 1 --branch "$BRANCH" "$REPO_URL" "$WORK_DIR"
  run mkdir -p "$INSTALL_DIR"
  run env CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN_PATH" "$WORK_DIR/cmd/reames-agent"
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
    --binary-source)
      BINARY_SOURCE="${2:?--binary-source needs a value}"
      shift 2
      ;;
    --version)
      VERSION="${2:?--version needs a value}"
      shift 2
      ;;
    --release-base-url)
      RELEASE_BASE_URL="${2:?--release-base-url needs a value}"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="${2:?--install-dir needs a value}"
      shift 2
      ;;
    --home)
      AGENT_HOME="${2:?--home needs a value}"
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

case "$BINARY_SOURCE" in
  source|release) ;;
  *)
    echo "error: --binary-source must be source or release" >&2
    exit 2
    ;;
esac

WORK_DIR="${TMPDIR:-/tmp}/reames-agent-install-$$"
BIN_PATH="$INSTALL_DIR/$BIN_NAME"

echo "Installing Reames Agent"
echo "  binary mode: $BINARY_SOURCE"
echo "  repo:        $REPO_URL"
echo "  ref:         $BRANCH"
if [ "$BINARY_SOURCE" = "release" ]; then
  echo "  release:     $RELEASE_BASE_URL/$VERSION"
fi
echo "  binary:      $BIN_PATH"
echo "  agent home:  $AGENT_HOME"

run rm -rf "$WORK_DIR"

if [ "$BINARY_SOURCE" = "release" ]; then
  install_from_release
else
  install_from_source
fi

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
  run env REAMES_AGENT_HOME="$AGENT_HOME" "$BIN_PATH" setup
fi

if [ "$INSTALL_GATEWAY" -eq 1 ]; then
  echo "Gateway credential source: ${AGENT_HOME%/}/.env"
  echo "Gateway service definitions pin REAMES_AGENT_HOME and do not embed secret values."
  gateway_args=("$BIN_PATH" gateway install --start-now --home "$AGENT_HOME")
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
