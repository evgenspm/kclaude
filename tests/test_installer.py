import hashlib
import io
import os
from pathlib import Path
import platform
import subprocess
import tempfile
import tarfile
import unittest

SOURCE = Path(__file__).resolve().parents[1]


class InstallerTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.home = Path(self.tmp.name)
        self.mocks = self.home / 'mocks'
        self.mocks.mkdir()
        fakecurl = self.mocks / 'curl'
        fakecurl.write_text('''#!/usr/bin/env python3
import os,shutil,sys
from pathlib import Path
url=next(a for a in sys.argv if a.startswith('https://'))
shutil.copyfile(Path(os.environ['FIXTURES'])/url.rsplit('/',1)[1],sys.argv[sys.argv.index('-o')+1])
''')
        fakecurl.chmod(0o755)
        self.env = dict(os.environ, HOME=str(self.home), PATH=str(self.mocks) + os.pathsep + os.environ['PATH'],
                        FIXTURES=str(self.home), KCLAUDE_INSTALL_DIR=str(self.home / 'app'),
                        KCLAUDE_BIN_DIR=str(self.home / 'bin'), KCLAUDE_VERSION='v1.0.0.0')
        osname = 'darwin' if platform.system() == 'Darwin' else 'linux'
        arch = 'arm64' if platform.machine() in ('arm64', 'aarch64') else 'amd64'
        self.asset = self.home / f'kclaude-{osname}-{arch}.tar.gz'
        self.bundle()

    def bundle(self, malicious=False):
        with tarfile.open(self.asset, 'w:gz') as archive:
            for name, data in {'kclaude': b'#!/bin/sh\necho installed\n', 'kclaude-router': b'fake',
                               'VERSION': b'1.0.0.0\n', 'LICENSE': b'license', 'NOTICE': b'notice'}.items():
                info = tarfile.TarInfo(name)
                info.size = len(data)
                archive.addfile(info, io.BytesIO(data))
            if malicious:
                info = tarfile.TarInfo('../escape')
                info.size = 1
                archive.addfile(info, io.BytesIO(b'x'))
        (self.home / 'SHA256SUMS').write_text(hashlib.sha256(self.asset.read_bytes()).hexdigest() + '  ' + self.asset.name + '\n')

    def install(self):
        return subprocess.run(['sh', str(SOURCE / 'install.sh')], env=self.env, text=True, capture_output=True, timeout=15)

    def test_install_is_repeatable_and_preserves_claude_settings(self):
        original = self.home / '.claude/settings.json'
        original.parent.mkdir()
        original.write_text('{"original":true}')
        for _ in range(2):
            result = self.install()
            self.assertEqual(result.returncode, 0, result.stderr)
        command = self.home / 'bin/kclaude'
        self.assertTrue(command.is_symlink())
        self.assertEqual(subprocess.check_output([str(command)], text=True).strip(), 'installed')
        self.assertEqual(original.read_text(), '{"original":true}')
        self.assertFalse((self.home / '.zshrc').exists())

    def test_refuses_to_replace_unrelated_command(self):
        (self.home / 'bin').mkdir()
        command = self.home / 'bin/kclaude'
        command.write_text('personal launcher')
        self.assertNotEqual(self.install().returncode, 0)
        self.assertEqual(command.read_text(), 'personal launcher')

    def test_checksum_failure_does_not_install(self):
        self.asset.write_bytes(b'corrupt')
        self.assertNotEqual(self.install().returncode, 0)
        self.assertFalse((self.home / 'bin/kclaude').exists())

    def test_archive_path_traversal_rejected(self):
        self.bundle(malicious=True)
        self.assertNotEqual(self.install().returncode, 0)
        self.assertFalse((self.home / 'bin/kclaude').exists())

    def test_failed_copy_does_not_poison_reinstall(self):
        interrupted = self.home / 'interrupted.sh'
        interrupted.write_text((SOURCE / 'install.sh').read_text().replace(
            'shutil.copytree(unpacked, staging, dirs_exist_ok=True)',
            "raise OSError('injected disk full')"))
        result = subprocess.run(['sh', str(interrupted)], env=self.env, text=True, capture_output=True, timeout=15)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.home / 'app/v1.0.0.0').exists())
        result = self.install()
        self.assertEqual(result.returncode, 0, result.stderr)
