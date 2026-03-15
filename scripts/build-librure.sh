#!/usr/bin/env bash
set -euo pipefail

VENDOR_DIR="$(cd "$(dirname "$0")/.." && pwd)/vendor"
REGEX_SRC="$VENDOR_DIR/regex-src"
LIBRURE_DIR="$VENDOR_DIR/librure"

# Pin to a specific tag for reproducible builds.
# Update this when upgrading the Rust regex engine.
REGEX_VERSION="regex-syntax-0.8.10"

if [ -d "$REGEX_SRC" ]; then
  # Verify existing checkout matches pinned version. Remove stale checkout if not.
  CURRENT=$(git -C "$REGEX_SRC" describe --tags --exact-match 2>/dev/null || echo "unknown")
  if [ "$CURRENT" != "$REGEX_VERSION" ]; then
    echo "Stale checkout ($CURRENT), re-cloning at $REGEX_VERSION..."
    rm -rf "$REGEX_SRC"
  fi
fi

if [ ! -d "$REGEX_SRC" ]; then
  echo "Cloning rust-lang/regex at $REGEX_VERSION..."
  git clone --branch "$REGEX_VERSION" --depth 1 https://github.com/rust-lang/regex.git "$REGEX_SRC"
fi

echo "Building librure..."
cargo build --release --manifest-path "$REGEX_SRC/regex-capi/Cargo.toml"

mkdir -p "$LIBRURE_DIR"
cp "$REGEX_SRC/target/release/librure.a" "$LIBRURE_DIR/"
echo "librure.a built at $LIBRURE_DIR/librure.a"
