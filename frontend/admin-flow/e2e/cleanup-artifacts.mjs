import { rm } from 'node:fs/promises';
import { resolve } from 'node:path';

const root = resolve(new URL('../../..', import.meta.url).pathname);
const targets = [
  resolve(root, 'test-results/.last-run.json'),
];

for (const target of targets) {
  await rm(target, { force: true });
  console.log(`[cleanup] removed ${target}`);
}
