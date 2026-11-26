import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const webRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));

const ALLOWED_LEGACY_EL_TAG_FILES = new Set([
  'projects/vgpu/components/DetailPageState.vue',
  'projects/vgpu/components/EnvironmentSources.vue',
  'projects/vgpu/components/VersionTag.vue',
  'projects/vgpu/components/timeSelect.vue',
  'projects/vgpu/views/task/admin/Detail.vue',
  'src/App.vue',
  'src/components/EllipsisText.vue',
  'src/components/LangSelect/index.vue',
  'src/components/PageHeader.vue',
  'src/components/TablePlus/StatefulTable.vue',
  'src/layout/index.vue',
]);

const collectVueFiles = (dir, out = []) => {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name);
    if (entry.isDirectory()) {
      collectVueFiles(fullPath, out);
    } else if (entry.isFile() && entry.name.endsWith('.vue')) {
      out.push(fullPath);
    }
  }
  return out;
};

test('no new Vue files introduce Element Plus <el-*>, <El*>, or <el*> template tags', () => {
  const vueFiles = [
    ...collectVueFiles(join(webRoot, 'src')),
    ...collectVueFiles(join(webRoot, 'projects')),
  ];
  const violations = [];
  for (const filePath of vueFiles) {
    const rel = relative(webRoot, filePath).replace(/\\/g, '/');
    const content = readFileSync(filePath, 'utf8');
    if ((/<el-[a-z-]+/i.test(content) || /<[Ee]l[A-Z][A-Za-z0-9]*/.test(content)) && !ALLOWED_LEGACY_EL_TAG_FILES.has(rel)) {
      violations.push(rel);
    }
  }
  assert.deepEqual(
    violations,
    [],
    `Unexpected <el-*>, <El*>, or <el*> usage in new or un-exempted Vue files: ${violations.join(', ')}`,
  );
});
