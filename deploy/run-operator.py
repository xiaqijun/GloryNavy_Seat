#!/usr/bin/env python3
"""Load the generated systemd environment without executing it as shell code."""
import os
from pathlib import Path
import shlex
import sys

def environment():
    result = dict(os.environ)
    for line in Path('/etc/glorynavy/seat.env').read_text().splitlines():
        line = line.strip()
        if not line or line.startswith('#'):
            continue
        fields = shlex.split(line)
        if len(fields) != 1 or '=' not in fields[0]:
            raise ValueError('Invalid production environment format')
        key, value = fields[0].split('=', 1)
        result[key] = value
    return result

if __name__ == '__main__':
    if len(sys.argv) < 2:
        raise SystemExit('Usage: run-operator.py COMMAND [ARGS]')
    os.execvpe(sys.argv[1], sys.argv[1:], environment())
