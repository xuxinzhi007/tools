import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { analyzeGates } from '../index.js';

const stages = [
  {
    id: 1,
    name: '写下决定',
    requires: ['原话', '本质', '未知'],
    artifacts: ['specs/<feature>/decision.md'],
    code: false,
    gateCheck: '',
  },
  {
    id: 2,
    name: '按决定改',
    requires: [],
    artifacts: [],
    code: true,
    gateCheck: 'test -n "$(git status --porcelain)"',
  },
];

async function fixture() {
  const repo = await mkdtemp(path.join(tmpdir(), 'dsh-hud-gates-'));
  const spec = path.join(repo, 'specs', 'demo');
  await mkdir(spec, { recursive: true });
  const run = {
    workflow: 'change',
    feature: 'demo',
    stage: 1,
    status: 'working',
    question: '',
    reply: '',
    raw_request: 'demo',
  };
  await writeFile(path.join(spec, 'run.json'), JSON.stringify(run), 'utf8');
  return { repo, spec, run };
}

test('analyzeGates reports missing required fields as a failure', async (t) => {
  const { repo, spec, run } = await fixture();
  t.after(() => rm(repo, { recursive: true, force: true }));

  await writeFile(
    path.join(spec, 'decision.md'),
    '- 原话（一字不改）：demo\n- 本质：demo\n',
    'utf8',
  );

  const gate = await analyzeGates(repo, 'demo', run, stages, 'active');
  assert.equal(gate.state, 'fail');
  assert.equal(gate.label, '缺必填项');
  assert.deepEqual(
    gate.requires.filter((item) => !item.found).map((item) => item.label),
    ['未知'],
  );
  assert.deepEqual(gate.artifacts, [{ path: 'specs/demo/decision.md', exists: true }]);
  assert.equal(gate.openDecision, false);
  assert.equal(gate.evidence, null);
});

test('analyzeGates reports an unanswered decision as open', async (t) => {
  const { repo, spec, run } = await fixture();
  t.after(() => rm(repo, { recursive: true, force: true }));

  await writeFile(
    path.join(spec, 'decision.md'),
    '- 原话（一字不改）：demo\n- 本质：demo\n- 未知：无\n- 待拍板：要不要改\n',
    'utf8',
  );

  const gate = await analyzeGates(repo, 'demo', run, stages, 'active');
  assert.equal(gate.state, 'open');
  assert.equal(gate.openDecision, true);
  assert.deepEqual(gate.openDocs, ['specs/demo/decision.md']);
});

test('analyzeGates trusts a matching evidence row and verify exit', async (t) => {
  const { repo, spec, run } = await fixture();
  t.after(() => rm(repo, { recursive: true, force: true }));

  await writeFile(
    path.join(spec, 'decision.md'),
    '- 原话（一字不改）：demo\n- 本质：demo\n- 未知：无\n',
    'utf8',
  );
  await writeFile(
    path.join(spec, 'evidence.log'),
    'stage=1 requires=ok gate_check=none verify=none head=abc at=2026-01-01 by=runner\n',
    'utf8',
  );
  await writeFile(path.join(spec, 'verify.log'), 'exit 0\ncmd npm test\nPASS\n', 'utf8');

  const gate = await analyzeGates(repo, 'demo', run, stages, 'active');
  assert.equal(gate.state, 'pass');
  assert.equal(gate.requires.every((item) => item.found), true);
  assert.equal(gate.evidence.stage, 1);
  assert.equal(gate.verify.exit, 0);
  assert.equal(gate.verify.cmd, 'npm test');
});

test('analyzeGates fails a run whose evidence records a non-zero verify', async (t) => {
  const { repo, spec, run } = await fixture();
  t.after(() => rm(repo, { recursive: true, force: true }));

  await writeFile(
    path.join(spec, 'decision.md'),
    '- 原话（一字不改）：demo\n- 本质：demo\n- 未知：无\n',
    'utf8',
  );
  await writeFile(
    path.join(spec, 'evidence.log'),
    'stage=1 requires=ok gate_check=none verify=exit3 head=abc at=2026-01-01 by=runner\n',
    'utf8',
  );
  await writeFile(path.join(spec, 'verify.log'), 'exit 3\ncmd npm test\nFAIL\n', 'utf8');

  const gate = await analyzeGates(repo, 'demo', run, stages, 'active');
  assert.equal(gate.state, 'fail');
  assert.match(gate.reason, /verify=exit3/);
  assert.equal(gate.verify.exit, 3);
  assert.equal(gate.history.length, 1);
  assert.equal(gate.history[0].kind, 'FAIL');
  assert.match(gate.history[0].detail, /verify=exit3/);
});

test('analyzeGates trims the code stage differently and reads the write gate', async (t) => {
  const { repo, spec, run } = await fixture();
  t.after(() => rm(repo, { recursive: true, force: true }));

  run.stage = 2;
  await writeFile(
    path.join(repo, 'config.yaml'),
    'protected_paths: "api/**,*.proto"\nwaiver_globs: "*.go"\n',
    'utf8',
  );
  await writeFile(
    path.join(repo, 'specs', 'passes.log'),
    'design demo api approved 2026-01-01 "可以"\n# comment\n',
    'utf8',
  );

  const gate = await analyzeGates(repo, 'demo', run, stages, 'active');
  assert.equal(gate.state, 'working');
  assert.equal(gate.gateCheckDeclared, true);
  assert.equal(gate.protection.configured, true);
  assert.deepEqual(gate.protection.protectedPaths, ['api/**', '*.proto']);
  assert.deepEqual(gate.protection.waiverGlobs, ['*.go']);
  assert.equal(gate.protection.passesCount, 1);
});

test('analyzeGates exposes every stage and keeps future stages pending', async (t) => {
  const { repo, spec, run } = await fixture();
  t.after(() => rm(repo, { recursive: true, force: true }));

  const wide = [
    ...stages,
    {
      id: 3,
      name: '核对',
      requires: ['exit'],
      artifacts: ['specs/<feature>/check.md'],
      code: false,
      gateCheck: '',
    },
  ];
  run.stage = 2;
  await writeFile(
    path.join(spec, 'decision.md'),
    '- 原话（一字不改）：demo\n- 本质：demo\n- 未知：无\n',
    'utf8',
  );
  await writeFile(
    path.join(spec, 'evidence.log'),
    'stage=1 requires=ok gate_check=none verify=none head=abc at=2026-01-01 by=runner\n',
    'utf8',
  );

  const gate = await analyzeGates(repo, 'demo', run, wide, 'active');
  assert.equal(gate.stageGates.length, 3);
  assert.deepEqual(
    gate.stageGates.map((entry) => entry.name),
    ['写下决定', '按决定改', '核对'],
  );
  assert.equal(gate.stageGates[0].tone, 'pass');
  assert.equal(gate.stageGates[0].label, '阶段已过');
  assert.equal(gate.stageGates[1].tone, 'working');
  assert.equal(gate.stageGates[1].label, '待 done');
  assert.equal(gate.stageGates[2].tone, 'pending');
  assert.equal(gate.stageGates[2].label, '未开始');
  // A future stage must not be painted as missing its artifacts.
  assert.deepEqual(gate.stageGates[2].missingArtifacts, []);
  assert.deepEqual(gate.stageGates[2].missingRequires, []);
  assert.equal(gate.stageGates[2].reason, '等待前序阶段完成');
  assert.equal(gate.state, 'working');
});

test('analyzeGates reports a finished run with evidence as passed', async (t) => {
  const { repo, spec, run } = await fixture();
  t.after(() => rm(repo, { recursive: true, force: true }));

  run.stage = 2;
  run.status = 'done';
  await writeFile(
    path.join(spec, 'evidence.log'),
    'stage=2 requires=ok gate_check=ok verify=exit0 head=abc at=2026-01-01 by=runner\n',
    'utf8',
  );
  await writeFile(path.join(spec, 'verify.log'), 'exit 0\ncmd npm test\n', 'utf8');

  const gate = await analyzeGates(repo, 'demo', run, stages, 'finished');
  assert.equal(gate.state, 'pass');
  assert.equal(gate.label, '已留证据');
  assert.equal(gate.evidence.gateCheck, 'ok');
  assert.equal(gate.history.at(-1).kind, 'PASS');
  assert.match(gate.history.at(-1).detail, /STAGE 2/);
  assert.equal(gate.next, '运行已结束；需要新工作时运行 start 开启下一次');
});
