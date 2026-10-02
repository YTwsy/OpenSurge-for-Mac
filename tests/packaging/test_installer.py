#!/usr/bin/env python3
"""Run installer scripts in a disposable filesystem with fake system commands.

This verifies script ordering and file retention, not launchd, authorization,
PackageKit, SMAppService or host networking. No root privileges are used.
"""
import json
import os
import pathlib
import plistlib
import shutil
import subprocess
import sys
import tempfile
import unittest

REPO = pathlib.Path(__file__).resolve().parents[2]
SHIM = r'''
import json, os, pathlib, subprocess, sys
name, args = pathlib.Path(sys.argv[0]).name, sys.argv[1:]
with open(os.environ['FIXTURE_LOG'], 'a') as output:
    output.write(json.dumps([name, *args]) + '\n')
if name == 'id':
    print('staff admin' if args[0] == '-Gn' else ('501' if len(args) > 1 else '0'))
elif name == 'stat':
    print('tester')
elif name == 'dscl':
    print('NFSHomeDirectory: ' + os.environ['FIXTURE_HOME'])
elif name == 'pgrep':
    sys.exit(1)
elif name in ('ps', 'launchctl', 'chown', 'pmset', 'pkgutil'):
    pass
elif name == 'kill':
    raise SystemExit('unexpected process signal in filesystem fixture')
elif name == 'install':
    filtered = []
    while args:
        item = args.pop(0)
        if item in ('-o', '-g'):
            args.pop(0)
        else:
            filtered.append(item)
    sys.exit(subprocess.call(['/usr/bin/install', *filtered]))
elif name == 'opensurge-install-config':
    if '--output' in args:
        pathlib.Path(args[args.index('--output') + 1]).write_text('fixture: first-install\n')
elif name == 'omg':
    print(json.dumps({'gateway': os.environ.get('FIXTURE_GATEWAY', 'stopped')}))
elif name == 'omg-recovery':
    sys.exit(int(os.environ.get('FIXTURE_STOP_EXIT', '0')))
else:
    raise SystemExit('unexpected command: ' + name)
'''


class InstallerScripts(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='opensurge-installer-contract-')
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name).resolve()
        self.fs = self.root / 'fs'
        self.home = self.fs / 'Users/tester'
        self.system = self.fs / 'Library/Application Support/OpenSurge'
        self.user = self.home / 'Library/Application Support/OpenSurge'
        self.scripts = self.root / 'scripts'
        self.bin = self.root / 'commands'
        self.log = self.root / 'commands.jsonl'
        self.env = dict(os.environ, PATH=f'{self.bin}:/usr/bin:/bin:/usr/sbin:/sbin',
                        FIXTURE_LOG=str(self.log), FIXTURE_HOME=str(self.home))
        # Do not inherit shell-startup hooks from the caller.
        self.env.pop('BASH_ENV', None)
        self.env.pop('ENV', None)
        self.scripts.mkdir()
        self.bin.mkdir()
        for name in ('id', 'stat', 'dscl', 'pgrep', 'ps', 'launchctl', 'chown',
                     'pmset', 'pkgutil', 'kill', 'install', 'omg', 'omg-recovery',
                     'opensurge-install-config'):
            self.write(self.bin / name, f'#!{sys.executable}\n' + SHIM, executable=True)
        for source in [*(REPO / 'packaging/pkg-scripts').iterdir(), REPO / 'scripts/uninstall-gui.sh']:
            text = source.read_text()
            # Relocate only literal absolute install paths, never $USER_HOME paths.
            for prefix in ('/Library/', '/Applications/', '/var/run/'):
                for boundary in ('"', ' '):
                    text = text.replace(boundary + prefix, boundary + str(self.fs) + prefix)
            for command in ('/bin/launchctl', '/bin/ps', '/bin/kill', '/usr/bin/pgrep',
                            '/usr/bin/pmset', '/usr/sbin/pkgutil'):
                text = text.replace(command, str(self.bin / pathlib.Path(command).name))
            self.write(self.scripts / source.name, text, executable=True)
        shutil.copy2(self.bin / 'omg-recovery', self.scripts / 'omg-recovery')
        for directory in ('Library/LaunchDaemons', 'Library/PrivilegedHelperTools',
                          'Library/Logs/OpenSurge', 'var/run/opensurge'):
            (self.fs / directory).mkdir(parents=True, exist_ok=True)
        for directory in ('bin', 'share', 'data', 'runtime'):
            (self.system / directory).mkdir(parents=True, exist_ok=True)
        (self.user / 'bin').mkdir(parents=True)
        (self.home / 'Library/LaunchAgents').mkdir()
        for name in ('omg', 'opensurge-install-config'):
            shutil.copy2(self.bin / name, self.system / 'bin' / name)
        for name in ('com.opensurge.control.plist', 'com.opensurge.helper.plist'):
            shutil.copy2(REPO / 'packaging/launchd' / name, self.system / 'share' / name)
        self.write(self.system / 'share/config.yaml', 'fixture: example\n')
        self.write(self.system / 'share/opensurge-control', 'fixture control', executable=True)
        self.write(self.fs / 'Library/PrivilegedHelperTools/com.opensurge.helper', 'fixture helper')
        self.write(self.system / 'config.yaml', 'fixture: existing\n')
        self.write(self.system / 'data/policies.json', 'existing policies')
        self.write(self.system / 'runtime/history', 'existing history')
        self.write(self.system / 'runtime/sleep-prevention-owned', 'owned')
        self.write(self.user / 'credentials', 'fixture-only credentials')
        self.write(self.user / 'control-endpoint.json', '{}')
        self.write(self.user / 'bin/opensurge-control', 'old control')
        self.write(self.fs / 'Library/Logs/OpenSurge/history.log', 'existing log')
        self.write(self.fs / 'Applications/OpenSurge.app/Contents/MacOS/OpenSurgeDesktop', 'new host')
        self.write(self.fs / 'Applications/OpenSurge Menu Bar.app/Contents/MacOS/OpenSurgeMenuBar', 'legacy host')

    def write(self, path, text, executable=False):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
        if executable:
            path.chmod(0o755)

    def run_script(self, name, *args, expected=0, **environment):
        result = subprocess.run(['/bin/bash', str(self.scripts / name), *args],
                                env=dict(self.env, **environment), text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=15)
        self.assertEqual(result.returncode, expected, result.stdout)
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_upgrade_recovery_guard_and_cleanup_order(self):
        for stage in ('prepared', 'gateway_stopped_waiting_router_dhcp', 'unknown'):
            self.write(self.user / 'recovery.json', json.dumps({'stage': stage}))
            calls = self.run_script('preinstall', expected=2)
            self.assertFalse(any(c[0] in ('launchctl', 'omg-recovery', 'pmset') for c in calls))
        for stage in ('idle', 'complete', 'complete_static'):
            self.log.write_text('')
            self.write(self.user / 'recovery.json', json.dumps({'stage': stage}))
            self.write(self.system / 'runtime/sleep-prevention-owned', 'owned')
            calls = self.run_script('preinstall')
            actions = [c for c in calls if c[0] in ('launchctl', 'omg-recovery', 'pmset')]
            self.assertEqual(actions, [
                ['launchctl', 'bootout', 'gui/501/com.opensurge.control'],
                ['omg-recovery', 'stop', '--config', str(self.system / 'config.yaml')],
                ['pmset', '-a', 'disablesleep', '0'],
                ['launchctl', 'bootout', 'system/com.opensurge.helper']])
            self.assertEqual((self.system / 'config.yaml').read_text(), 'fixture: existing\n')

    def test_failed_gateway_cleanup_preserves_helper_and_sleep_marker(self):
        calls = self.run_script('preinstall', expected=9, FIXTURE_STOP_EXIT='9')
        self.assertFalse(any(c[0] == 'pmset' or c[-1] == 'system/com.opensurge.helper' for c in calls))
        self.assertTrue((self.system / 'runtime/sleep-prevention-owned').exists())

    def test_postinstall_preserves_data_and_installs_exact_user_agent(self):
        self.run_script('postinstall')
        self.assertEqual((self.system / 'config.yaml').read_text(), 'fixture: existing\n')
        self.assertEqual((self.system / 'data/policies.json').read_text(), 'existing policies')
        self.assertEqual((self.user / 'credentials').read_text(), 'fixture-only credentials')
        agent = plistlib.loads((self.home / 'Library/LaunchAgents/com.opensurge.control.plist').read_bytes())
        self.assertEqual(agent['ProgramArguments'][:3],
                         [str(self.user / 'bin/opensurge-control'), '--config', str(self.system / 'config.yaml')])
        self.assertTrue((self.fs / 'Applications/OpenSurge.app').is_dir())
        self.assertFalse((self.fs / 'Applications/OpenSurge Menu Bar.app').exists())

    def test_first_install_initializes_config(self):
        (self.system / 'config.yaml').unlink()
        self.run_script('postinstall')
        self.assertEqual((self.system / 'config.yaml').read_text(), 'fixture: first-install\n')

    def test_running_gateway_blocks_uninstall(self):
        calls = self.run_script('uninstall-gui.sh', '--remove-all', expected=2, FIXTURE_GATEWAY='running')
        self.assertFalse(any(c[0] in ('launchctl', 'pmset', 'pkgutil') for c in calls))
        self.assertTrue((self.fs / 'Applications/OpenSurge.app').is_dir())
        self.assertTrue((self.system / 'config.yaml').exists())

    def test_keep_data_removes_programs_and_retains_reinstall_data(self):
        self.write(self.user / 'recovery.json', '{"stage":"prepared"}')
        calls = self.run_script('uninstall-gui.sh', '--keep-data')
        for path in (self.system / 'config.yaml', self.system / 'data/policies.json',
                     self.system / 'runtime/history', self.user / 'credentials', self.user / 'recovery.json',
                     self.fs / 'Library/Logs/OpenSurge/history.log'):
            self.assertTrue(path.exists(), str(path))
        for path in (self.system / 'bin', self.system / 'share', self.user / 'bin',
                     self.user / 'control-endpoint.json', self.fs / 'Applications/OpenSurge.app',
                     self.fs / 'Applications/OpenSurge Menu Bar.app',
                     self.fs / 'Library/PrivilegedHelperTools/com.opensurge.helper'):
            self.assertFalse(path.exists(), str(path))
        self.assertIn(['pkgutil', '--forget', 'com.opensurge.installer'], calls)
        self.assertFalse(any(c[0] == 'kill' for c in calls))

    def test_remove_all_removes_both_data_roots_and_logs(self):
        self.run_script('uninstall-gui.sh', '--remove-all')
        for path in (self.system, self.user, self.fs / 'Library/Logs/OpenSurge', self.fs / 'var/run/opensurge'):
            self.assertFalse(path.exists(), str(path))

    def test_builder_refuses_to_overwrite_an_existing_package(self):
        package = self.root / 'previous.pkg'
        package.write_bytes(b'previous artifact')
        result = subprocess.run(['/bin/bash', str(REPO / 'scripts/build-gui-installer.sh')],
                                env=dict(self.env, OPENSURGE_PKG_OUTPUT=str(package)),
                                text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=5)
        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertIn('package already exists', result.stdout)
        self.assertEqual(package.read_bytes(), b'previous artifact')


if __name__ == '__main__':
    unittest.main(verbosity=2)
