#!/usr/bin/env sh

set -eu

REPO_OWNER="everydaydevopsio"
REPO_NAME="bilge-pump"
CASK_NAME="cache-cleaner"

# Available apps
APPS="cache-cleaner dev-cache git-cleaner"

usage() {
  cat <<EOF
Install cache-cleaner tools

RECOMMENDED: Use Homebrew instead of this script:
  brew tap ${REPO_OWNER}/${REPO_NAME}
  brew install --cask ${CASK_NAME}

Usage:
  install.sh -b <bin_dir> [-a <app>] [<version>]

Options:
  -b <bin_dir>   Install destination directory (REQUIRED)
  -a <app>       Install one app: cache-cleaner, dev-cache, git-cleaner, or the platform cleaner
                 (default: install the unified command and its platform tools)

Arguments:
  <version>      Version tag to install (e.g., v1.2.3). Default: latest release

Examples:
  # Install the unified command and its platform tools to ~/.local/bin
  curl -sSfL https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/HEAD/install.sh | sh -s -- -b \$HOME/.local/bin

  # Install only the platform cleaner
  curl -sSfL https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/HEAD/install.sh | sh -s -- -b \$HOME/.local/bin -a mac-cache-cleaner

  # Install specific version
  curl -sSfL https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/HEAD/install.sh | sh -s -- -b \$HOME/.local/bin v1.2.3

Note: Make sure your bin directory is in PATH:
  export PATH="\$HOME/.local/bin:\$PATH"
EOF
}

error() { printf "Error: %s\n" "$1" >&2; exit 1; }

BIN_DIR=""
VERSION=""
APP_FILTER=""

while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    -b) shift; [ $# -gt 0 ] || error "-b requires a directory"; BIN_DIR="$1" ;;
    -a) shift; [ $# -gt 0 ] || error "-a requires an app name"; APP_FILTER="$1" ;;
    -*) error "unknown option: $1" ;;
    *) VERSION="$1" ;;
  esac
  shift
done

# Determine OS/ARCH
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  darwin) PLATFORM_APP=mac-cache-cleaner ;;
  linux) PLATFORM_APP=linux-cache-cleaner ;;
  *) error "unsupported operating system: $OS (use the Windows release archive on Windows)" ;;
esac
APPS="$APPS $PLATFORM_APP"

# Validate app filter if provided
if [ -n "$APP_FILTER" ]; then
  case "$APP_FILTER" in
    cache-cleaner|dev-cache|git-cleaner) : ;;
    mac-cache-cleaner) [ "$OS" = "darwin" ] || error "mac-cache-cleaner is only available on macOS" ;;
    linux-cache-cleaner) [ "$OS" = "linux" ] || error "linux-cache-cleaner is only available on Linux" ;;
    *) error "invalid app: $APP_FILTER" ;;
  esac
  APPS="$APP_FILTER"
fi

UNAME_M="$(uname -m)"
case "$UNAME_M" in
  x86_64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) error "unsupported architecture: $UNAME_M" ;;
esac

# Determine BIN_DIR
if [ -z "$BIN_DIR" ]; then
  # Check if Homebrew is available - recommend using it instead
  if command -v brew >/dev/null 2>&1; then
    cat >&2 <<EOF
Error: Homebrew installation is recommended over direct installation.

Please install using Homebrew instead:

    brew tap ${REPO_OWNER}/${REPO_NAME}
    brew install --cask ${CASK_NAME}

If you prefer direct installation, specify an installation directory:

    curl -sSfL https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/HEAD/install.sh | sh -s -- -b \$HOME/.local/bin

For more information, visit:
    https://github.com/${REPO_OWNER}/${REPO_NAME}
EOF
    exit 1
  fi

  # No Homebrew - require explicit -b flag
  cat >&2 <<EOF
Error: Installation directory must be specified with -b flag.

Example:
    curl -sSfL https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}/HEAD/install.sh | sh -s -- -b \$HOME/.local/bin

Make sure the directory is in your PATH:
    export PATH="\$HOME/.local/bin:\$PATH"

Alternatively, if you have Homebrew, use:
    brew tap ${REPO_OWNER}/${REPO_NAME}
    brew install --cask ${CASK_NAME}
EOF
  exit 1
fi

# Determine VERSION
if [ -z "$VERSION" ] || [ "$VERSION" = "latest" ]; then
  # Fetch latest tag via GitHub API
  if command -v curl >/dev/null 2>&1; then
    VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest" | awk -F'"' '/tag_name/ {print $4}')" || true
  fi
  [ -n "$VERSION" ] || error "failed to determine latest version; pass a version like v1.2.3"
fi

# Validate tag format vx.x.x
case "$VERSION" in
  v[0-9]*\.[0-9]*\.[0-9]*) : ;;  # strict semver check: vMAJOR.MINOR.PATCH
  *) error "version must be a semver tag like v1.2.3" ;;
esac

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT HUP TERM
VERSION_NUMBER="${VERSION#v}"
ARCHIVE_NAME="cache-cleaner-${VERSION_NUMBER}-${OS}-${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${VERSION}/${ARCHIVE_NAME}"
ARCHIVE_PATH="$TMP_DIR/$ARCHIVE_NAME"

printf "Downloading %s for %s/%s\n" "$VERSION" "$OS" "$ARCH"
curl -fL "$DOWNLOAD_URL" -o "$ARCHIVE_PATH" || error "download failed: ${DOWNLOAD_URL}"
tar -xzf "$ARCHIVE_PATH" -C "$TMP_DIR" || error "failed to extract release archive"
mkdir -p "$BIN_DIR"

for APP_NAME in $APPS; do
  SOURCE="$TMP_DIR/$APP_NAME"
  DEST="$BIN_DIR/$APP_NAME"
  [ -f "$SOURCE" ] || error "release archive does not contain $APP_NAME"
  install -m 0755 "$SOURCE" "$DEST" || error "failed to install to $DEST (try with sudo or set -b)"
  printf "Installed: %s\n" "$DEST"
done
