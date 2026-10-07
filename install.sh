#!/bin/sh
# Install the public release. Does not edit shell configuration or Claude settings.
set -eu
umask 077

fail() { printf 'kclaude: %s\n' "$*" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || fail 'curl is required'
command -v python3 >/dev/null 2>&1 || fail 'Python 3.9+ is required'
python3 -c 'import sys; sys.exit(sys.version_info < (3, 9))' || fail 'Python 3.9+ is required'

case "$(uname -s)" in Darwin) os=darwin ;; Linux) os=linux ;; *) fail 'Use macOS, Linux, or WSL2' ;; esac
case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) fail 'Supported CPUs: arm64, amd64' ;; esac

version=${KCLAUDE_VERSION:-latest}
case "$version" in latest|v[0-9]*) ;; *) fail 'KCLAUDE_VERSION must be a v-prefixed release tag' ;; esac
case "$version" in *[!a-zA-Z0-9._-]*) fail 'Invalid release tag' ;; esac
asset="kclaude-${os}-${arch}.tar.gz"
if [ "$version" = latest ]; then
    base='https://github.com/evgenspm/kclaude/releases/latest/download'
else
    base="https://github.com/evgenspm/kclaude/releases/download/$version"
fi
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$base/$asset" -o "$tmp/$asset"
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"

python3 - "$tmp" "$asset" "$version" <<'PY'
from pathlib import Path
import hashlib
import os
import re
import shutil
import sys
import tarfile
import tempfile

temporary, asset, requested = Path(sys.argv[1]), sys.argv[2], sys.argv[3]
checksums = dict(line.split()[::-1] for line in (temporary / 'SHA256SUMS').read_text().splitlines() if line.strip())
digest = hashlib.sha256((temporary / asset).read_bytes()).hexdigest()
if checksums.get(asset) != digest:
    sys.exit('kclaude: release checksum verification failed')
allowed = {'kclaude', 'kclaude-router', 'VERSION', 'LICENSE', 'NOTICE'}
unpacked = temporary / 'unpacked'
unpacked.mkdir()
with tarfile.open(temporary / asset, 'r:gz') as archive:
    members = archive.getmembers()
    if {m.name for m in members} != allowed or len(members) != len(allowed):
        sys.exit('kclaude: unexpected release contents')
    for member in members:
        if not member.isfile():
            sys.exit('kclaude: release contains a non-file entry')
        with archive.extractfile(member) as source, (unpacked / member.name).open('wb') as target:
            shutil.copyfileobj(source, target)
release = (unpacked / 'VERSION').read_text().strip()
if not re.fullmatch(r'[0-9]+(?:\.[0-9]+){3}', release):
    sys.exit('kclaude: invalid release version')
if requested != 'latest' and requested != 'v' + release:
    sys.exit('kclaude: release version does not match requested tag')
install = Path(os.environ.get('KCLAUDE_INSTALL_DIR', str(Path.home() / '.local/share/kclaude/app'))).expanduser().absolute()
bindir = Path(os.environ.get('KCLAUDE_BIN_DIR', str(Path.home() / '.local/bin'))).expanduser().absolute()
command = bindir / 'kclaude'
expected_link = install / 'current' / 'kclaude'
if os.path.lexists(command):
    if not command.is_symlink() or Path(os.readlink(command)) != expected_link:
        sys.exit(f'kclaude: refusing to replace an unrelated command: {command}')
install.mkdir(mode=0o700, parents=True, exist_ok=True)
bindir.mkdir(parents=True, exist_ok=True)
if os.path.lexists(install / 'current') and not (install / 'current').is_symlink():
    sys.exit('kclaude: app/current exists and is not an installer-managed symlink')
destination = install / ('v' + release)
if destination.exists():
    # Reinstalling a published version is a no-op only if every byte matches.
    for name in allowed:
        if not (destination / name).is_file() or (destination / name).read_bytes() != (unpacked / name).read_bytes():
            sys.exit('kclaude: existing release differs; use a new install directory')
else:
    staging = Path(tempfile.mkdtemp(prefix='.staging-', dir=install))
    try:
        shutil.copytree(unpacked, staging, dirs_exist_ok=True)
        for name in ('kclaude', 'kclaude-router'):
            (staging / name).chmod(0o755)
        os.rename(staging, destination)
    finally:
        if staging.exists():
            shutil.rmtree(staging)
for name in ('kclaude', 'kclaude-router'):
    (destination / name).chmod(0o755)
link = install / ('.current-' + str(os.getpid()))
link.symlink_to(destination.name)
os.replace(link, install / 'current')
if not os.path.lexists(command):
    command.symlink_to(expected_link)
print(f'Installed kclaude {release}: {command}')
if str(bindir) not in os.environ.get('PATH', '').split(os.pathsep):
    print(f'Add {bindir} to PATH, or run {command} by its full path.')
print('Next: kclaude accounts add personal')
print('Then: cd your-project && kclaude')
print('Default: --dangerously-skip-permissions. Use kclaude --safe for permission prompts.')
print('After upgrading, run kclaude stop between sessions to load the new router.')
PY
