#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
version=$(cat VERSION)
mkdir -p dist
for target in ${KCLAUDE_TARGETS:-darwin/arm64 darwin/amd64 linux/arm64 linux/amd64}; do
    os=${target%/*}
    arch=${target#*/}
    staging=$(mktemp -d)
    trap 'rm -rf "$staging"' EXIT HUP INT TERM
    GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 GOEXPERIMENT=jsonv2 go build \
        -trimpath -ldflags='-s -w' -o "$staging/kclaude-router" ./cmd/kclaude-router
    cp cli/kclaude.py "$staging/kclaude"
    cp VERSION LICENSE NOTICE "$staging/"
    chmod 755 "$staging/kclaude" "$staging/kclaude-router"
    COPYFILE_DISABLE=1 tar -czf "dist/kclaude-$os-$arch.tar.gz" -C "$staging" kclaude kclaude-router VERSION LICENSE NOTICE
    rm -rf "$staging"
    trap - EXIT HUP INT TERM
    printf 'Built %s for %s/%s\n' "$version" "$os" "$arch"
done
python3 - <<'PY'
from pathlib import Path
import hashlib
root = Path('dist')
lines = [hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + p.name for p in sorted(root.glob('kclaude-*.tar.gz'))]
(root / 'SHA256SUMS').write_text('\n'.join(lines) + '\n')
PY
