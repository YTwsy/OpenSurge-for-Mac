#!/usr/bin/env python3
"""Exercise network-helper recovery without root or host network changes."""
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parent / 'host/omg-lab-network'
FAKE_COMMAND = r'''
import json, os, pathlib, socket, sys
root = pathlib.Path(os.environ['LAB_HELPER_FIXTURE'])
command = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
state = json.loads((root / 'fixture.json').read_text())
with (root / 'calls.log').open('a') as log:
    log.write(command + ' ' + ' '.join(args) + '\n')
if command == 'ps':
    print('/opt/socket_vmnet/bin/socket_vmnet --fixture')
elif command == 'ifconfig':
    if args == ['-l']:
        print('bridge202' if state['bridge'] else '')
    elif len(args) == 1 and args[0] == 'bridge202':
        print('bridge202: flags=UP\n    member: vmenet0\n    inet 192.168.50.1 netmask 0xfffffc00')
    elif not (args[:2] == ['bridge202', 'inet'] and state['bridge']):
        sys.exit(1)
elif command == 'launchctl' and args[0] == 'bootstrap':
    state['bridge'] = True
    (root / 'fixture.json').write_text(json.dumps(state))
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
    sock.bind(str(root / 'network.sock'))
    sock.close()
    (root / 'network.pid').write_text('4242\n')
'''


class NetworkHelperRecovery(unittest.TestCase):
    def run_start(self, bridge):
        with tempfile.TemporaryDirectory(prefix='opensurge-lab-helper-') as directory:
            root = Path(directory)
            (root / 'fixture.json').write_text(json.dumps({'bridge': bridge}))
            (root / 'network.pid').write_text('4242\n')
            (root / 'launch.plist').write_text('fixture\n')
            sock = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
            self.addCleanup(sock.close)
            sock.bind(str(root / 'network.sock'))
            script = SOURCE.read_text()
            replacements = {
                '/private/var/run/open-mihomo-gateway-lab.sock': root / 'network.sock',
                '/private/var/run/open-mihomo-gateway-lab.pid': root / 'network.pid',
                '/private/var/run/open-mihomo-gateway-lab.interface': root / 'network.interface',
                '/private/var/log/open-mihomo-gateway-lab-vmnet.log': root / 'network.log',
                '/Library/LaunchDaemons/io.opensurge.lab.socket-vmnet.plist': root / 'launch.plist',
            }
            for original, replacement in replacements.items():
                script = script.replace(original, str(replacement))
            for command, original in [('ps', '/bin/ps'), ('kill', '/bin/kill'),
                                      ('sleep', '/bin/sleep'), ('ifconfig', '/sbin/ifconfig'),
                                      ('launchctl', '/bin/launchctl')]:
                fake = root / command
                fake.write_text('#!' + sys.executable + '\n' + FAKE_COMMAND)
                fake.chmod(0o755)
                script = script.replace(original, str(fake))
            helper = root / 'helper'
            helper.write_text(script)
            result = subprocess.run(['/bin/sh', str(helper), 'start'],
                                    env=dict(os.environ, LAB_HELPER_FIXTURE=str(root), LC_ALL='C'),
                                    capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertTrue((root / 'network.interface').is_file(), result.stdout + result.stderr)
            self.assertEqual((root / 'network.interface').read_text(), 'bridge202\n')
            self.assertIn('interface=bridge202', result.stdout)
            return (root / 'calls.log').read_text(), result.stderr

    def test_running_socket_without_bridge_recreates_network(self):
        calls, error = self.run_start(bridge=False)
        self.assertIn('launchctl bootout system/io.opensurge.lab.socket-vmnet', calls)
        self.assertIn('launchctl bootstrap system ', calls)
        self.assertIn('recreating the lab network', error)

    def test_healthy_bridge_is_reused_and_interface_record_repaired(self):
        calls, error = self.run_start(bridge=True)
        self.assertNotIn('launchctl ', calls)
        self.assertIn('ifconfig bridge202 inet 192.168.50.1 netmask 255.255.252.0 up', calls)
        self.assertEqual(error, '')


if __name__ == '__main__':
    unittest.main(verbosity=2)
