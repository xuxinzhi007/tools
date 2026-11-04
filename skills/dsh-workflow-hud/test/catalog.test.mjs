import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { catalogFromRepo } from '../index.js';

async function fixture() {
  const repo = await mkdtemp(path.join(tmpdir(), 'dsh-hud-catalog-'));
  const dir = path.join(repo, 'skills', 'data-workflow', 'workflows');
  await mkdir(dir, { recursive: true });
  await writeFile(
    path.join(dir, 'change.json'),
    JSON.stringify({
      name: 'change',
      kind: 'execute',
      stages: [
        { id: 1, name: '写下决定', requires: ['原话', '本质'], artifacts: ['specs/<feature>/decision.md'] },
        {
          id: 2,
          name: '按决定改',
          code: true,
          gate_check: 'test -n "$(git status --porcelain)"',
        },
      ],
    }),
    'utf8',
  );
  await writeFile(
    path.join(dir, 'feature.json'),
    JSON.stringify({
      name: 'feature',
      kind: 'execute',
      stages: [
        { id: 1, name: '需求' },
        { id: 2, name: '影响' },
        { id: 3, name: '方案' },
        { id: 4, name: '实现', code: true },
      ],
    }),
    'utf8',
  );
  return { repo, dir };
}

test('catalogFromRepo lists installed workflows with their stage chains', async (t) => {
  const { repo } = await fixture();
  t.after(() => rm(repo, { recursive: true, force: true }));

  const catalog = await catalogFromRepo(repo);
  const ids = catalog.map((item) => item.id);
  assert.equal(new Set(ids).size, ids.length, 'workflow ids must be unique');

  const change = catalog.find((item) => item.id === 'change');
  assert.ok(change, 'change must be present');
  assert.equal(change.kind, 'execute');
  assert.equal(change.stages.length, 2);
  assert.deepEqual(
    change.stages.map((stage) => stage.name),
    ['写下决定', '按决定改'],
  );
  assert.equal(change.stages[0].requires, 2);
  assert.equal(change.stages[0].artifacts, 1);
  assert.equal(change.stages[1].code, true);
  assert.equal(change.stages[1].gateCheck, true);

  const feature = catalog.find((item) => item.id === 'feature');
  assert.ok(feature, 'feature must be present');
  assert.equal(feature.stages.length, 4);
  assert.equal(feature.stages[3].code, true);
});

test('catalogFromRepo omits workflows the repository does not install', async (t) => {
  const { repo, dir } = await fixture();
  t.after(() => rm(repo, { recursive: true, force: true }));

  await rm(path.join(dir, 'feature.json'));

  const catalog = await catalogFromRepo(repo, { homeFallback: false });
  const ids = catalog.map((item) => item.id);
  assert.equal(ids.includes('change'), true);
  assert.equal(ids.includes('feature'), false);
});

test('catalogFromRepo returns an empty list without the skill', async (t) => {
  const repo = await mkdtemp(path.join(tmpdir(), 'dsh-hud-catalog-empty-'));
  t.after(() => rm(repo, { recursive: true, force: true }));

  // Without the home fallback a repo with no skill has nothing to list.
  const catalog = await catalogFromRepo(repo, { homeFallback: false });
  assert.equal(Array.isArray(catalog), true);
  for (const item of catalog) {
    assert.equal(typeof item.id, 'string');
    assert.equal(Array.isArray(item.stages), true);
  }
});
