import assert from 'node:assert/strict';
import {
  mkdtemp,
  mkdir,
  readdir,
  readFile,
  rm,
  writeFile,
} from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { afterEach, test } from 'node:test';
import { fileURLToPath } from 'node:url';

import {
  buildSvgSprite,
  compileSvgSymbol,
  createSvgIconsPlugin,
  createSvgSymbolId,
} from '../build/svg-icons-plugin.mjs';

const webRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '..',
);
const iconDir = path.join(webRoot, 'src/icons/svg');
const temporaryDirectories = [];

afterEach(async () => {
  await Promise.all(
    temporaryDirectories
      .splice(0)
      .map((directory) => rm(directory, { force: true, recursive: true })),
  );
});

const listSourceIcons = async (directory) => {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];

  for (const entry of entries) {
    const absolutePath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await listSourceIcons(absolutePath)));
    } else if (entry.isFile() && entry.name.toLowerCase().endsWith('.svg')) {
      files.push(absolutePath);
    }
  }

  return files.sort((left, right) => left.localeCompare(right, 'en'));
};

const extractSymbols = (sprite) =>
  [
    ...sprite.matchAll(/<symbol\b[^>]*\bid="([^"]+)"[^>]*>[\s\S]*?<\/symbol>/g),
  ].map(([markup, id]) => ({ id, markup }));

const extractIds = (markup) =>
  new Set([...markup.matchAll(/\sid="([^"]+)"/g)].map((match) => match[1]));

const extractReferences = (markup) => [
  ...[...markup.matchAll(/url\(#([^)]+)\)/g)].map((match) => match[1]),
  ...[...markup.matchAll(/(?:xlink:)?href="#([^"]+)"/g)].map(
    (match) => match[1],
  ),
];

test('the repository icon catalog compiles completely and deterministically', async () => {
  const sourceFiles = await listSourceIcons(iconDir);
  const expectedIds = sourceFiles.map((file) =>
    createSvgSymbolId(path.relative(iconDir, file)),
  );
  const first = await buildSvgSprite(iconDir);
  const second = await buildSvgSprite(iconDir);
  const symbols = extractSymbols(first.sprite);

  assert.equal(sourceFiles.length, 210);
  assert.deepEqual(first.files, sourceFiles);
  assert.deepEqual(first.symbolIds, expectedIds);
  assert.equal(new Set(first.symbolIds).size, sourceFiles.length);
  assert.equal(symbols.length, sourceFiles.length);
  assert.equal(
    symbols.every(({ markup }) => /\bviewBox="/.test(markup)),
    true,
  );
  assert.equal(first.sprite, second.sprite);
  assert.equal(first.symbolIds.includes('icon-resource-pool'), true);
  assert.equal(first.symbolIds.includes('icon-menu/resource-pool'), true);
  assert.equal(first.symbolIds.includes('icon-ACTIVE copy'), true);

  const globalIds = new Set();
  for (const { id, markup } of symbols) {
    const localIds = extractIds(markup);
    for (const localId of localIds) {
      assert.equal(
        globalIds.has(localId),
        false,
        `duplicate SVG ID: ${localId}`,
      );
      globalIds.add(localId);
    }
    for (const reference of extractReferences(markup)) {
      assert.equal(
        localIds.has(reference),
        true,
        `${id} has an unresolved internal reference to ${reference}`,
      );
    }
  }

  const vastLogo = symbols.find(({ id }) => id === 'icon-logo-vast')?.markup;
  assert.match(vastLogo, /<pattern\b/);
  assert.match(vastLogo, /xlink:href="#icon-logo-vast_[^"]+"/);
  assert.match(vastLogo, /xlink:href="data:image\/png;base64,/);

  const alarmHistory = symbols.find(
    ({ id }) => id === 'icon-alarm-history',
  )?.markup;
  assert.match(alarmHistory, /<linearGradient\b/);
  assert.match(alarmHistory, /fill="url\(#icon-alarm-history_[^)]+\)"/);
});

test('symbol compilation preserves the existing rendering contract and removes scripts', async () => {
  const missingViewBoxFile = path.join(iconDir, '404.svg');
  const missingViewBox = compileSvgSymbol(
    await readFile(missingViewBoxFile, 'utf8'),
    missingViewBoxFile,
    'icon-404',
  );
  assert.match(missingViewBox, /^<symbol\b/);
  assert.match(missingViewBox, /viewBox="0 0 128 128"/);
  assert.doesNotMatch(missingViewBox, /\b(?:width|height)="/);

  const currentColorFile = path.join(iconDir, 'home-avatar.svg');
  const currentColor = compileSvgSymbol(
    await readFile(currentColorFile, 'utf8'),
    currentColorFile,
    'icon-home-avatar',
  );
  assert.match(currentColor, /stroke="currentColor"/);

  const unsafe = compileSvgSymbol(
    '<svg width="10" height="20" onload="alert(1)"><script>alert(1)</script><a href="javascript:alert(2)"><path onclick="alert(3)" d="M0 0h1v1z"/></a></svg>',
    'unsafe.svg',
    'icon-unsafe',
  );
  assert.match(unsafe, /viewBox="0 0 10 20"/);
  assert.match(unsafe, /<path\b/);
  assert.doesNotMatch(unsafe, /script|onload|onclick|javascript:/i);
});

test('sanitized internal ID prefixes cannot collide silently', async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), 'hami-svg-icons-'));
  temporaryDirectories.push(directory);
  const firstDirectory = path.join(directory, 'space name');
  const secondDirectory = path.join(directory, 'space_name');
  await mkdir(firstDirectory);
  await mkdir(secondDirectory);
  const source =
    '<svg viewBox="0 0 1 1"><defs><linearGradient id="paint"><stop/></linearGradient></defs><path fill="url(#paint)" d="M0 0h1v1z"/></svg>';
  const firstFile = path.join(firstDirectory, 'icon.svg');
  const secondFile = path.join(secondDirectory, 'icon.svg');
  await writeFile(firstFile, source);
  await writeFile(secondFile, source);

  await assert.rejects(
    buildSvgSprite(directory),
    (error) =>
      error.message.includes('Duplicate internal SVG ID') &&
      error.message.includes(firstFile) &&
      error.message.includes(secondFile),
  );
});

test('the Vite plugin registers every source file and reloads SVG changes', async () => {
  const plugin = createSvgIconsPlugin({ iconDir });
  const resolvedId = plugin.resolveId('virtual:svg-icons-register');
  const watchedFiles = [];
  const moduleCode = await plugin.load.call(
    { addWatchFile: (file) => watchedFiles.push(file) },
    resolvedId,
  );

  assert.equal(plugin.resolveId('unrelated-module'), null);
  assert.equal(watchedFiles.length, 210);
  assert.match(moduleCode, /__svg__icons__dom__/);
  assert.match(moduleCode, /<symbol[^>]+id=\\?"icon-more\\?"/);

  const listeners = new Map();
  const invalidatedModules = [];
  const messages = [];
  const virtualModule = { id: resolvedId };
  const watchedDirectories = [];
  plugin.configureServer({
    moduleGraph: {
      getModuleById: (id) => (id === resolvedId ? virtualModule : undefined),
      invalidateModule: (module) => invalidatedModules.push(module),
    },
    watcher: {
      add: (directory) => watchedDirectories.push(directory),
      on: (event, handler) => listeners.set(event, handler),
    },
    ws: { send: (message) => messages.push(message) },
  });

  assert.deepEqual(watchedDirectories, [iconDir]);
  for (const event of ['add', 'change', 'unlink']) {
    listeners.get(event)(path.join(iconDir, `${event}.svg`));
  }
  listeners.get('change')(path.join(iconDir, 'not-an-icon.txt'));
  listeners.get('change')(path.join(path.dirname(iconDir), 'outside.svg'));

  assert.deepEqual(invalidatedModules, [
    virtualModule,
    virtualModule,
    virtualModule,
  ]);
  assert.deepEqual(messages, [
    { type: 'full-reload' },
    { type: 'full-reload' },
    { type: 'full-reload' },
  ]);
});
