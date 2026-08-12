import {cp, mkdir, readFile, rm, writeFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import path from 'node:path';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const source = path.join(root, 'src');
const output = path.join(root, 'dist');

await rm(output, {recursive: true, force: true});
await mkdir(output, {recursive: true});
await cp(source, output, {recursive: true});
await writeFile(path.join(output, '.nojekyll'), '');

const html = await readFile(path.join(output, 'index.html'), 'utf8');
const requiredCopy = [
  '1Agents',
  'VoiceContext',
  '54',
  'Amazon Bedrock AgentCore',
  '创始人自用验证',
];

for (const copy of requiredCopy) {
  if (!html.includes(copy)) {
    throw new Error(`Missing required site copy: ${copy}`);
  }
}

console.log(`Built static site at ${output}`);
