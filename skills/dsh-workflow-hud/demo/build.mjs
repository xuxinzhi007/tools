/**
 * Single-file build for the cyber web-crawler demo.
 *
 *   node build.mjs          # writes demo/index.html
 *   node build.mjs --check  # verifies the emitted file without writing
 *
 * The output has no network dependency: CSS, core, scene, and the mascot
 * artwork are inlined, so the file opens directly from `file://`.
 */
import { readFile, writeFile, stat } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..');

function stripModuleSyntax(source) {
  return source
    .replace(/^export\s+(?=(?:const|let|var|function|class)\b)/gm, '')
    .replace(/^export\s*\{[^}]*\}\s*;?\s*$/gm, '')
    .trim();
}

function assertSafe(source, label) {
  if (/<\/script/i.test(source)) {
    throw new Error(`${label} contains a closing script tag; it cannot be inlined`);
  }
  if (/\bimport\s+[^('"]/.test(source)) {
    throw new Error(`${label} still contains an import statement`);
  }
}

/**
 * Build the single HTML document.
 *
 * @returns {Promise<{ html: string, outPath: string, bytes: number }>}
 */
export async function buildDemo() {
  const [template, css, coreRaw, scene, asset, pkgText] = await Promise.all([
    readFile(path.join(here, 'index.template.html'), 'utf8'),
    readFile(path.join(here, 'styles.css'), 'utf8'),
    readFile(path.join(here, 'src', 'core.js'), 'utf8'),
    readFile(path.join(here, 'src', 'scene.js'), 'utf8'),
    readFile(path.join(here, 'assets', 'pet-idle.png')),
    readFile(path.join(root, 'package.json'), 'utf8'),
  ]);

  const pkg = JSON.parse(pkgText);
  const core = stripModuleSyntax(coreRaw);
  assertSafe(css, 'styles.css');
  assertSafe(core, 'src/core.js');
  assertSafe(scene, 'src/scene.js');

  const dataUrl = `data:image/png;base64,${asset.toString('base64')}`;
  const stamp = new Date().toISOString();

  let html = template;
  const replace = (needle, value) => {
    if (!html.includes(needle)) throw new Error(`template placeholder missing: ${needle}`);
    html = html.split(needle).join(value);
  };

  replace('/*__CSS__*/', css);
  replace('/*__CORE__*/', core);
  replace('/*__SCENE__*/', scene);
  replace('__VERSION__', `${pkg.version}-demo`);
  replace('__BUILT_AT__', stamp);
  replace('__PET_DATA_URL__', dataUrl);

  const leftovers = html.match(/__(?:CSS|CORE|SCENE|VERSION|PET_DATA_URL)__/g);
  if (leftovers) throw new Error(`unreplaced placeholders: ${leftovers.join(', ')}`);

  const scripts = html.match(/<\/script>/gi) || [];
  if (scripts.length !== 1) {
    throw new Error(`expected exactly one closing script tag, found ${scripts.length}`);
  }

  const outPath = path.join(here, 'index.html');
  return { html, outPath, bytes: Buffer.byteLength(html, 'utf8') };
}

export async function writeDemo() {
  const result = await buildDemo();
  await writeFile(result.outPath, result.html, 'utf8');
  const info = await stat(result.outPath);
  return { ...result, bytes: info.size };
}

const invokedDirectly =
  process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url;

if (invokedDirectly) {
  const checkOnly = process.argv.includes('--check');
  const result = checkOnly ? await buildDemo() : await writeDemo();
  const kb = (result.bytes / 1024).toFixed(1);
  if (checkOnly) {
    console.log(`index.html check ok · would be ${kb} KB`);
  } else {
    console.log(`index.html written · ${kb} KB · ${result.outPath}`);
  }
}
