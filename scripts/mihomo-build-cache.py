#!/usr/bin/env python3
"""Local build cache, keyed by inputs and verified against the executable digest.

This is provenance for a trusted local cache, not a signature or an attestation.
The shell builder verifies the pinned archive before invoking this helper.
"""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile


def digest(path):
    checksum = hashlib.sha256()
    with open(path, 'rb') as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b''):
            checksum.update(chunk)
    return checksum.hexdigest()


def encoded(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':')).encode()


def describe(args):
    root = Path(args.root)
    sources = [root / 'scripts/build-opensurge-mihomo.sh',
               root / 'scripts/mihomo-build-cache.py',
               *sorted((root / 'patches/mihomo').rglob('*'))]
    environment = dict(os.environ, GOOS='darwin', GOARCH=args.arch, CGO_ENABLED='0')
    go_env = json.loads(subprocess.check_output([
        args.go, 'env', '-json', 'GOVERSION', 'GOOS', 'GOARCH', 'CGO_ENABLED',
        'GOAMD64', 'GOARM64', 'GOEXPERIMENT', 'GOFLAGS', 'GOTOOLCHAIN', 'GOFIPS140',
    ], env=environment))
    # These flags reference arbitrary external files/programs. Their string
    # values alone cannot establish cache validity; build without caching them.
    cacheable = not any(flag in go_env.get('GOFLAGS', '')
                        for flag in ('-overlay', '-modfile', '-toolexec'))
    print(encoded({
        'schema': 1, 'cacheable': cacheable, 'source_sha256': digest(args.source),
        'files': {str(p.relative_to(root)): digest(p) for p in sources if p.is_file()},
        'go_binary_sha256': digest(shutil.which(args.go) or args.go),
        'go_env': go_env, 'version': args.version, 'GODEBUG': os.environ.get('GODEBUG', ''),
        # Fixed tags/ldflags and test commands are covered by the builder hash.
    }).decode())


def atomic_copy(source, destination, expected):
    destination = Path(destination)
    destination.parent.mkdir(parents=True, exist_ok=True)
    fd, name = tempfile.mkstemp(prefix='.mihomo-', dir=destination.parent)
    try:
        with os.fdopen(fd, 'wb') as target, open(source, 'rb') as data:
            shutil.copyfileobj(data, target)
        if digest(name) != expected:
            raise ValueError('mihomo changed while copying; refusing to publish it')
        os.chmod(name, 0o755)
        os.replace(name, destination)
    finally:
        if os.path.exists(name):
            os.unlink(name)


def cache(args):
    inputs = json.loads(Path(args.manifest).read_text())
    if not inputs['cacheable']:
        if args.command == 'restore':
            return 1
        atomic_copy(args.binary, args.output, digest(args.binary))
        print('Built without caching: GOFLAGS references external build inputs')
        return 0
    key = hashlib.sha256(encoded(inputs)).hexdigest()
    directory = Path(args.cache) / key
    directory.mkdir(parents=True, exist_ok=True)
    binary, manifest = directory / 'mihomo', directory / 'manifest.json'
    # Readers and publishers see a consistent binary/manifest pair. Concurrent
    # misses may compile independently, but cannot publish a torn entry.
    with open(directory / '.lock', 'a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        if args.command == 'restore':
            try:
                record = json.loads(manifest.read_text())
                if (record['inputs'] != inputs or not os.access(binary, os.X_OK)
                        or digest(binary) != record['binary_sha256']):
                    return 1
            except (OSError, ValueError, KeyError, TypeError):
                return 1
            atomic_copy(binary, args.output, record['binary_sha256'])
            print(f'Reused verified mihomo cache {key[:12]}: {args.output}')
        else:
            checksum = digest(args.binary)
            atomic_copy(args.binary, binary, checksum)
            fd, name = tempfile.mkstemp(prefix='.manifest-', dir=directory)
            try:
                with os.fdopen(fd, 'wb') as output:
                    output.write(encoded({'inputs': inputs, 'binary_sha256': checksum}))
                os.replace(name, manifest)
            finally:
                if os.path.exists(name):
                    os.unlink(name)
            atomic_copy(binary, args.output, checksum)
            print(f'Stored verified mihomo cache {key[:12]}: {args.output}')
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    description = commands.add_parser('describe')
    for name in ('root', 'source', 'go', 'arch', 'version'):
        description.add_argument('--' + name, required=True)
    for command in ('restore', 'publish'):
        sub = commands.add_parser(command)
        for name in ('manifest', 'cache', 'output'):
            sub.add_argument('--' + name, required=True)
        if command == 'publish':
            sub.add_argument('--binary', required=True)
    args = parser.parse_args()
    if args.command == 'describe':
        describe(args)
        return 0
    return cache(args)


if __name__ == '__main__':
    raise SystemExit(main())
