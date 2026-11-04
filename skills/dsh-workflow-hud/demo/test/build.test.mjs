import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile, stat } from 'node:fs/promises';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';
import { buildDemo } from '../build.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));

test('the single-file build inlines CSS, core, scene, and the mascot artwork', async () => {
  const { html, bytes } = await buildDemo();

  assert.ok(bytes > 200_000, 'the artwork should make the file substantial');
  assert.match(html, /^<!doctype html>/i);
  assert.match(html, /CYBER WEB CRAWLER/);
  assert.match(html, /<style>/);
  assert.match(html, /data:image\/png;base64,/);
  assert.match(html, /class Activity/);
  assert.match(html, /function initCyberDemo/);
  assert.ok(!html.includes('__CORE__'));
  assert.ok(!html.includes('__SCENE__'));
  assert.ok(!html.includes('__CSS__'));
  assert.ok(!html.includes('__PET_DATA_URL__'));
  assert.ok(!html.includes('__VERSION__'));

  // Inlined modules must not keep ESM syntax or close the host script early.
  const scriptStart = html.indexOf('<script>');
  const scriptEnd = html.lastIndexOf('</script>');
  assert.ok(scriptStart > 0 && scriptEnd > scriptStart);
  const script = html.slice(scriptStart + '<script>'.length, scriptEnd);
  assert.ok(!/^\s*export\s/m.test(script), 'export keyword leaked into the build');
  assert.ok(!/^\s*import\s/m.test(script), 'import keyword leaked into the build');
  assert.equal((html.match(/<\/script>/g) || []).length, 1);

  // Compile the inlined script so a broken build fails here, not in a browser.
  assert.doesNotThrow(
    () => new vm.Script(script, { filename: 'demo/index.html#script' }),
    'inlined script must parse',
  );
});

test('the demo source stays module-clean for the inliner', async () => {
  for (const file of ['core.js', 'scene.js']) {
    const source = await readFile(path.join(here, '..', 'src', file), 'utf8');
    assert.ok(!/<\/script/i.test(source), `${file} closes the host script`);
    assert.ok(!/^\s*import\s/m.test(source), `${file} imports a module`);
  }
  // The template legitimately owns its closing style/script tags; only the
  // pieces that get inlined must stay free of them.
  const css = await readFile(path.join(here, '..', 'styles.css'), 'utf8');
  assert.ok(!/<\/style/i.test(css), 'styles.css closes the host style');
});

test('a built index.html is present and current when the build has been run', async (t) => {
  const out = path.join(here, '..', 'index.html');
  try {
    const info = await stat(out);
    assert.ok(info.size > 200_000);
    const text = await readFile(out, 'utf8');
    assert.match(text, /__CW_ASSET__/);
    assert.ok(!text.includes('__PET_DATA_URL__'));
  } catch (error) {
    if (error && error.code === 'ENOENT') {
      t.skip('run `node build.mjs` first to emit demo/index.html');
      return;
    }
    throw error;
  }
});
