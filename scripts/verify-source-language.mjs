import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const CJK_SOURCE_CHAR =
  /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Hangul}\p{Script=Bopomofo}]/u;

const FIRST_PARTY_PREFIXES = [
  '.github/',
  'charts/',
  'packages/web/',
  'scripts/',
  'server/',
];

const ROOT_SOURCE_FILES = new Set([
  '.browserslistrc',
  '.dockerignore',
  '.eslintignore',
  '.eslintrc.js',
  '.node-version',
  '.prettierrc',
  'Dockerfile',
  'Makefile',
  'package.json',
  'pnpm-workspace.yaml',
]);

const TEXT_EXTENSIONS = new Set([
  '.cjs',
  '.css',
  '.go',
  '.html',
  '.js',
  '.json',
  '.jsx',
  '.mjs',
  '.mod',
  '.proto',
  '.scss',
  '.sh',
  '.toml',
  '.tpl',
  '.ts',
  '.tsx',
  '.txt',
  '.vue',
  '.yaml',
  '.yml',
]);

const TEXT_BASENAMES = new Set([
  '.browserslistrc',
  '.dockerignore',
  '.env.development',
  '.env.production',
  '.eslintignore',
  '.gitignore',
  '.go-version',
  '.helmignore',
  '.node-version',
  '.prettierrc',
  'Dockerfile',
  'Makefile',
]);

const EXCLUDED_FILES = new Set([
  'packages/web/src/locales/en.js',
  'packages/web/src/locales/zh.js',
]);

const EXCLUDED_PREFIXES = [
  'packages/web/public/',
  'packages/web/src/assets/',
  'packages/web/src/icons/svg/',
  'packages/web/test/',
  'scripts/chart/tests/',
  'scripts/release/tests/',
  'server/third_party/',
  'test/',
];

const isGeneratedSource = (filePath) => {
  const basename = path.posix.basename(filePath);
  return (
    basename === 'wire_gen.go' ||
    basename.startsWith('zz_generated.') ||
    filePath.endsWith('.pb.go') ||
    filePath.endsWith('.pb.gw.go')
  );
};

const isTestFixture = (filePath) => {
  const basename = path.posix.basename(filePath);
  return (
    basename.includes('.test.') ||
    basename.includes('.spec.') ||
    basename.endsWith('_test.go')
  );
};

const isTextSource = (filePath) => {
  const basename = path.posix.basename(filePath);
  return (
    TEXT_BASENAMES.has(basename) || TEXT_EXTENSIONS.has(path.posix.extname(filePath))
  );
};

const shouldScanPath = (filePath) => {
  if (
    EXCLUDED_FILES.has(filePath) ||
    EXCLUDED_PREFIXES.some((prefix) => filePath.startsWith(prefix)) ||
    isGeneratedSource(filePath) ||
    isTestFixture(filePath)
  ) {
    return false;
  }

  const isFirstParty =
    ROOT_SOURCE_FILES.has(filePath) ||
    FIRST_PARTY_PREFIXES.some((prefix) => filePath.startsWith(prefix));
  return isFirstParty && isTextSource(filePath);
};

const assertGuardContract = () => {
  const matchingCodePoints = [0x6c49, 0x20000, 0x3042, 0x30a2, 0xac00, 0x3105];
  for (const codePoint of matchingCodePoints) {
    if (!CJK_SOURCE_CHAR.test(String.fromCodePoint(codePoint))) {
      throw new Error(`CJK source regex must match U+${codePoint.toString(16)}`);
    }
  }

  const nonMatchingCodePoints = [0x41, 0xb7, 0x2022, 0x1f680];
  for (const codePoint of nonMatchingCodePoints) {
    if (CJK_SOURCE_CHAR.test(String.fromCodePoint(codePoint))) {
      throw new Error(`CJK source regex must not match U+${codePoint.toString(16)}`);
    }
  }

  const pathAssertions = [
    ['.github/workflows/pr-frontend.yaml', true],
    ['charts/hami-webui/templates/deployment.yaml', true],
    ['packages/web/src/main.js', true],
    ['packages/web/src/locales/runtime.js', true],
    ['scripts/verify-source-language.mjs', true],
    ['server/internal/app.go', true],
    ['.browserslistrc', true],
    ['.eslintignore', true],
    ['.node-version', true],
    ['.prettierrc', true],
    ['package.json', true],
    ['README_ZH.md', false],
    ['packages/web/src/locales/en.js', false],
    ['packages/web/src/locales/zh.js', false],
    ['packages/web/test/locale-resolution.test.mjs', false],
    ['packages/web/src/example.test.mjs', false],
    ['packages/web/src/assets/example.css', false],
    ['packages/web/src/icons/svg/example.svg', false],
    ['scripts/chart/tests/example.yaml', false],
    ['scripts/release/tests/example.sh', false],
    ['server/internal/app_test.go', false],
    ['server/third_party/example.proto', false],
    ['server/api/v1/example.pb.go', false],
    ['server/cmd/hami-webui/wire_gen.go', false],
    ['pnpm-lock.yaml', false],
  ];

  for (const [filePath, expected] of pathAssertions) {
    if (shouldScanPath(filePath) !== expected) {
      throw new Error(`Unexpected source-language path rule for ${filePath}`);
    }
  }
};

assertGuardContract();

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const trackedFiles = execFileSync('git', ['-C', repoRoot, 'ls-files', '-z'], {
  encoding: 'utf8',
})
  .split('\0')
  .filter(Boolean);
const sourceFiles = trackedFiles.filter(shouldScanPath);
const violations = [];

for (const filePath of sourceFiles) {
  const lines = readFileSync(path.join(repoRoot, filePath), 'utf8').split(/\r?\n/u);
  lines.forEach((line, index) => {
    if (CJK_SOURCE_CHAR.test(line)) {
      violations.push(`${filePath}:${index + 1}: ${line.trim()}`);
    }
  });
}

if (violations.length > 0) {
  console.error('CJK script characters are not allowed in active first-party source:');
  violations.forEach((violation) => console.error(violation));
  process.exit(1);
}

console.log(`Source language check passed (${sourceFiles.length} tracked files scanned).`);
