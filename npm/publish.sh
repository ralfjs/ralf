#!/usr/bin/env bash
set -euo pipefail

# Publish npm packages for a ralf release.
# Called by the release workflow after artifacts are downloaded.
# Expects:
#   - VERSION env var (e.g. "0.1.0")
#   - NODE_AUTH_TOKEN env var
#   - artifacts/ directory with platform tarballs

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ARTIFACTS_DIR="${SCRIPT_DIR}/../artifacts"

if [ -z "${VERSION:-}" ]; then
  echo "Error: VERSION env var not set"
  exit 1
fi

# Map Go os/arch to npm package names and archive names.
declare -A PLATFORMS=(
  ["darwin-arm64"]="ralf-darwin-arm64"
  ["darwin-amd64"]="ralf-darwin-x64"
  ["linux-amd64"]="ralf-linux-x64"
  ["linux-arm64"]="ralf-linux-arm64"
)

# Extract binaries and publish platform packages.
for key in "${!PLATFORMS[@]}"; do
  pkg="${PLATFORMS[$key]}"
  goos="${key%-*}"
  goarch="${key#*-}"

  # Find the tarball.
  tarball=$(find "$ARTIFACTS_DIR" -name "ralf_*_${goos}_${goarch}.tar.gz" | head -1)
  if [ -z "$tarball" ]; then
    echo "Warning: no tarball found for $goos/$goarch, skipping $pkg"
    continue
  fi

  # Extract binary into package.
  pkg_dir="${SCRIPT_DIR}/${pkg}"
  mkdir -p "${pkg_dir}/bin"
  tar xzf "$tarball" -C "${pkg_dir}/bin"
  chmod +x "${pkg_dir}/bin/ralf"

  # Update version.
  cd "$pkg_dir"
  npm version "$VERSION" --no-git-tag-version --allow-same-version
  npm publish --access public
  echo "Published $pkg@$VERSION"
  cd -
done

# Publish root package.
cd "${SCRIPT_DIR}/ralf"
npm version "$VERSION" --no-git-tag-version --allow-same-version

# Update optionalDependencies versions.
node -e "
  const pkg = require('./package.json');
  for (const dep of Object.keys(pkg.optionalDependencies || {})) {
    pkg.optionalDependencies[dep] = '${VERSION}';
  }
  require('fs').writeFileSync('package.json', JSON.stringify(pkg, null, 2) + '\n');
"

npm publish --access public
echo "Published ralf-lint@$VERSION"
