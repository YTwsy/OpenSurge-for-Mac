#!/usr/bin/env python3
"""Check explicit core selection without building Go or touching host networking."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

RUNNER = Path(__file__).resolve().parent / 'lab.sh'


class LabCoreSelection(unittest.TestCase):
    def run_prepare(self, selection):
        with tempfile.TemporaryDirectory(prefix='opensurge-lab-core-') as directory:
            root = Path(directory)
            (root / 'scripts').mkdir()
            (root / 'logs').mkdir()
            custom = root / 'selected core'
            custom.write_text('#!/bin/sh\nexit 0\n')
            if selection == 'valid':
                custom.chmod(0o755)
            builder = root / 'scripts/build-opensurge-mihomo.sh'
            builder.write_text('#!/bin/sh\nset -eu\n: >"$ROOT/builder-called"\n: >"$OPENSURGE_MIHOMO_OUTPUT"\n')
            builder.chmod(0o755)
            function = RUNNER.read_text().split('build_ipv6_lab_binaries() {', 1)[1].split('\n}\n', 1)[0]
            script = '''set -eu
require_command() { command -v "$1" >/dev/null; }
go() { :; }
STATE_DIR="$ROOT"
IPV6_PACKET_BINARY="$ROOT/broker"
PATCHED_MIHOMO_BINARY="$ROOT/default-core"
build_ipv6_lab_binaries() {
''' + function + '''
}
build_ipv6_lab_binaries
printf '%s\n' "$PATCHED_MIHOMO_BINARY" > "$ROOT/selected-path"
'''
            env = dict(os.environ, ROOT=str(root), GOCACHE=str(root / 'cache'),
                       GOMODCACHE=str(root / 'modcache'), LC_ALL='C')
            env.pop('OMG_LAB_MIHOMO_BINARY', None)
            if selection != 'default':
                env['OMG_LAB_MIHOMO_BINARY'] = str(custom)
            result = subprocess.run(['/bin/bash', '-c', script], env=env,
                                    capture_output=True, text=True, timeout=10)
            called = (root / 'builder-called').exists()
            selected_file = root / 'selected-path'
            selected = selected_file.read_text().strip() if selected_file.exists() else None
            return result, called, selected, str(custom), str(root / 'default-core')

    def test_explicit_core_is_used_without_rebuilding_for_host_architecture(self):
        result, called, selected, custom, _ = self.run_prepare('valid')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(called)
        self.assertEqual(selected, custom)

    def test_invalid_override_fails_without_silent_fallback(self):
        result, called, selected, _, _ = self.run_prepare('invalid')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('mihomo binary is not executable', result.stderr)
        self.assertFalse(called)
        self.assertIsNone(selected)

    def test_default_still_uses_pinned_builder(self):
        result, called, selected, _, default = self.run_prepare('default')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(called)
        self.assertEqual(selected, default)


if __name__ == '__main__':
    unittest.main(verbosity=2)
