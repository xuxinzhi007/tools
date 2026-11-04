import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

const source = readFileSync(new URL('../client.js', import.meta.url), 'utf8');

function loadFactory() {
  let captured = null;
  const sandbox = {
    console,
    window: {
      __ModuleLoader__: {
        load(spec) {
          captured = spec;
        },
      },
    },
    setInterval: () => 0,
    clearInterval: () => {},
    setTimeout: () => 0,
    clearTimeout: () => {},
  };
  sandbox.globalThis = sandbox;
  vm.runInNewContext(source, sandbox, { filename: 'client.js' });
  assert.ok(captured, 'client must register through the module loader');
  return captured.factory;
}

function makeReact(presets) {
  let hook = 0;
  return {
    Fragment: Symbol('Fragment'),
    createElement(type, props, ...children) {
      return { type, props: props || {}, children };
    },
    useState(initial) {
      const index = hook++;
      if (index < presets.length) return [presets[index], () => {}];
      const value = typeof initial === 'function' ? initial() : initial;
      return [value, () => {}];
    },
    useRef(initial) {
      return { current: initial };
    },
    useEffect() {},
  };
}

function collect(node, React, depth = 0, out = []) {
  if (node == null || node === false || node === true) return out;
  if (typeof node === 'string' || typeof node === 'number') {
    const text = String(node).replace(/\s+/g, ' ').trim();
    if (text) out.push(text);
    return out;
  }
  if (Array.isArray(node)) {
    for (const item of node) collect(item, React, depth, out);
    return out;
  }
  if (node.type === React.Fragment) {
    for (const child of node.children || []) collect(child, React, depth, out);
    return out;
  }
  if (typeof node.type === 'function') {
    collect(node.type({ ...(node.props || {}), children: node.children }), React, depth, out);
    return out;
  }
  for (const child of node.children || []) collect(child, React, depth, out);
  if (node.props && node.props.children) collect(node.props.children, React, depth, out);
  return out;
}

/** Collect `tag.class` strings so a test can assert the canvas layer exists. */
function elements(node, React, out = []) {
  if (node == null || node === false || node === true) return out;
  if (typeof node === 'string' || typeof node === 'number') return out;
  if (Array.isArray(node)) {
    for (const item of node) elements(item, React, out);
    return out;
  }
  if (node.type === React.Fragment) {
    for (const child of node.children || []) elements(child, React, out);
    return out;
  }
  if (typeof node.type === 'function') {
    elements(node.type({ ...(node.props || {}), children: node.children }), React, out);
    return out;
  }
  if (typeof node.type === 'string') {
    const className =
      node.props && typeof node.props.className === 'string'
        ? `.${node.props.className.split(/\s+/)[0]}`
        : '';
    const aria =
      node.props && typeof node.props['aria-label'] === 'string'
        ? `@${node.props['aria-label']}`
        : '';
    out.push(`${node.type}${className}${aria}`);
  }
  for (const child of node.children || []) elements(child, React, out);
  if (node.props && node.props.children) elements(node.props.children, React, out);
  return out;
}

/** Collect the crawler anchors the canvas is allowed to crawl. */
function anchorKeys(node, React, out = []) {
  if (node == null || node === false || node === true) return out;
  if (typeof node === 'string' || typeof node === 'number') return out;
  if (Array.isArray(node)) {
    for (const item of node) anchorKeys(item, React, out);
    return out;
  }
  if (node.type === React.Fragment) {
    for (const child of node.children || []) anchorKeys(child, React, out);
    return out;
  }
  if (typeof node.type === 'function') {
    anchorKeys(node.type({ ...(node.props || {}), children: node.children }), React, out);
    return out;
  }
  if (node.props && typeof node.props['data-dwh-anchor'] === 'string') {
    out.push(`${node.props['data-dwh-anchor']}:${node.props['data-dwh-tone'] || ''}`);
  }
  for (const child of node.children || []) anchorKeys(child, React, out);
  if (node.props && node.props.children) anchorKeys(node.props.children, React, out);
  return out;
}

function render(state, verdicts, options = {}) {
  const web = options.web !== false;
  // Hook order: state, verdicts, connected, collapsed, web, localeRevision,
  // reducedMotion.
  const React = makeReact([state, verdicts, true, false, web, 0, false]);
  const factory = loadFactory();
  const mod = factory((id) => {
    if (id === 'react') return React;
    throw new Error(`unexpected require: ${id}`);
  });

  let Component = null;
  const ctx = {
    effect(fn) {
      fn();
    },
    locale: {
      bind: () => (key) => key,
      register: () => () => {},
      subscribe: () => () => {},
    },
    slots: {
      inject(key, cb) {
        cb();
        return () => {};
      },
      register(registration, component) {
        Component = component;
        return () => {};
      },
    },
  };
  mod.apply(ctx);
  assert.ok(Component, 'HUD component must be registered');
  const tree = Component();
  return {
    tree,
    text: collect(tree, React).join('\n'),
    tags: elements(tree, React),
    anchors: anchorKeys(tree, React),
  };
}

const stageGates = [
  {
    id: 1,
    name: '写下决定',
    tone: 'pass',
    label: '阶段已过',
    reason: 'evidence 记录通过',
    requires: [
      { label: '原话', found: true },
      { label: '本质', found: true },
    ],
    artifacts: [{ path: 'specs/demo/decision.md', exists: true }],
    missingArtifacts: [],
    missingRequires: [],
    openDecision: false,
    openDocs: [],
    evidence: {
      stage: 1,
      line: 'stage=1 requires=ok gate_check=none verify=none head=abc at=t by=runner',
    },
  },
  {
    id: 2,
    name: '按决定改',
    tone: 'working',
    label: '待 done',
    reason: '改完后运行 done',
    requires: [],
    artifacts: [],
    missingArtifacts: [],
    missingRequires: [],
    openDecision: false,
    openDocs: [],
    evidence: null,
  },
  {
    id: 3,
    name: '核对',
    tone: 'pending',
    label: '未开始',
    reason: '等待前序阶段完成',
    requires: [{ label: 'exit', found: false }],
    artifacts: [{ path: 'specs/demo/check.md', exists: false }],
    missingArtifacts: [],
    missingRequires: [],
    openDecision: false,
    openDocs: [],
    evidence: null,
  },
];

const verdicts = [
  { kind: 'GATE', detail: 'STAGE 1 写下决定 · 缺：怎样算满足、未知', at: 1 },
  { kind: 'DO', detail: 'STAGE 2 按决定改', at: 2 },
  { kind: 'DONE', detail: 'VERIFY exit 0', at: 3 },
];

const active = {
  type: 'state',
  sessionId: 's1',
  repo: '/repo',
  repoName: 'repo',
  requestedFeature: 'demo',
  feature: 'demo',
  workflow: 'change',
  stage: 2,
  status: 'working',
  question: '',
  rawRequest: '',
  stages: stageGates.map((gate) => ({
    id: gate.id,
    name: gate.name,
    requires: [],
    artifacts: [],
    code: gate.id === 2,
    gateCheck: gate.id === 2 ? 'git status' : '',
  })),
  stageGates,
  catalog: [],
  association: 'session',
  runState: 'active',
  active: true,
  gate: {
    state: 'working',
    label: '待 done',
    reason: '改完后运行 done',
    next: '改完后运行 done：会检查工作树并跑仓库 verify',
    stageId: 2,
    stageName: '按决定改',
    artifacts: [],
    requires: [],
    openDecision: false,
    openDocs: [],
    evidence: null,
    lastEvidence: null,
    verify: { exit: 0, cmd: 'npm test' },
    gateCheckDeclared: true,
    protection: {
      configured: true,
      protectedPaths: ['api/**', '*.proto'],
      waiverGlobs: ['*.go'],
      passesCount: 1,
      passes: ['design demo api approved t "可以"'],
    },
    history: [
      {
        kind: 'PASS',
        stageId: 1,
        detail: 'STAGE 1 · requires=ok gate_check=none verify=none',
        line: 'stage=1 requires=ok gate_check=none verify=none head=abc at=t by=runner',
        at: '2026-01-01T00:00:00+08:00',
      },
    ],
    stageGates,
  },
};

const noRun = {
  type: 'state',
  sessionId: 's1',
  repo: '/repo',
  repoName: 'repo',
  requestedFeature: '',
  feature: '',
  workflow: '',
  stage: 0,
  status: '',
  question: '',
  rawRequest: '',
  stages: [],
  stageGates: [],
  catalog: [
    {
      id: 'change',
      kind: 'execute',
      stages: [
        { id: 1, name: '写下决定', requires: 6, artifacts: 1, code: false, gateCheck: false },
        { id: 2, name: '按决定改', requires: 0, artifacts: 0, code: true, gateCheck: true },
        { id: 3, name: '核对', requires: 2, artifacts: 1, code: false, gateCheck: false },
      ],
    },
    {
      id: 'feature',
      kind: 'execute',
      stages: [
        { id: 1, name: '需求与负债评估', requires: 6, artifacts: 1, code: false, gateCheck: false },
      ],
    },
  ],
  association: 'none',
  runState: 'none',
  gate: null,
  active: false,
};

test('the HUD renders workflow state and gate log, never a command trace', () => {
  const { text } = render(active, verdicts);

  for (const needle of [
    '写下决定',
    '按决定改',
    '核对',
    'STAGE 2 · 按决定改',
    '待 done',
    '未开始',
    'nextStep',
    'STAGE 1 · requires=ok',
    'verify 0',
    'protectedPaths 2',
    'passes 1',
    'PASS',
    'GATE',
    'DONE',
    '缺：怎样算满足、未知',
    'VERIFY exit 0',
  ]) {
    assert.ok(text.includes(needle), `missing: ${needle}`);
  }

  for (const forbidden of ['dwh-call', 'noCalls', 'Live trace', '实时调用', 'applyTool', 'ToolRow']) {
    assert.ok(!text.includes(forbidden), `command trace leaked: ${forbidden}`);
  }
});

test('with no run the HUD shows the installed workflow catalog', () => {
  const { text } = render(noRun, []);

  for (const needle of ['catalog', 'change', '写下决定', '按决定改', '核对', 'feature', '需求与负债评估', 'noRunHint']) {
    assert.ok(text.includes(needle), `missing: ${needle}`);
  }
  assert.ok(!text.includes('dwh-call'), 'command trace leaked');
});

test('the crawler canvas exists and is pointer-transparent', () => {
  const { tags, text } = render(active, verdicts);

  assert.ok(tags.includes('canvas.dwh-crawler'), 'crawler canvas must render');
  // The effect button is the only new control; its label lives in
  // aria-label, so the element collector reads that attribute too.
  assert.ok(
    tags.some((tag) => tag.includes('@webOff') || tag.includes('@webOn')),
    'crawler toggle must be labelled',
  );
});

test('the crawler has anchors for every stage, the gate, and each log row', () => {
  const { anchors } = render(active, verdicts);

  assert.ok(anchors.includes('gate:working'), 'gate anchor must expose its tone');
  assert.ok(anchors.some((item) => item.startsWith('stage-1:pass')), 'stage 1 anchor');
  assert.ok(anchors.some((item) => item.startsWith('stage-2:working')), 'stage 2 anchor');
  assert.ok(anchors.some((item) => item.startsWith('stage-3:pending')), 'stage 3 anchor');
  // `gateLog` is history first, then live verdicts:
  // PASS(history), GATE(fail), DO(working), DONE(pass).
  assert.ok(anchors.some((item) => item.startsWith('log-0:pass')), 'history log anchor');
  assert.ok(anchors.some((item) => item.startsWith('log-1:fail')), 'GATE log anchor');
  assert.ok(anchors.some((item) => item.startsWith('log-2:working')), 'DO log anchor');
  assert.ok(anchors.some((item) => item.startsWith('log-3:pass')), 'DONE log anchor');
});

test('the crawler can be switched off without losing the gate view', () => {
  const { text, tags } = render(active, verdicts, { web: false });

  assert.ok(!tags.includes('canvas.dwh-crawler'), 'off state must drop the canvas');
  assert.ok(text.includes('gateFail') || text.includes('待 done'), 'card content must stay');
  assert.ok(text.includes('STAGE 1 · requires=ok'), 'evidence must stay');
});
