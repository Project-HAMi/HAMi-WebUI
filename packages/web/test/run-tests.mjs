import { spawnSync } from 'node:child_process';
import { readdirSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const webRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));

const discoverTests = (dir, out = []) => {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name);
    if (entry.isDirectory()) {
      discoverTests(fullPath, out);
    } else if (entry.isFile() && entry.name.endsWith('.test.mjs')) {
      out.push(relative(webRoot, fullPath).replace(/\\/g, '/'));
    }
  }
  return out;
};

const tests = [
  ...discoverTests(join(webRoot, 'src')),
  ...discoverTests(join(webRoot, 'test')),
].sort();

const result = spawnSync(process.execPath, ['--test', ...tests], {
  cwd: webRoot,
  stdio: 'inherit',
});
if (result.error) {
  console.error(result.error);
  process.exit(1);
}
process.exit(result.status ?? 1);
