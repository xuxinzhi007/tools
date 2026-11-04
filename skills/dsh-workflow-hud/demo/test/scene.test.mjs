import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..');

async function readDemoFiles() {
  const [template, scene, css] = await Promise.all([
    readFile(path.join(root, 'index.template.html'), 'utf8'),
    readFile(path.join(root, 'src', 'scene.js'), 'utf8'),
    readFile(path.join(root, 'styles.css'), 'utf8'),
  ]);
  return { template, scene, css };
}

function idsIn(html) {
  return new Set(
    Array.from(html.matchAll(/\bid="([a-z0-9-]+)"/gi), (match) => match[1]),
  );
}

test('every DOM id read by the scene exists in the page shell', async () => {
  const { template, scene } = await readDemoFiles();
  const pageIds = idsIn(template);
  const sceneIds = Array.from(
    new Set(Array.from(scene.matchAll(/\$\('#([a-z0-9-]+)'/g), (match) => match[1])),
  );

  assert.ok(sceneIds.length >= 25, `unexpectedly few ids: ${sceneIds.length}`);
  for (const id of sceneIds) {
    assert.ok(pageIds.has(id), `scene reads #${id}, but the template lacks it`);
  }
});

test('the canvas is decorative and click-through', async () => {
  const { template, css } = await readDemoFiles();
  assert.match(template, /<canvas id="web" aria-hidden="true"><\/canvas>/);
  assert.match(css, /#web\s*\{[^}]*pointer-events:\s*none;/s);
  assert.match(css, /#web\s*\{[^}]*z-index:\s*9;/s);
});

test('controls are real buttons with an explicit type', async () => {
  const { template } = await readDemoFiles();
  for (const id of ['btn-pause', 'btn-replay', 'btn-mode', 'btn-speed']) {
    const pattern = new RegExp(`<button id="${id}" type="button">`);
    assert.match(template, pattern, `${id} must be a typed button`);
  }
});

test('reduced motion and the artwork attribution are present', async () => {
  const { css, template } = await readDemoFiles();
  assert.match(css, /@media\s*\(prefers-reduced-motion:\s*reduce\)/);
  assert.match(css, /\.is-reduced/);
  assert.match(template, /YunYueSama/);
  assert.match(template, /codex-deepseek-pet/);
  assert.match(template, /署名许可/);
});

test('the scene protects the render loop with an error budget', async () => {
  const { scene } = await readDemoFiles();
  assert.match(scene, /if \(errors > 6\)/);
  assert.match(scene, /\[cyber-web\] frame failed/);
  assert.match(scene, /document\.hidden/);
  assert.match(scene, /window\.addEventListener\('resize', resizeCanvas\)/);
});
