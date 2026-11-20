#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT HUP INT TERM
MOCK_BIN="$TEST_DIR/mock-bin"
mkdir -p "$MOCK_BIN"

cat > "$MOCK_BIN/uname" <<'EOF'
#!/bin/sh
case "$1" in
  -s) echo Linux ;;
  -m) echo x86_64 ;;
  *) exit 1 ;;
esac
EOF

cat > "$MOCK_BIN/curl" <<'EOF'
#!/bin/sh
while [ $# -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    : > "$2"
    exit 0
  fi
  shift
done
exit 1
EOF

cat > "$MOCK_BIN/tar" <<'EOF'
#!/bin/sh
destination=""
archive=""
while [ $# -gt 0 ]; do
  if [ "$1" = "-xzf" ]; then
    archive=$2
    shift 2
  elif [ "$1" = "-C" ]; then
    destination=$2
    shift 2
  else
    shift
  fi
done
[ -n "$destination" ] || exit 1
mkdir -p "$destination"
case "${archive##*/}" in
  cache-cleaner-*) apps="cache-cleaner dev-cache git-cleaner linux-cache-cleaner" ;;
  *) apps="bilge dev-cache git-cleaner bilge-linux" ;;
esac
for app in $apps; do
  : > "$destination/$app"
done
EOF

chmod +x "$MOCK_BIN/uname" "$MOCK_BIN/curl" "$MOCK_BIN/tar"
PATH="$MOCK_BIN:$PATH" "$ROOT_DIR/install.sh" -b "$TEST_DIR/unified-bin" -a bilge v2.0.1 >/dev/null
for app in cache-cleaner dev-cache git-cleaner linux-cache-cleaner; do
  [ -x "$TEST_DIR/unified-bin/$app" ] || {
    echo "unified installer did not install $app" >&2
    exit 1
  }
done

PATH="$MOCK_BIN:$PATH" "$ROOT_DIR/install.sh" -b "$TEST_DIR/current-bin" -a bilge v2.1.0 >/dev/null
for app in bilge dev-cache git-cleaner bilge-linux; do
  [ -x "$TEST_DIR/current-bin/$app" ] || {
    echo "current installer did not install $app" >&2
    exit 1
  }
done

PATH="$MOCK_BIN:$PATH" "$ROOT_DIR/install.sh" -b "$TEST_DIR/standalone-bin" -a dev-cache v2.1.0 >/dev/null
[ -x "$TEST_DIR/standalone-bin/dev-cache" ] || {
  echo "standalone installer did not install dev-cache" >&2
  exit 1
}
[ ! -e "$TEST_DIR/standalone-bin/bilge" ] || {
  echo "standalone selection unexpectedly installed the unified launcher" >&2
  exit 1
}
