#!/usr/bin/env bash
set -euo pipefail

# Build librure.a (Rust regex-capi) for the host or a specified target.
#
# Usage:
#   ./scripts/build-librure.sh                              # host platform
#   ./scripts/build-librure.sh aarch64-unknown-linux-gnu    # cross-compile

VENDOR_DIR="$(cd "$(dirname "$0")/.." && pwd)/vendor"
REGEX_SRC="$VENDOR_DIR/regex-src"
LIBRURE_DIR="$VENDOR_DIR/librure"
TARGET="${1:-}"

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

if [ -n "$TARGET" ]; then
  echo "Building librure for target $TARGET..."
  if ! rustup target list --installed | grep -Fxq "$TARGET"; then
    rustup target add "$TARGET"
  fi
  cargo build --release --target "$TARGET" --manifest-path "$REGEX_SRC/regex-capi/Cargo.toml"
  SRC_LIB="$REGEX_SRC/target/$TARGET/release/librure.a"
else
  echo "Building librure for host..."
  cargo build --release --manifest-path "$REGEX_SRC/regex-capi/Cargo.toml"
  SRC_LIB="$REGEX_SRC/target/release/librure.a"
fi

mkdir -p "$LIBRURE_DIR"
cp "$SRC_LIB" "$LIBRURE_DIR/"
echo "librure.a built at $LIBRURE_DIR/librure.a"
