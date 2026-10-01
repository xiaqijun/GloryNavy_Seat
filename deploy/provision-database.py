#!/usr/bin/env python3
"""Create the app's non-superuser database role on the dedicated instance."""
import importlib.util
from pathlib import Path
import subprocess
from urllib.parse import unquote, urlparse

spec = importlib.util.spec_from_file_location('operator_env', Path(__file__).with_name('run-operator.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
url = urlparse(module.environment()['DATABASE_URL'])
if url.username != 'glorynavy' or url.hostname != '127.0.0.1' or url.port != 55433 or url.path != '/glorynavy':
    raise SystemExit('Expected the dedicated local Glory Navy database')
password = unquote(url.password or '')
if not password or not all(c.isascii() and (c.isalnum() or c in '_-') for c in password):
    raise SystemExit('Expected a generated URL-safe database password')
sql = """DO $block$ BEGIN
IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'glorynavy') THEN
CREATE ROLE glorynavy LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD '%s';
END IF;
END $block$;
ALTER DATABASE glorynavy OWNER TO glorynavy;
""" % password
result = subprocess.run(['docker', 'compose', '-f', '/etc/glorynavy/compose.database.yaml',
    'exec', '-T', 'db', 'psql', '-v', 'ON_ERROR_STOP=1', '-U', 'postgres', '-d', 'glorynavy'],
    input=sql, text=True, capture_output=True)
if result.returncode:
    raise SystemExit('Database role provisioning failed; confidential SQL output suppressed')
print('Dedicated application database role ready')
