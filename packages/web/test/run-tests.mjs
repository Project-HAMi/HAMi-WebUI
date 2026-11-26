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

const promqlTest = 'test/homepage-promql.test.mjs';
const tests = [
  ...discoverTests(join(webRoot, 'src')),
  ...discoverTests(join(webRoot, 'projects')),
  ...discoverTests(join(webRoot, 'test')),
].sort();

const promtool = process.env.PROMTOOL || 'promtool';
const hasPromtool = spawnSync(promtool, ['--version'], { stdio: 'ignore' }).status === 0;
const requirePromtool = process.env.CI === 'true' || process.env.REQUIRE_PROMTOOL === '1';

let selectedTests = tests;
if (!hasPromtool) {
  if (requirePromtool) {
    console.error(`Required promtool binary not found (${promtool}) while running ${promqlTest}`);
    process.exit(1);
  }
  console.log(`[run-tests] promtool unavailable locally; skipping ${promqlTest}`);
  selectedTests = tests.filter((file) => file !== promqlTest);
}

const result = spawnSync(process.execPath, ['--test', ...selectedTests], {
  cwd: webRoot,
  stdio: 'inherit',
});
if (result.error) {
  console.error(result.error);
  process.exit(1);
}
process.exit(result.status ?? 1);
