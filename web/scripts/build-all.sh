#!/bin/bash
# Build shared packages, all plugins, and shell
set -e

echo "=== Building shared packages ==="
(cd web/packages/ui-kit && pnpm build)

echo "=== Building all plugins ==="
for plugin in web/plugins/*/; do
  name=$(basename "$plugin")
  echo "  Building plugin: $name"
  (cd "$plugin" && pnpm build)
done

echo "=== Building shell ==="
(cd web/shell && pnpm build)

echo "=== Copying plugins to shell dist ==="
mkdir -p web/shell/dist/modules
for plugin in web/plugins/*/; do
  name=$(basename "$plugin")
  cp -r "$plugin/dist" "web/shell/dist/modules/$name"
done

echo "=== Done ==="
echo "Output: web/shell/dist/"