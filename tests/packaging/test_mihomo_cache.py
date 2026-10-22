#!/usr/bin/env python3
"""Exercise the real builder/cache protocol with a tiny source archive and fake Go.

No network, installation, TUN, or running gateway is used.
"""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest

REPO = Path(__file__).resolve().parents[2]
PIN = '971dd4533e4e2c3dad7473e8115200da8c0d7471b4b61da54da896345c5b3850'
FAKE_GO = r'''
import json, os, pathlib, sys
args = sys.argv[1:]
if args[0] == 'env':
    values = {k: os.environ.get(k, '') for k in args[2:]}
    values['GOVERSION'] = os.environ.get('FAKE_GO_VERSION', 'go1.25.3')
    print(json.dumps(values))
else:
    with open(os.environ['CALLS'], 'a') as log:
        log.write(args[0] + '\n')
    assert pathlib.Path('source.txt').read_text() == 'after\n'
    if args[0] == 'test':
        assert pathlib.Path('config/opensurge_ipv6_test.go').exists()
        assert pathlib.Path('dns/opensurge_filter.go').exists()
        assert pathlib.Path('dns-patched.txt').read_text() == 'filtered\n'
        sys.exit(int(os.environ.get('FAIL_TEST', '0')))
    if os.environ.get('MUTATE_INPUT'):
        with open(os.environ['MUTATE_INPUT'], 'a') as out:
            out.write('\n// changed during compilation\n')
    if os.environ.get('FAIL_BUILD'):
        sys.exit(2)
    path = pathlib.Path(args[args.index('-o') + 1])
    path.write_text('#!/bin/sh\n# built for ' + os.environ['GOARCH'] + '\n')
    path.chmod(0o755)
'''


class MihomoCache(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='opensurge-cache-test-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.scripts = self.root / 'scripts'
        self.scripts.mkdir()
        for name in ('build-opensurge-mihomo.sh', 'mihomo-build-cache.py'):
            shutil.copy2(REPO / 'scripts' / name, self.scripts / name)
        self.builder = self.scripts / 'build-opensurge-mihomo.sh'
        patchdir = self.root / 'patches/mihomo'
        (patchdir / 'overlay/config').mkdir(parents=True)
        (patchdir / 'overlay/listener').mkdir()
        (patchdir / 'overlay/dns').mkdir()
        self.filter_overlay = patchdir / 'overlay/dns/opensurge_filter.go'
        self.filter_overlay.write_text('// filter fixture\n')
        self.filter_patch = patchdir / '0002-opensurge-external-dns-filter.patch'
        self.filter_patch.write_text('--- /dev/null\n+++ b/dns-patched.txt\n@@ -0,0 +1 @@\n+filtered\n')
        self.patch = patchdir / '0001-opensurge-packet-listener.patch'
        self.patch.write_text('--- a/source.txt\n+++ b/source.txt\n@@ -1 +1 @@\n-before\n+after\n')
        self.overlay = patchdir / 'overlay/config/opensurge_ipv6_test.go'
        self.overlay.write_text('// fixture test\n')
        self.source = self.root / 'source'
        (self.source / 'config').mkdir(parents=True)
        (self.source / 'source.txt').write_text('before\n')
        self.archive = self.root / 'source.tar.gz'
        self.pin_archive(PIN)
        self.go = self.root / 'go'
        self.go.write_text(f'#!{sys.executable}\n' + FAKE_GO)
        self.go.chmod(0o755)
        self.cache = self.root / 'cache'
        self.output = self.root / 'output/mihomo'
        self.calls = self.root / 'calls'
        self.env = dict(os.environ, GO_BIN=str(self.go), CALLS=str(self.calls),
                        OPENSURGE_MIHOMO_SOURCE_ARCHIVE=str(self.archive),
                        OPENSURGE_MIHOMO_OUTPUT=str(self.output),
                        OPENSURGE_MIHOMO_CACHE_DIR=str(self.cache),
                        OPENSURGE_MIHOMO_ARCH='arm64', GOFLAGS='', LC_ALL='C')
        for key in ('BASH_ENV', 'ENV', 'FAIL_TEST', 'FAIL_BUILD', 'MUTATE_INPUT'):
            self.env.pop(key, None)

    def pin_archive(self, old):
        with tarfile.open(self.archive, 'w:gz') as archive:
            archive.add(self.source, arcname='mihomo')
        self.pin = hashlib.sha256(self.archive.read_bytes()).hexdigest()
        self.builder.write_text(self.builder.read_text().replace(old, self.pin))

    def run_build(self, expected=0, **env):
        result = subprocess.run(['/bin/bash', str(self.builder)], env=dict(self.env, **env),
                                text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=20)
        self.assertEqual(result.returncode, expected, result.stdout)
        return result.stdout

    def builds(self):
        return self.calls.read_text().splitlines().count('build') if self.calls.exists() else 0

    def test_cold_build_then_reuse_into_new_staging_output(self):
        self.run_build()
        original = self.output.read_bytes()
        destination = self.root / 'another-staging/mihomo'
        result = self.run_build(OPENSURGE_MIHOMO_OUTPUT=str(destination))
        self.assertIn('Reused verified mihomo cache', result)
        self.assertEqual(self.calls.read_text().splitlines(), ['test', 'test', 'build'])
        self.assertEqual(destination.read_bytes(), original)
        self.assertTrue(os.access(destination, os.X_OK))

    def test_legacy_output_without_manifest_is_not_trusted(self):
        self.output.parent.mkdir()
        self.output.write_text('old unverified executable')
        self.output.chmod(0o755)
        self.run_build()
        self.assertEqual(self.builds(), 1)
        self.assertNotIn(b'old unverified', self.output.read_bytes())

    def test_identical_checkout_reuses_shared_cache(self):
        self.run_build()
        checkout = self.root / 'clean-export'
        shutil.copytree(self.scripts, checkout / 'scripts')
        shutil.copytree(self.root / 'patches', checkout / 'patches')
        result = subprocess.run(['/bin/bash', str(checkout / 'scripts/build-opensurge-mihomo.sh')],
                                env=self.env, text=True, stdout=subprocess.PIPE,
                                stderr=subprocess.STDOUT, timeout=20)
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertIn('Reused verified', result.stdout)
        self.assertEqual(self.builds(), 1)

    def test_caller_relative_paths_survive_source_directory_change(self):
        self.run_build()
        result = subprocess.run(['/bin/bash', str(self.builder)], cwd=self.root,
                                env=dict(self.env, GO_BIN='./go',
                                         OPENSURGE_MIHOMO_SOURCE_ARCHIVE='source.tar.gz',
                                         OPENSURGE_MIHOMO_CACHE_DIR='cache',
                                         OPENSURGE_MIHOMO_OUTPUT='relative/mihomo'),
                                text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=20)
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertIn('Reused verified', result.stdout)
        self.assertTrue((self.root / 'relative/mihomo').exists())
        self.assertEqual(self.builds(), 1)

    def test_concurrent_builds_publish_consistent_cache(self):
        processes = [subprocess.Popen(['/bin/bash', str(self.builder)],
                     env=dict(self.env, OPENSURGE_MIHOMO_OUTPUT=str(self.root / f'staging-{i}/mihomo')),
                     text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT) for i in range(2)]
        for process in processes:
            output, _ = process.communicate(timeout=20)
            self.assertEqual(process.returncode, 0, output)
        count = self.builds()
        self.assertIn('Reused verified', self.run_build())
        self.assertEqual(self.builds(), count)

    def test_changed_patch_overlay_and_builder_invalidate(self):
        self.run_build()
        for count, path in enumerate([self.patch, self.filter_patch, self.overlay, self.filter_overlay, self.builder], 2):
            with path.open('a') as file:
                file.write('\n')
            self.run_build()
            self.assertEqual(self.builds(), count)

    def test_added_and_removed_overlay_files_invalidate(self):
        self.run_build()
        extra = self.overlay.parent / 'extra.go'
        extra.write_text('// extra\n')
        self.run_build()
        self.assertEqual(self.builds(), 2)
        extra.unlink()
        self.assertIn('Reused verified', self.run_build())
        self.assertEqual(self.builds(), 2)  # Original content key is still valid.

    def test_pinned_source_change_invalidate(self):
        self.run_build()
        (self.source / 'extra').write_text('new source')
        self.pin_archive(self.pin)
        self.run_build()
        self.assertEqual(self.builds(), 2)

    def test_corrupt_archive_rejected_even_with_valid_binary_cache(self):
        self.run_build()
        self.archive.write_bytes(b'corrupted')
        self.assertIn('pinned SHA-256', self.run_build(expected=1))
        self.assertEqual(self.builds(), 1)

    def test_arch_version_toolchain_and_flags_invalidate(self):
        self.run_build()
        for count, env in enumerate([
            {'OPENSURGE_MIHOMO_ARCH': 'x86_64'},
            {'OPENSURGE_MIHOMO_VERSION': 'different-version'},
            {'FAKE_GO_VERSION': 'go1.26.0'},
            {'GOFLAGS': '-p=2'}, {'GOARM64': 'v8.1'}, {'GOEXPERIMENT': 'newinliner'},
        ], 2):
            self.run_build(**env)
            self.assertEqual(self.builds(), count)
        self.assertIn('Reused verified', self.run_build())

    def test_corrupt_missing_or_untrusted_manifest_rebuilds(self):
        self.run_build()
        manifest = next(self.cache.glob('*/manifest.json'))
        for count, mutation in enumerate(['broken JSON', 'wrong inputs', 'missing'], 2):
            if mutation == 'missing':
                manifest.unlink()
            elif mutation == 'wrong inputs':
                record = json.loads(manifest.read_text())
                record['inputs']['version'] = 'old'
                manifest.write_text(json.dumps(record))
            else:
                manifest.write_text('{broken')
            self.run_build()
            self.assertEqual(self.builds(), count)

    def test_corrupt_missing_or_nonexecutable_binary_rebuilds(self):
        self.run_build()
        binary = next(self.cache.glob('*/mihomo'))
        for count, mutation in enumerate(['corrupt', 'missing', 'nonexecutable'], 2):
            if mutation == 'missing':
                binary.unlink()
            elif mutation == 'nonexecutable':
                binary.chmod(0o644)
            else:
                binary.write_text('wrong binary')
            self.run_build()
            self.assertEqual(self.builds(), count)

    def test_failed_test_or_build_never_publishes(self):
        for env in ({'FAIL_TEST': '2'}, {'FAIL_BUILD': '1'}):
            self.run_build(expected=2, **env)
            self.assertFalse(self.output.exists())
            self.assertFalse(list(self.cache.glob('*/manifest.json')))
        self.run_build()
        self.assertTrue(self.output.exists())

    def test_changing_inputs_mid_build_refuses_publication(self):
        self.assertIn('inputs changed', self.run_build(expected=1, MUTATE_INPUT=str(self.overlay)))
        self.assertFalse(self.output.exists())
        self.assertFalse(list(self.cache.glob('*/manifest.json')))

    def test_external_build_flags_bypass_cache(self):
        for _ in range(2):
            self.assertIn('Built without caching', self.run_build(GOFLAGS='-overlay=external.json'))
        self.assertEqual(self.builds(), 2)
        self.assertFalse(list(self.cache.glob('*/manifest.json')))


if __name__ == '__main__':
    unittest.main(verbosity=2)
