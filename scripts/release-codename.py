#!/usr/bin/env python3
"""Resolve a release's display codename from the shared series catalog."""
import json
from pathlib import Path
import re
import sys

if len(sys.argv) != 2:
    raise SystemExit("usage: release-codename.py RELEASE_TAG")

catalog = json.loads((Path(__file__).resolve().parent.parent / "packaging/release-codenames.json").read_text())
match = re.fullmatch(r"v?(\d+\.\d+)\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?", sys.argv[1])
print(catalog.get(match[1], "") if match else "")
