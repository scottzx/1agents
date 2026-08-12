import {createHash} from 'node:crypto';
import {cp, mkdir, readFile, rm, writeFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import path from 'node:path';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const source = path.join(root, 'src');
const output = path.join(root, 'dist');
const siteBase = (process.env.SITE_BASE || '/1agents/').replace(/\/?$/, '/');

await rm(output, {recursive: true, force: true});
await mkdir(output, {recursive: true});
await cp(source, output, {recursive: true});
await writeFile(path.join(output, '.nojekyll'), '');

const cssPath = path.join(output, 'styles.css');
const jsPath = path.join(output, 'script.js');
const htmlPath = path.join(output, 'index.html');

const [css, js, rawHtml] = await Promise.all([
  readFile(cssPath),
  readFile(jsPath),
  readFile(htmlPath, 'utf8'),
]);

const cssHash = createHash('sha1').update(css).digest('hex').slice(0, 8);
const jsHash = createHash('sha1').update(js).digest('hex').slice(0, 8);

let html = rawHtml
  .replaceAll('href="/1agents/styles.css"', `href="${siteBase}styles.css?v=${cssHash}"`)
  .replaceAll('href="./styles.css"', `href="${siteBase}styles.css?v=${cssHash}"`)
  .replaceAll('src="/1agents/script.js"', `src="${siteBase}script.js?v=${jsHash}"`)
  .replaceAll('src="./script.js"', `src="${siteBase}script.js?v=${jsHash}"`);

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

if (!html.includes(`${siteBase}styles.css?v=${cssHash}`)) {
  throw new Error('Built HTML is missing versioned stylesheet path');
}

// Guard against common desktop layout regressions in CSS
const cssText = css.toString('utf8');
for (const token of [
  'grid-template-columns: minmax(0, 1.2fr) minmax(420px, 0.95fr)',
  '.site-nav {',
  '@media (max-width: 720px)',
]) {
  if (!cssText.includes(token)) {
    throw new Error(`Missing required CSS layout token: ${token}`);
  }
}

await writeFile(htmlPath, html);
console.log(`Built static site at ${output}`);
console.log(`Base: ${siteBase}`);
console.log(`CSS: ${siteBase}styles.css?v=${cssHash}`);
console.log(`JS:  ${siteBase}script.js?v=${jsHash}`);
