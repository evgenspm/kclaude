#!/usr/bin/env python3
"""Run an installed Claude Code with a separate profile and local Kiro router."""
import argparse
import fcntl
import getpass
import hashlib
import hmac
import http.client
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import socket
import ssl
import subprocess
import sys
import tempfile
import time
import uuid

APP = Path(__file__).resolve().parent
ROOT = Path(os.environ.get('KCLAUDE_HOME', str(Path.home() / '.local/share/kclaude'))).expanduser().resolve()
PROFILE = ROOT / 'claude'
PORT = int(os.environ.get('KCLAUDE_PORT', '17391'))
URL = f'https://127.0.0.1:{PORT}'
ROUTER = Path(os.environ.get('KCLAUDE_ROUTER', str(APP / 'kclaude-router')))
REGIONS = ('us-east-1', 'eu-central-1')


def write_private(path, data):
    """Replace a private file atomically, including its permissions."""
    fd, temporary = tempfile.mkstemp(prefix='.' + path.name, dir=path.parent)
    try:
        with os.fdopen(fd, 'w') as stream:
            stream.write(data)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def initialize():
    if not 1024 <= PORT <= 65535:
        raise ValueError('KCLAUDE_PORT must be between 1024 and 65535')
    ROOT.mkdir(mode=0o700, parents=True, exist_ok=True)
    os.chmod(ROOT, 0o700)
    with (ROOT / 'start.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        for directory in (PROFILE, ROOT / 'keys'):
            directory.mkdir(mode=0o700, exist_ok=True)
            os.chmod(directory, 0o700)
        defaults = {
            ROOT / 'proxy.key': secrets.token_hex(32),
            ROOT / 'accounts.json': json.dumps({'accounts': []}, indent=2) + '\n',
            PROFILE / 'settings.json': json.dumps({
                'skipDangerousModePermissionPrompt': True,
                'statusLine': {'type': 'command', 'command': "printf 'KIRO / kclaude / separate profile'"},
            }, indent=2) + '\n',
        }
        for path, data in defaults.items():
            if not path.exists():
                write_private(path, data)


def config():
    return json.loads((ROOT / 'accounts.json').read_text())


def request(path, method='GET'):
    token = (ROOT / 'proxy.key').read_text().strip()
    # Prove server identity before sending a bearer token to a occupied local port.
    # Keep the authenticated request on the same TCP connection as the challenge.
    nonce = secrets.token_hex(32)
    context = ssl.create_default_context(cafile=str(ROOT / 'router-cert.pem'))
    connection = http.client.HTTPSConnection('127.0.0.1', PORT, timeout=2, context=context)
    try:
        connection.request('GET', '/health?challenge=' + nonce)
        response = connection.getresponse()
        proof = json.loads(response.read(1024 * 1024))
        expected = hmac.new(token.encode(), ('kclaude-router:' + nonce).encode(), hashlib.sha256).hexdigest()
        if response.status != 200 or not isinstance(proof, dict) or not hmac.compare_digest(str(proof.get('proof', '')), expected) or connection.sock is None:
            raise ValueError('Local router identity check failed')
        connection.request(method, path, headers={'Authorization': 'Bearer ' + token})
        response = connection.getresponse()
        data = response.read(1024 * 1024)
        if response.status != 200:
            raise ValueError(f'Router returned HTTP {response.status}')
        return json.loads(data)
    finally:
        connection.close()


def running():
    try:
        status = request('/kclaude/status')
        if status.get('service') == 'kclaude-router' and status.get('state') == str(ROOT):
            return status
    except (OSError, ValueError, http.client.HTTPException):
        pass
    return None


def stop_locked():
    status = running()
    if not status:
        return False
    request('/kclaude/stop', method='POST')
    for _ in range(250):
        with socket.socket() as sock:
            if sock.connect_ex(('127.0.0.1', PORT)) != 0:
                return True
        time.sleep(.1)
    raise RuntimeError('Router is still finishing requests; try again shortly')


def start():
    with (ROOT / 'start.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        if running():
            return
        if not any(not a.get('disabled') for a in config()['accounts']):
            raise RuntimeError('Add a Kiro account first: kclaude accounts add personal')
        with socket.socket() as sock:
            if sock.connect_ex(('127.0.0.1', PORT)) == 0:
                raise RuntimeError(f'Port {PORT} is in use. Choose another KCLAUDE_PORT for this profile.')
        if not ROUTER.is_file():
            raise RuntimeError('Router binary missing. Reinstall kclaude or set KCLAUDE_ROUTER.')
        with (ROOT / 'router.log').open('ab') as log:
            os.chmod(ROOT / 'router.log', 0o600)
            env = {k: v for k, v in os.environ.items() if not k.startswith(('KIRO', 'OTEL_'))}
            proc = subprocess.Popen([str(ROUTER), '--state', str(ROOT), '--port', str(PORT)],
                                    stdin=subprocess.DEVNULL, stdout=log, stderr=log,
                                    start_new_session=True, env=env)
        for _ in range(100):
            if proc.poll() is not None:
                raise RuntimeError(f'Router failed to start; see {ROOT / "router.log"}')
            status = running()
            if status and status['pid'] == proc.pid:
                return
            time.sleep(.1)
        proc.terminate()
        raise RuntimeError('Router startup timed out')


def accounts(args):
    parser = argparse.ArgumentParser(prog='kclaude accounts', description='Use your own Kiro API keys from app.kiro.dev.')
    commands = parser.add_subparsers(dest='command', required=True)
    add = commands.add_parser('add', help='add or replace an account; key is entered without echo')
    add.add_argument('name')
    add.add_argument('--key-file', type=Path, help='copy a key from this file instead of prompting')
    add.add_argument('--region', choices=REGIONS, default='us-east-1')
    commands.add_parser('list')
    for command in ('remove', 'enable', 'disable'):
        commands.add_parser(command).add_argument('name')
    opts = parser.parse_args(args)
    if opts.command == 'list':
        rows = config()['accounts']
        for account in rows:
            print(f"{account['name']}\t{account['region']}\t{'disabled' if account.get('disabled') else 'enabled'}")
        if not rows:
            print('No accounts. Run: kclaude accounts add personal')
        return
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]{0,63}', opts.name):
        raise ValueError('Account name: 1-64 letters, digits, underscores or hyphens')
    key = None
    if opts.command == 'add':
        if opts.key_file:
            key = opts.key_file.expanduser().read_text().strip()
        else:
            if not sys.stdin.isatty():
                raise ValueError('Use a terminal for the hidden key prompt, or --key-file PATH')
            key = getpass.getpass('Kiro API key (ksk_..., hidden): ').strip()
        if not key.startswith('ksk_') or len(key) <= 4 or any(c.isspace() for c in key):
            raise ValueError('Expected a Kiro API key starting with ksk_')
    with (ROOT / 'start.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        cfg = config()
        existing = next((a for a in cfg['accounts'] if a['name'] == opts.name), None)
        if opts.command != 'add' and existing is None:
            raise ValueError('Unknown account: ' + opts.name)
        # Stop before changing credentials, so removed accounts cannot keep serving.
        stopped = stop_locked()
        key_path = ROOT / 'keys' / (opts.name + '.key')
        if opts.command == 'add':
            write_private(key_path, key + '\n')
            row = {'name': opts.name, 'key_file': str(key_path), 'region': opts.region}
            if existing is None:
                cfg['accounts'].append(row)
            else:
                existing.clear()
                existing.update(row)
        elif opts.command == 'remove':
            cfg['accounts'].remove(existing)
        else:
            existing['disabled'] = opts.command == 'disable'
        write_private(ROOT / 'accounts.json', json.dumps(cfg, indent=2) + '\n')
        if opts.command == 'remove':
            key_path.unlink(missing_ok=True)
    print(f'{opts.name}: {opts.command} complete.' + (' Router will restart on the next request.' if stopped else ''))


def source_config():
    original = Path(os.environ.get('KCLAUDE_SOURCE_CONFIG', str(Path.home() / '.claude'))).expanduser()
    if original.resolve() == PROFILE.resolve():
        raise ValueError('Source profile must differ from the kclaude profile')
    return original


# Linked from the source profile so kclaude behaves like your normal Claude Code.
SHARED = ('CLAUDE.md', 'skills', 'plugins', 'agents', 'commands', 'output-styles', 'mcp.json', 'keybindings.json')
STATUS_LINE = {'type': 'command', 'command': "printf 'KIRO / kclaude'"}


def share_profile():
    """Mirror the user's own Claude setup; history and sessions stay separate.

    KCLAUDE_SHARE_PROFILE=0 keeps the profile fully separate.
    KCLAUDE_SHARED=a,b replaces the list of linked items."""
    if os.environ.get('KCLAUDE_SHARE_PROFILE', '1') == '0':
        return
    original = source_config()
    names = [n.strip() for n in os.environ.get('KCLAUDE_SHARED', '').split(',') if n.strip()] or SHARED
    for name in names:
        if '/' in name or name in ('.', '..', 'projects', 'settings.json'):
            raise ValueError('KCLAUDE_SHARED accepts top-level profile names only: ' + name)
        src, dst = original / name, PROFILE / name
        if not src.exists() or (dst.is_symlink() and dst.resolve() == src.resolve()):
            continue
        if dst.is_symlink() or dst.is_file():
            dst.unlink()
        elif dst.is_dir():
            dst.rename(ROOT / f'{name}.kclaude-old-{int(time.time())}')
        dst.symlink_to(src)
    # Rebuilt on every launch so hooks, permissions, plugins and env follow the source.
    source = original / 'settings.json'
    settings = json.loads(source.read_text()) if source.is_file() else {}
    settings.pop('apiKeyHelper', None)  # credentials come from the router
    settings.setdefault('statusLine', STATUS_LINE)
    settings['skipDangerousModePermissionPrompt'] = True
    write_private(PROFILE / 'settings.json', json.dumps(settings, indent=2, ensure_ascii=False) + '\n')
    # Project memory is per folder; link the one for this directory.
    folder = re.sub(r'[^a-zA-Z0-9]', '-', str(Path.cwd()))
    src, dst = original / 'projects' / folder / 'memory', PROFILE / 'projects' / folder / 'memory'
    if src.is_dir() and not dst.is_symlink():
        dst.parent.mkdir(parents=True, exist_ok=True)
        if dst.is_dir():
            for item in dst.iterdir():
                if not (src / item.name).exists():
                    shutil.move(str(item), str(src / item.name))
            shutil.rmtree(dst)
        dst.symlink_to(src)


def import_session(session_id=None):
    folder = re.sub(r'[^a-zA-Z0-9]', '-', str(Path.cwd()))
    original = source_config()
    source = original / 'projects' / folder
    if session_id:
        session_id = str(uuid.UUID(session_id))
        files = [source / (session_id + '.jsonl')]
        if not files[0].is_file():
            files = list((original / 'projects').glob(f'*/{session_id}.jsonl'))
            if len(files) > 1:
                raise RuntimeError('Session UUID occurs in multiple source project folders')
    else:
        files = sorted(source.glob('*.jsonl'), key=lambda p: p.stat().st_mtime, reverse=True)
    if not files or not files[0].is_file():
        raise RuntimeError('No original Claude session found. Use --from-claude-id UUID from its /status.')
    src = files[0]
    dest = PROFILE / 'projects' / folder
    dest.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src, dest / src.name)
    if src.with_suffix('').is_dir():
        # Copy file contents; do not leave links back into the original history.
        shutil.copytree(src.with_suffix(''), dest / src.stem, dirs_exist_ok=True)
    print(f'KIRO: copied Claude session {src.stem}; continuing as a new session.', file=sys.stderr)
    return ['--resume', src.stem, '--fork-session']


def claude_command():
    configured = os.environ.get('KCLAUDE_CLAUDE_BIN', 'claude')
    path = shutil.which(configured)
    if not path:
        raise RuntimeError('Claude Code is required. Install it from https://code.claude.com/docs/en/setup')
    if Path(path).resolve() == Path(__file__).resolve():
        raise ValueError('KCLAUDE_CLAUDE_BIN must point to Claude Code, not kclaude')
    return path


def launch_args(args):
    args = list(args)
    # Arguments after -- belong to Claude's prompt, not this wrapper.
    split = args.index('--') if '--' in args else len(args)
    args, tail = args[:split], args[split:]
    seat = None
    for flag in ('--seat', '--from-claude-id'):
        if flag in args:
            i = args.index(flag)
            if i + 1 == len(args):
                raise ValueError(flag + ' requires a value')
            value = args[i + 1]
            del args[i:i + 2]
            if flag == '--seat':
                seat = value
            else:
                if '--from-claude' in args:
                    raise ValueError('Choose one session import option')
                args = import_session(value) + args
    if '--from-claude' in args:
        args.remove('--from-claude')
        args = import_session() + args
    if seat and seat not in {a['name'] for a in config()['accounts'] if not a.get('disabled')}:
        raise ValueError('Unknown or disabled account: ' + seat)
    safe = '--safe' in args
    if safe:
        args.remove('--safe')
        if '--dangerously-skip-permissions' in args:
            raise ValueError('--safe conflicts with --dangerously-skip-permissions')
    explicit_mode = '--permission-mode' in args or any(a.startswith('--permission-mode=') for a in args)
    if safe and not explicit_mode:
        args = ['--permission-mode', 'default'] + args
    elif not safe and not explicit_mode and '--dangerously-skip-permissions' not in args:
        args = ['--dangerously-skip-permissions'] + args
    if '--model' not in args and not any(a.startswith('--model=') for a in args):
        args = ['--model', 'opus'] + args
    return args + tail, seat


def launch_env(seat):
    env = {k: v for k, v in os.environ.items()
           if not k.startswith(('ANTHROPIC_', 'CLAUDE_CODE_', 'KIRO_', 'KIROCC_', 'OTEL_'))
           and k not in ('CLAUDE_CONFIG_DIR', 'CLAUDECODE', 'HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY',
                         'http_proxy', 'https_proxy', 'all_proxy', 'NODE_TLS_REJECT_UNAUTHORIZED')}
    env.update(CLAUDE_CONFIG_DIR=str(PROFILE), ANTHROPIC_BASE_URL=URL,
               ANTHROPIC_AUTH_TOKEN=(ROOT / 'proxy.key').read_text().strip(),
               ANTHROPIC_DEFAULT_OPUS_MODEL='claude-opus-5-5[1m]',
               ANTHROPIC_DEFAULT_SONNET_MODEL='claude-sonnet-5-5[1m]',
               ANTHROPIC_DEFAULT_HAIKU_MODEL='claude-haiku-4.5',
               NODE_EXTRA_CA_CERTS=str(ROOT / 'router-cert.pem'),
               CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC='1', DISABLE_AUTOUPDATER='1')
    if seat:
        env['ANTHROPIC_CUSTOM_HEADERS'] = 'X-Kclaude-Seat: ' + seat
    return env


HELP = '''kclaude: Claude Code through your own Kiro accounts. Default model: Opus 5.5 (1M).

  kclaude accounts add personal          Add a Kiro API key with a hidden prompt
  kclaude accounts add work --key-file PATH [--region eu-central-1]
  kclaude accounts list|remove|enable|disable [NAME]
  kclaude                               Start Claude Code
  kclaude --safe                         Use normal Claude permission prompts
  kclaude --seat work                    Use one account without failover
  kclaude --model sonnet                 Choose Sonnet 5.5
  kclaude --from-claude                  Copy and fork the latest original chat here
  kclaude --from-claude-id UUID           Copy and fork a specific original chat
  kclaude --continue                     Continue the latest kclaude chat
  kclaude status|models|start|stop        Inspect or manage the local router
  kclaude version                       Show kclaude version

Claude Code options pass through, including -p, --resume and --permission-mode.
Default: --dangerously-skip-permissions. Use --safe to restore permission prompts.
Uses your ~/.claude setup (skills, plugins, hooks, MCP, memory); history stays separate.
KCLAUDE_SHARE_PROFILE=0 keeps the profile fully separate.
Get your own Kiro API keys at https://app.kiro.dev/. Docs: https://github.com/evgenspm/kclaude
'''


def main(args=None):
    args = list(sys.argv[1:] if args is None else args)
    if args in (['help'], ['--help'], ['-h']):
        print(HELP)
        return
    if args in (['version'], ['--version']):
        version = APP / 'VERSION'
        print('kclaude ' + (version.read_text().strip() if version.exists() else 'dev'))
        return
    initialize()
    if args and args[0] == 'accounts':
        return accounts(args[1:])
    if args == ['stop']:
        with (ROOT / 'start.lock').open('a') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            stopped = stop_locked()
        print('KIRO router stopped' if stopped else 'KIRO router is not running')
        return
    if args == ['status']:
        print(json.dumps(running() or {'service': 'kclaude-router', 'running': False,
                                      'accounts': len(config()['accounts']), 'state': str(ROOT)}, indent=2))
        return
    if args in (['models'], ['start']):
        start()
        if args == ['models']:
            for model in request('/v1/models')['data']:
                print(model['id'])
        else:
            print(json.dumps(request('/kclaude/status'), indent=2))
        return
    claude = claude_command()
    share_profile()
    args, seat = launch_args(args)
    start()
    count = sum(not a.get('disabled') for a in config()['accounts'])
    print(f'KIRO / kclaude / {seat or str(count) + " account(s), automatic failover"}', file=sys.stderr)
    os.execve(claude, [claude] + args, launch_env(seat))


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, ValueError, OSError, KeyError) as error:
        print(f'kclaude: {error}', file=sys.stderr)
        sys.exit(1)
    except (KeyboardInterrupt, EOFError):
        sys.exit(130)
