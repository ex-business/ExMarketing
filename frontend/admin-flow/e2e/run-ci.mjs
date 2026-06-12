import { spawn } from 'node:child_process';
import { rm } from 'node:fs/promises';
import { resolve } from 'node:path';

const root = resolve(new URL('../../..', import.meta.url).pathname);
const npmCommand = process.platform === 'win32' ? 'npx.cmd' : 'npx';

function run(command, args) {
  return new Promise((resolveExit) => {
    const child = spawn(command, args, {
      cwd: root,
      stdio: 'inherit',
      shell: false,
    });
    child.on('close', (code, signal) => {
      if (signal) resolveExit(1);
      else resolveExit(code ?? 1);
    });
    child.on('error', (error) => {
      console.error(error);
      resolveExit(1);
    });
  });
}

const exitCode = await run(npmCommand, ['playwright', 'test', '--config', 'frontend/admin-flow/playwright.config.ts']);

await rm(resolve(root, 'test-results/.last-run.json'), { force: true });
console.log('[cleanup] removed test-results/.last-run.json');

process.exit(exitCode);
