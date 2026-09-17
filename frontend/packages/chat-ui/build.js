import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { execSync } from 'node:child_process';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const esbuild = require('esbuild');

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const distDir = path.resolve(__dirname, 'dist');

if (!fs.existsSync(distDir)) {
  fs.mkdirSync(distDir, { recursive: true });
}

// 1. Copy style.css to dist/
const srcCssPath = path.resolve(__dirname, 'src/style.css');
const distCssPath = path.resolve(distDir, 'style.css');
const cssContent = fs.readFileSync(srcCssPath, 'utf8');
fs.writeFileSync(distCssPath, cssContent, 'utf8');

// 2. Build ESM
await esbuild.build({
  entryPoints: [path.resolve(__dirname, 'src/index.ts')],
  outfile: path.resolve(distDir, 'index.js'),
  bundle: true,
  format: 'esm',
  platform: 'browser',
  target: ['es2022'],
  external: ['preact', 'preact/hooks'],
  loader: { '.tsx': 'tsx', '.ts': 'ts' },
  define: {
    'process.env.NODE_ENV': '"production"',
    '__CHAT_CSS__': JSON.stringify(cssContent),
  },
});

// 3. Build CJS
await esbuild.build({
  entryPoints: [path.resolve(__dirname, 'src/index.ts')],
  outfile: path.resolve(distDir, 'index.cjs'),
  bundle: true,
  format: 'cjs',
  platform: 'neutral',
  target: ['es2022'],
  external: ['preact', 'preact/hooks'],
  loader: { '.tsx': 'tsx', '.ts': 'ts' },
  define: {
    'process.env.NODE_ENV': '"production"',
    '__CHAT_CSS__': JSON.stringify(cssContent),
  },
});

// 4. Generate TypeScript declarations
try {
  console.log('Generating type declarations via tsc...');
  execSync('npx tsc -p tsconfig.json', { cwd: __dirname, stdio: 'inherit' });
} catch (e) {
  console.warn('tsc declaration notice:', e.message);
}

console.log('Successfully built @1agents/chat-ui to dist/!');
