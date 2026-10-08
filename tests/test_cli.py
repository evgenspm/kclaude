import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import socket
import ssl
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock
import urllib.error
import urllib.request

SOURCE = Path(__file__).resolve().parents[1]


class CLITest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.home = Path(self.tmp.name)
        self.env = mock.patch.dict(os.environ, {'HOME': str(self.home), 'KCLAUDE_HOME': str(self.home / 'state'), 'KCLAUDE_PORT': '17391'})
        self.env.start()
        self.addCleanup(self.env.stop)
        spec = importlib.util.spec_from_file_location('kclaude_test', SOURCE / 'cli/kclaude.py')
        self.cli = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.cli)
        self.cli.initialize()

    def add_account(self, name='personal'):
        key = self.home / 'input.key'
        key.write_text('ksk_unit_test_only')
        with contextlib.redirect_stdout(io.StringIO()), mock.patch.object(self.cli, 'stop_locked', return_value=False):
            self.cli.accounts(['add', name, '--key-file', str(key)])

    def test_private_state_without_hardcoded_accounts(self):
        self.assertEqual(self.cli.config(), {'accounts': []})
        self.assertEqual(self.cli.ROOT.stat().st_mode & 0o777, 0o700)
        self.assertEqual((self.cli.ROOT / 'proxy.key').stat().st_mode & 0o777, 0o600)
        self.assertFalse((self.home / '.claude').exists())

    def test_account_lifecycle_and_secret_not_printed(self):
        self.add_account()
        key = self.cli.ROOT / 'keys/personal.key'
        self.assertEqual(key.stat().st_mode & 0o777, 0o600)
        self.assertNotIn('ksk_', (self.cli.ROOT / 'accounts.json').read_text())
        output = io.StringIO()
        with contextlib.redirect_stdout(output), mock.patch.object(self.cli, 'stop_locked', return_value=False):
            self.cli.accounts(['list'])
            self.cli.accounts(['disable', 'personal'])
            self.assertTrue(self.cli.config()['accounts'][0]['disabled'])
            self.cli.accounts(['enable', 'personal'])
            self.assertFalse(self.cli.config()['accounts'][0]['disabled'])
            self.cli.accounts(['remove', 'personal'])
        self.assertEqual(self.cli.config()['accounts'], [])
        self.assertFalse(key.exists())
        self.assertNotIn('ksk_unit_test_only', output.getvalue())

    def test_bad_account_and_bad_key_do_not_change_config(self):
        key = self.home / 'input.key'
        key.write_text('wrong')
        for name in ('../escape', 'header\ninjection', 'valid'):
            with self.assertRaises(ValueError):
                self.cli.accounts(['add', name, '--key-file', str(key)])
        self.assertEqual(self.cli.config()['accounts'], [])

    def test_readding_account_replaces_it(self):
        self.add_account()
        self.add_account()
        self.assertEqual(len(self.cli.config()['accounts']), 1)

    def test_default_and_explicit_permission_modes(self):
        args, _ = self.cli.launch_args(['-p', 'hello'])
        self.assertIn('--dangerously-skip-permissions', args)
        self.assertEqual(args[:2], ['--model', 'opus'])
        for explicit in (['--safe'], ['--permission-mode', 'plan'], ['--permission-mode=default']):
            args, _ = self.cli.launch_args(explicit)
            self.assertNotIn('--dangerously-skip-permissions', args)
        with self.assertRaises(ValueError):
            self.cli.launch_args(['--safe', '--dangerously-skip-permissions'])

    def test_argument_boundary_and_account_validation(self):
        self.add_account()
        args, seat = self.cli.launch_args(['--seat', 'personal', '--', '--safe'])
        self.assertEqual(seat, 'personal')
        self.assertEqual(args[-2:], ['--', '--safe'])
        self.assertIn('--dangerously-skip-permissions', args)
        for args in (['--seat'], ['--seat', 'missing'], ['--from-claude-id']):
            with self.assertRaises(ValueError):
                self.cli.launch_args(args)

    def test_environment_is_separate(self):
        injected = {'ANTHROPIC_API_KEY': 'old', 'CLAUDE_CODE_USE_BEDROCK': '1',
                    'CLAUDE_CONFIG_DIR': '/old', 'HTTPS_PROXY': 'http://proxy', 'KIRO_API_KEY': 'old'}
        with mock.patch.dict(os.environ, injected):
            env = self.cli.launch_env('personal')
        self.assertNotIn('ANTHROPIC_API_KEY', env)
        self.assertNotIn('CLAUDE_CODE_USE_BEDROCK', env)
        self.assertNotIn('HTTPS_PROXY', env)
        self.assertNotIn('KIRO_API_KEY', env)
        self.assertEqual(env['CLAUDE_CONFIG_DIR'], str(self.cli.PROFILE))
        self.assertEqual(env['ANTHROPIC_BASE_URL'], self.cli.URL)
        self.assertEqual(env['ANTHROPIC_CUSTOM_HEADERS'], 'X-Kclaude-Seat: personal')

    def test_import_forks_original_history_even_after_cwd_change(self):
        sid = 'f716ee5e-1e61-471b-9d91-6659da7378d5'
        source = self.home / '.claude/projects/-old-cwd'
        source.mkdir(parents=True)
        transcript = source / (sid + '.jsonl')
        transcript.write_text('{"test":"original"}\n')
        sidecar = source / sid / 'subagents'
        sidecar.mkdir(parents=True)
        (sidecar / 'agent.jsonl').write_text('agent history')
        with contextlib.redirect_stderr(io.StringIO()):
            args = self.cli.import_session(sid)
        self.assertEqual(args, ['--resume', sid, '--fork-session'])
        folder = re.sub(r'[^a-zA-Z0-9]', '-', str(Path.cwd()))
        copy = self.cli.PROFILE / 'projects' / folder / (sid + '.jsonl')
        self.assertEqual(copy.read_bytes(), transcript.read_bytes())
        copy.write_text('modified copy')
        self.assertIn('original', transcript.read_text())
        self.assertTrue((copy.with_suffix('') / 'subagents/agent.jsonl').exists())
        with self.assertRaises(ValueError):
            self.cli.import_session('../not-a-uuid')

    def test_profile_mirrors_source_claude_setup(self):
        own = self.home / '.claude'
        folder = re.sub(r'[^a-zA-Z0-9]', '-', str(Path.cwd()))
        (own / 'skills/mine').mkdir(parents=True)
        (own / 'CLAUDE.md').write_text('my rules')
        (own / 'settings.json').write_text('{"hooks":{"SessionStart":[]},"model":"opus","apiKeyHelper":"x"}')
        (own / 'projects' / folder / 'memory').mkdir(parents=True)
        stale = self.cli.PROFILE / 'projects' / folder / 'memory'
        stale.mkdir(parents=True)
        (stale / 'note.md').write_text('keep me')
        with mock.patch.dict(os.environ, {'KCLAUDE_SOURCE_CONFIG': '', 'KCLAUDE_SHARED': '', 'KCLAUDE_SHARE_PROFILE': '1'}):
            os.environ.pop('KCLAUDE_SOURCE_CONFIG')
            for _ in range(2):  # repeat launches are idempotent
                self.cli.share_profile()
        self.assertEqual((self.cli.PROFILE / 'CLAUDE.md').read_text(), 'my rules')
        self.assertTrue((self.cli.PROFILE / 'skills').is_symlink())
        settings = json.loads((self.cli.PROFILE / 'settings.json').read_text())
        self.assertIn('hooks', settings)
        self.assertEqual(settings['model'], 'opus')
        self.assertNotIn('apiKeyHelper', settings)
        self.assertTrue(settings['skipDangerousModePermissionPrompt'])
        self.assertTrue(stale.is_symlink())
        self.assertEqual((own / 'projects' / folder / 'memory/note.md').read_text(), 'keep me')
        self.assertEqual(json.loads((own / 'settings.json').read_text())['apiKeyHelper'], 'x')
        with mock.patch.dict(os.environ, {'KCLAUDE_SHARED': '../escape'}), self.assertRaises(ValueError):
            self.cli.share_profile()

    def test_share_profile_can_be_disabled(self):
        (self.home / '.claude').mkdir()
        (self.home / '.claude/CLAUDE.md').write_text('mine')
        with mock.patch.dict(os.environ, {'KCLAUDE_SHARE_PROFILE': '0'}):
            self.cli.share_profile()
        self.assertFalse((self.cli.PROFILE / 'CLAUDE.md').exists())

    def test_start_without_accounts_fails_with_next_step(self):
        with mock.patch.object(self.cli, 'running', return_value=None):
            with self.assertRaisesRegex(RuntimeError, 'accounts add'):
                self.cli.start()

    def test_forged_router_proof_does_not_receive_bearer_or_trigger_stop(self):
        connection = mock.MagicMock()
        response = connection.getresponse.return_value
        response.status = 200
        response.read.return_value = b'{"service":"kclaude-router","proof":"forged"}'
        with mock.patch.object(self.cli.ssl, 'create_default_context'), \
                mock.patch.object(self.cli.http.client, 'HTTPSConnection', return_value=connection):
            self.assertIsNone(self.cli.running())
            self.assertFalse(self.cli.stop_locked())
        self.assertEqual(connection.request.call_count, 2)
        for call in connection.request.call_args_list:
            self.assertEqual(call.args[0], 'GET')
            self.assertTrue(call.args[1].startswith('/health?challenge='))
            self.assertNotIn('Authorization', call.kwargs.get('headers', {}))


@unittest.skipUnless(os.environ.get('KCLAUDE_TEST_ROUTER'), 'set KCLAUDE_TEST_ROUTER to a built host binary')
class RouterIntegration(unittest.TestCase):
    def test_real_process_auth_models_launch_and_stop(self):
        with tempfile.TemporaryDirectory() as temporary:
            home = Path(temporary).resolve()
            with socket.socket() as sock:
                sock.bind(('127.0.0.1', 0))
                port = sock.getsockname()[1]
            env = dict(os.environ, HOME=str(home), KCLAUDE_HOME=str(home / 'state'), KCLAUDE_PORT=str(port),
                       KCLAUDE_ROUTER=os.environ['KCLAUDE_TEST_ROUTER'])
            fake = home / 'claude'
            fake.write_text('#!' + sys.executable + '\nimport json,os,sys\nprint(json.dumps({"args":sys.argv[1:],"config":os.environ["CLAUDE_CONFIG_DIR"]}))\n')
            fake.chmod(0o755)
            env['KCLAUDE_CLAUDE_BIN'] = str(fake)
            key = home / 'input.key'
            key.write_text('ksk_integration_test_only')

            def call(*args):
                return subprocess.run([sys.executable, str(SOURCE / 'cli/kclaude.py'), *args], env=env,
                                      text=True, capture_output=True, timeout=35, check=True)

            call('accounts', 'add', 'test', '--key-file', str(key))
            try:
                status = json.loads(call('start').stdout)
                self.assertEqual(status['state'], str(home / 'state'))
                tls = ssl.create_default_context(cafile=str(home / 'state/router-cert.pem'))
                opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=tls))
                endpoint = f'https://127.0.0.1:{port}'
                with self.assertRaises(urllib.error.HTTPError) as error:
                    opener.open(endpoint + '/v1/models')
                self.assertEqual(error.exception.code, 401)
                error.exception.close()
                token = (home / 'state/proxy.key').read_text().strip()
                req = urllib.request.Request(endpoint + '/v1/models', headers={'Authorization': 'Bearer ' + token, 'Origin': 'https://example.com'})
                with self.assertRaises(urllib.error.HTTPError) as error:
                    opener.open(req)
                self.assertEqual(error.exception.code, 403)
                error.exception.close()
                self.assertIn('claude-opus-5.5', call('models').stdout)
                launched = json.loads(call('--safe', '-p', 'hello').stdout)
                self.assertEqual(launched['config'], str(home / 'state/claude'))
                self.assertIn('--permission-mode', launched['args'])
                self.assertNotIn('--dangerously-skip-permissions', launched['args'])
                self.assertFalse((home / '.claude').exists())
                # Hold a request body open while stopping the listener. The process
                # must drain the handler before exiting, without calling upstream.
                with tls.wrap_socket(socket.create_connection(('127.0.0.1', port), timeout=3), server_hostname='127.0.0.1') as pending:
                    body = b'{invalid}'
                    pending.sendall((f'POST /v1/messages HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer {token}\r\nContent-Length: {len(body)}\r\n\r\n').encode() + body[:1])
                    time.sleep(.1)
                    call('stop')
                    time.sleep(.1)
                    os.kill(status['pid'], 0)
                    pending.sendall(body[1:])
                    response = pending.recv(4096)
                    self.assertIn(b'400', response.split(b'\r\n', 1)[0])
                call('accounts', 'disable', 'test')
                self.assertFalse(json.loads(call('status').stdout)['running'])
            finally:
                call('stop')


if __name__ == '__main__':
    unittest.main()
