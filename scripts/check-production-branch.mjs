import { spawnSync } from 'node:child_process';

function run(args) {
  const result = spawnSync('git', args, { encoding: 'utf8', windowsHide: true });
  if (result.status !== 0) {
    process.stderr.write(result.stderr || `git ${args.join(' ')} failed\n`);
    process.exit(result.status || 1);
  }
  return result.stdout.trim();
}

const branch = run(['symbolic-ref', '--short', '-q', 'HEAD']);
if (branch !== 'main') {
  throw new Error(`Production deployment must be built from main; current branch is ${branch || 'detached HEAD'}`);
}

const dirty = run(['status', '--porcelain']);
if (dirty) {
  throw new Error('Production deployment requires a clean working tree; commit or discard local changes first');
}

const commit = run(['rev-parse', 'HEAD']);
const remote = run(['rev-parse', 'origin/main']);
if (commit !== remote) {
  throw new Error(`Local main (${commit}) is not equal to origin/main (${remote}); pull/merge and push main first`);
}

console.log(`Production branch verified: main @ ${commit}`);
