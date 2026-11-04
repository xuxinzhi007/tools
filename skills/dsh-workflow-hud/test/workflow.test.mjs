import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  FALLBACK_STAGES,
  configList,
  configScalar,
  contentText,
  featureFromEvents,
  featureFromOutput,
  featureFromTool,
  hasOpenDecision,
  isOpenRun,
  isWorkflowSignal,
  normalizeFeature,
  normalizeWorkflow,
  parseArgs,
  parseEvidence,
  parseEvidenceRows,
  parseExit,
  parseVerdict,
  redact,
  runStateOf,
  selectRunRecord,
  stageArtifacts,
  stagesFromWorkflow,
  toolDetail,
  toolKind,
  verifyCommand,
} from '../lib/workflow.js';

test('parseArgs accepts JSON and rejects junk', () => {
  assert.deepEqual(parseArgs('{"a":1}'), { a: 1 });
  assert.deepEqual(parseArgs({ a: 1 }), { a: 1 });
  assert.deepEqual(parseArgs('not json'), {});
  assert.deepEqual(parseArgs('[1,2]'), {});
  assert.deepEqual(parseArgs(''), {});
});

test('toolDetail keeps short human-readable summaries', () => {
  assert.equal(toolDetail('bash', { command: 'echo hello' }), 'echo hello');
  assert.equal(
    toolDetail('read', { file_path: '/Users/me/project/skills/data-workflow/SKILL.md' }),
    'data-workflow/SKILL.md',
  );
  assert.equal(
    toolDetail('grep', { pattern: 'run\\.json', path: '/repo/specs/demo' }),
    'specs/demo · run\\.json',
  );
  assert.equal(
    toolDetail('web_fetch', { url: 'https://example.com/a/b?q=1' }),
    'example.com/a/b',
  );
  assert.equal(toolDetail('todo_write', { todos: [] }), '0 项');
  assert.equal(
    toolDetail('todo_write', {
      todos: [{ content: 'implement the HUD', status: 'in_progress' }],
    }),
    'implement the HUD',
  );
});

test('toolKind groups the calls used by the HUD', () => {
  assert.equal(toolKind('read'), 'read');
  assert.equal(toolKind('edit'), 'write');
  assert.equal(toolKind('grep'), 'search');
  assert.equal(toolKind('bash'), 'exec');
  assert.equal(toolKind('web_search'), 'net');
  assert.equal(toolKind('skill'), 'skill');
  assert.equal(toolKind('unknown_tool'), 'other');
});

test('isWorkflowSignal recognizes calls that can reveal workflow state', () => {
  assert.equal(
    isWorkflowSignal('bash', { command: 'python3 scripts/workflow_next.py next' }),
    true,
  );
  assert.equal(isWorkflowSignal('bash', { command: 'ls -la' }), false);
  assert.equal(isWorkflowSignal('read', { file_path: '/repo/specs/demo/run.json' }), true);
  assert.equal(isWorkflowSignal('read', { file_path: '/repo/src/index.js' }), false);
  assert.equal(isWorkflowSignal('skill', { name: 'data-workflow' }), true);
  assert.equal(isWorkflowSignal('skill', { name: 'office' }), false);
});

test('stagesFromWorkflow keeps gate metadata', () => {
  const stages = stagesFromWorkflow(
    {
      stages: [
        {
          id: 3,
          name: '技术方案',
          requires: ['原话', '本质'],
          artifacts: ['specs/<feature>/plan.md'],
          code: false,
          gate_check: 'test -n "$(git status --porcelain)"',
        },
        { id: 4, name: '编码实现', code: true },
      ],
    },
    'change',
  );
  assert.deepEqual(stages, [
    {
      id: 3,
      name: '技术方案',
      requires: ['原话', '本质'],
      artifacts: ['specs/<feature>/plan.md'],
      code: false,
      gateCheck: 'test -n "$(git status --porcelain)"',
    },
    {
      id: 4,
      name: '编码实现',
      requires: [],
      artifacts: [],
      code: true,
      gateCheck: '',
    },
  ]);

  const fallback = stagesFromWorkflow(null, 'change');
  assert.equal(fallback.length, FALLBACK_STAGES.change.length);
  assert.deepEqual(fallback[0], {
    id: 1,
    name: FALLBACK_STAGES.change[0],
    requires: [],
    artifacts: [],
    code: false,
    gateCheck: '',
  });
  assert.equal(stagesFromWorkflow(null, 'feature').length, 7);
  assert.deepEqual(stagesFromWorkflow(null, 'unknown'), []);
});

test('run helpers normalize ids and states', () => {
  assert.equal(normalizeWorkflow('review_only'), 'review-only');
  assert.equal(normalizeWorkflow(' Feature '), 'feature');
  assert.equal(normalizeWorkflow(undefined), '');
  assert.equal(normalizeFeature('demo-1'), 'demo-1');
  assert.equal(normalizeFeature(' bad name '), '');
  assert.equal(normalizeFeature('../x'), '');

  assert.equal(isOpenRun({ status: 'working' }), true);
  assert.equal(isOpenRun({ status: 'waiting' }), true);
  assert.equal(isOpenRun({ status: 'done' }), false);
  assert.equal(runStateOf({ status: 'working' }), 'active');
  assert.equal(runStateOf({ status: 'done' }), 'finished');
  assert.equal(runStateOf({ status: 'abandoned' }), 'abandoned');
  assert.equal(runStateOf({}), 'unknown');
});

test('selectRunRecord never substitutes an unrelated finished run', () => {
  const records = [
    { feature: 'old', status: 'done', mtimeMs: 300 },
    { feature: 'open', status: 'working', mtimeMs: 200 },
    { feature: 'newer-old', status: 'done', mtimeMs: 400 },
  ];

  assert.equal(selectRunRecord(records), null);
  assert.equal(selectRunRecord(records, ''), null);
  assert.equal(selectRunRecord(records, 'missing'), null);

  const own = selectRunRecord(records, 'old');
  assert.equal(own.record.feature, 'old');
  assert.equal(own.runState, 'finished');
  assert.equal(own.association, 'session');

  const open = selectRunRecord(records, '', 'open');
  assert.equal(open.record.feature, 'open');
  assert.equal(open.runState, 'active');
  assert.equal(open.association, 'repo');

  const history = selectRunRecord(records, '', 'history');
  assert.equal(history.record.feature, 'newer-old');
  assert.equal(history.runState, 'finished');
  assert.equal(history.association, 'history');
});

test('featureFromTool only accepts strong session evidence', () => {
  assert.equal(
    featureFromTool('bash', {
      command: 'python3 x/workflow_next.py next docs-drift --repo .',
    }),
    'docs-drift',
  );
  assert.equal(
    featureFromTool('bash', {
      command: 'python3 x/workflow_next.py start change demo --repo .',
    }),
    'demo',
  );
  assert.equal(
    featureFromTool('bash', {
      command: 'python3 x/workflow_next.py done evidence-anchor --repo .',
    }),
    'evidence-anchor',
  );
  assert.equal(featureFromTool('bash', { command: 'ls -la' }), '');
  assert.equal(featureFromTool('read', { file_path: '/repo/specs/demo/run.json' }), '');
  assert.equal(featureFromTool('read', { file_path: '/repo/specs/demo/decision.md' }), '');
  assert.equal(
    featureFromTool('edit', { file_path: '/repo/specs/demo/decision.md' }),
    'demo',
  );
  assert.equal(
    featureFromTool('write', { file_path: 'C:\\repo\\specs\\demo\\check.md' }),
    'demo',
  );
});

test('featureFromOutput reads runner lines but rejects junk', () => {
  const output = [
    'RUN docs-drift change working',
    'RECORD specs/docs-drift/run.json',
  ].join('\n');
  assert.equal(featureFromOutput(output), 'docs-drift');
  assert.equal(featureFromOutput('RECORD specs/demo/run.json'), 'demo');
  assert.equal(featureFromOutput('RUN'), '');
  assert.equal(featureFromOutput('RUN bad/name change working'), '');
  assert.equal(featureFromOutput('hello'), '');
});

test('featureFromEvents recovers the newest run association from the log', () => {
  const events = [
    { type: 'tool/call', data: { name: 'read', arguments: '{"file_path":"/repo/specs/old/run.json"}' } },
    {
      type: 'tool/call',
      data: {
        name: 'bash',
        arguments: '{"command":"python3 scripts/workflow_next.py next demo-a"}',
      },
    },
    {
      type: 'tool/result',
      data: { message: { content: [{ type: 'text', text: 'RUN demo-b change working' }] } },
    },
    {
      type: 'tool/result',
      data: {
        message: {
          content: [
            { type: 'text', text: 'RUN demo-c change working' },
            { type: 'text', text: 'RECORD specs/demo-c/run.json' },
          ],
        },
      },
    },
  ];
  assert.equal(featureFromEvents(events), 'demo-c');
  assert.equal(
    featureFromEvents([
      {
        type: 'tool/call',
        data: {
          name: 'bash',
          arguments: '{"command":"python3 x/workflow_next.py done demo-a"}',
        },
      },
      { type: 'tool/result', data: { message: { content: [{ type: 'text', text: 'DONE demo-a' }] } } },
    ]),
    'demo-a',
  );
  assert.equal(featureFromEvents([]), '');
  assert.equal(featureFromEvents(undefined), '');
  assert.equal(
    featureFromEvents([
      { type: 'tool/call', data: { name: 'read', arguments: '{"file_path":"/repo/specs/demo/x.md"}' } },
    ]),
    '',
  );
});

test('contentText flattens text blocks only', () => {
  assert.equal(
    contentText([
      { type: 'text', text: 'line one' },
      { type: 'image' },
      { type: 'text', text: 'line two' },
    ]),
    'line one\nline two',
  );
  assert.equal(contentText(undefined), '');
  assert.equal(contentText([{ type: 'text' }]), '');
});

test('evidence parsing keeps the last row and all rows', () => {
  const text = [
    'stage=1 requires=ok gate_check=none verify=none head=aaa at=2026-01-01 by=runner',
    'stage=2 requires=ok gate_check=ok verify=exit0 head=bbb at=2026-01-02 by=runner',
  ].join('\n');
  const rows = parseEvidenceRows(text);
  assert.equal(rows.length, 2);
  assert.equal(rows[1].stage, '2');
  assert.equal(rows[1].gate_check, 'ok');
  assert.equal(rows[1].verify, 'exit0');

  const last = parseEvidence(text);
  assert.equal(last.stage, '2');
  assert.equal(last.line.startsWith('stage=2'), true);
  assert.deepEqual(parseEvidence(''), { line: '' });
});

test('verify and decision parsing matches the runner rules', () => {
  assert.equal(parseExit('exit 0\ncmd python3 x.py'), 0);
  assert.equal(parseExit('exit 3\nboom'), 3);
  assert.equal(parseExit('PASS'), null);
  assert.equal(verifyCommand('exit 0\ncmd python3 skills/x.py\nPASS'), 'python3 skills/x.py');
  assert.equal(verifyCommand('PASS'), '');

  assert.equal(hasOpenDecision('- 待拍板：要不要改'), true);
  assert.equal(hasOpenDecision('- 待拍板：要不要改\n- 已拍板：改'), false);
  assert.equal(hasOpenDecision('正文里提到 待拍板 不算'), false);
  assert.equal(hasOpenDecision('- 已拍板：改\n- 待拍板：再问'), true);
  assert.equal(hasOpenDecision(''), false);
});

test('stageArtifacts mirrors the runner artifact rules', () => {
  assert.deepEqual(
    stageArtifacts(
      {
        artifacts: [
          'specs/<feature>/decision.md',
          '<artifacts_root>/check.md',
          'decision.md',
          '代码改动',
          'api/**/x.go（仅当 stack=go）',
          'api/**/x.go',
          '$KNOW/<域>/（一次一个）',
        ],
      },
      'demo',
    ),
    ['specs/demo/decision.md', 'specs/demo/check.md'],
  );
  assert.deepEqual(
    stageArtifacts({ artifacts: ['decision.md'] }, ''),
    [],
  );
});

test('configScalar and configList read the repo config safely', () => {
  const text = [
    '# comment',
    'protected_paths: "api/**,*.proto"',
    "waiver_globs: ''",
    'verify: "python3 x.py" # trailing',
    'repo: /tmp/x',
  ].join('\n');
  assert.equal(configScalar(text, 'protected_paths'), 'api/**,*.proto');
  assert.equal(configScalar(text, 'waiver_globs'), '');
  assert.equal(configScalar(text, 'verify'), 'python3 x.py');
  assert.equal(configScalar(text, 'repo'), '/tmp/x');
  assert.equal(configScalar(text, 'missing'), '');
  assert.deepEqual(configList(text, 'protected_paths'), ['api/**', '*.proto']);
  assert.deepEqual(configList(text, 'waiver_globs'), []);
});

test('parseVerdict keeps the runner conclusion and its key detail', () => {
  const gate = parseVerdict(
    'RUN demo change working\nSTAGE 1 写下决定\nGATE\n缺：可能理解错、未知\nREAD x.md',
  );
  assert.equal(gate.kind, 'GATE');
  assert.match(gate.detail, /STAGE 1/);
  assert.match(gate.detail, /可能理解错/);

  assert.equal(parseVerdict('DONE demo').kind, 'DONE');
  assert.equal(parseVerdict('NEED_ANSWER 没有已答复的拍板').kind, 'NEED_ANSWER');
  assert.equal(parseVerdict('no runner output'), null);

  const need = parseVerdict(
    [
      'RUN demo change working',
      'STAGE 1 写下决定',
      'READ x/change.md',
      'NEED specs/demo/decision.md',
      'NEED specs/demo/plan.md',
      '写完后运行：',
    ].join('\n'),
  );
  assert.equal(need.kind, 'NEED');
  assert.match(need.detail, /NEED specs\/demo\/decision\.md/);
  assert.match(need.detail, /plan\.md/);

  const open = parseVerdict(
    [
      'RUN demo change waiting',
      'OPEN',
      'STAGE 1 写下决定',
      '文件里还有待拍板。',
      'QUESTION 要不要改？',
      'REPLY 改',
    ].join('\n'),
  );
  assert.equal(open.kind, 'OPEN');
  assert.match(open.detail, /QUESTION 要不要改/);
  assert.match(open.detail, /REPLY 改/);
});

test('redact removes common credential shapes', () => {
  assert.equal(redact('sk-abc123456789'), 'sk-***');
  assert.equal(redact('Authorization: Bearer abcdefghijk'), 'Authorization: Bearer ***');
  assert.equal(redact('token ghp_abcdefghijk'), 'token gh-***');
});

test('manifest exports locale, icon, and client files', () => {
  const manifest = JSON.parse(
    readFileSync(new URL('../package.json', import.meta.url), 'utf8'),
  );
  assert.equal(manifest.name, '@local/dsh-workflow-hud');
  assert.equal(manifest.exports['./client'], './client.js');
  assert.equal(manifest.exports['./icon'], './icon.svg');
  assert.equal(manifest.dsh.bundle.patch, './cordis.patch.yml');
  assert.equal(manifest.dsh.client.platform, 'web');
  assert.equal(manifest.files.includes('lib/*.js'), true);
});

test('parseVerdict captures verify output on done', () => {
  const verdict = parseVerdict(
    [
      'VERIFY exit 0',
      'LOG specs/demo/verify.log',
      'RUN demo change working',
      'RECORD specs/demo/run.json',
      'DONE demo',
    ].join('\n'),
  );
  assert.equal(verdict.kind, 'DONE');
  assert.match(verdict.detail, /VERIFY exit 0/);
});
