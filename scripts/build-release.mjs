// Build a Linux release locally; never package development configuration or data.
import { spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, cpSync, readFileSync, writeFileSync, readdirSync } from 'node:fs';
import { createHash } from 'node:crypto';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));
process.chdir(root);
const version = process.argv[2];
if (!version || !/^v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$/.test(version)) {
  throw new Error('Usage: node scripts/build-release.mjs v0.1.0[-suffix]');
}
const output = path.join(root, '.local/releases', version);
if (existsSync(output)) throw new Error('Release directory already exists; use a new version suffix');
const git = command => {
  const result = spawnSync('git', command, { cwd: root, encoding: 'utf8', windowsHide: true });
  if (result.status !== 0) throw new Error(`git ${command.join(' ')} failed (${result.status})`);
  return result.stdout.trim();
};
const sourceBranch = git(['symbolic-ref', '--short', '-q', 'HEAD']) || 'DETACHED';
const sourceCommit = git(['rev-parse', 'HEAD']);
const sourceDirty = git(['status', '--porcelain']).length > 0;
mkdirSync(path.join(output, 'bin'), { recursive: true });
const localGo = path.join(root, '.tools/go/bin', process.platform === 'win32' ? 'go.exe' : 'go');
const go = existsSync(localGo) ? localGo : 'go';
function run(command, args, options = {}) {
  const result = spawnSync(command, args, { stdio: 'inherit', windowsHide: true, ...options });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${command} failed (${result.status})`);
}
for (const name of ['server', 'migrate', 'jobs-migrate', 'access-admin', 'sde-import']) {
  run(go, ['build', '-trimpath', '-ldflags', `-s -w -X main.version=${version}`,
    '-o', path.join(output, 'bin', name), `./cmd/${name}`],
  { env: { ...process.env, GOOS: 'linux', GOARCH: 'amd64', CGO_ENABLED: '0' } });
}
const npmCLI = process.env.npm_execpath || path.join(path.dirname(process.execPath),
  process.platform === 'win32' ? 'node_modules/npm/bin/npm-cli.js' : '../lib/node_modules/npm/bin/npm-cli.js');
run(process.execPath, [npmCLI, '--prefix', 'web', 'run', 'build']);
cpSync(path.join(root, 'web/dist'), path.join(output, 'web'), { recursive: true });
cpSync(path.join(root, 'deploy'), path.join(output, 'deploy'), { recursive: true });
for (const name of ['CHANGELOG.md', 'docs/third-party-notices.md']) {
  cpSync(path.join(root, name), path.join(output, path.basename(name)));
}
writeFileSync(path.join(output, 'release.json'), JSON.stringify({ version, platform: 'linux/amd64',
  source_branch: sourceBranch, source_commit: sourceCommit, source_dirty: sourceDirty,
  built_at: new Date().toISOString() }, null, 2) + '\n');
function files(directory, prefix = '') {
  return readdirSync(directory, { withFileTypes: true }).flatMap(entry => {
    const relative = prefix + entry.name;
    return entry.isDirectory() ? files(path.join(directory, entry.name), relative + '/') : [relative];
  });
}
writeFileSync(path.join(output, 'SHA256SUMS'), files(output).sort().map(name =>
  `${createHash('sha256').update(readFileSync(path.join(output, name))).digest('hex')}  ${name}\n`).join(''));
run('tar', ['-czf', output + '.tar.gz', '-C', output, '.']);
console.log(`Release: ${output}.tar.gz`);
