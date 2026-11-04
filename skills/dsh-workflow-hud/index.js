/**
 * Host half of the workflow HUD.
 *
 * The HUD is scoped to one conversation: it associates the conversation with a
 * `specs/<feature>/run.json` only through that conversation's own runner calls
 * or writes. A finished run from another conversation must never appear here.
 *
 * It also reads the artifacts the data-workflow gates actually produce:
 * evidence.log, verify.log, decision/check documents, passes.log, and the
 * repository's protected-path configuration. Everything is read-only; the
 * plugin never writes the session log and never changes tool policy.
 */
import { readFile, readdir, stat } from 'node:fs/promises';
import { homedir } from 'node:os';
import path from 'node:path';
import {
  configList,
  contentText,
  featureFromEvents,
  featureFromOutput,
  featureFromTool,
  hasOpenDecision,
  isWorkflowSignal,
  normalizeFeature,
  normalizeWorkflow,
  parseArgs,
  parseEvidenceRows,
  parseExit,
  parseVerdict,
  runStateOf,
  selectRunRecord,
  stageArtifacts,
  stagesFromWorkflow,
  verifyCommand,
} from './lib/workflow.js';

export const name = 'workflow-hud';
export const inject = ['sessions'];

const EVENTS_ENDPOINT = '/plugins/workflow-hud/events';
const KEEPALIVE_MS = 20000;
const REFRESH_MS = 600;
const MAX_VERDICTS = 8;
const ACTIVE_STATUSES = new Set(['working', 'waiting']);
const CATALOG_WORKFLOWS = ['change', 'feature', 'bugfix', 'hotfix', 'review-only'];

function sse(frame) {
  return `data: ${JSON.stringify(frame)}\n\n`;
}

function isLoopback(address) {
  return address === '127.0.0.1' || address === '::1' || address === '::ffff:127.0.0.1';
}

function isSubagent(session) {
  return session?.header?.origin === 'subagent';
}

async function readIf(file) {
  try {
    return await readFile(file, 'utf8');
  } catch {
    return '';
  }
}

async function readArtifact(repo, rel) {
  const file = path.join(repo, rel);
  let info = null;
  try {
    info = await stat(file);
  } catch {
    info = null;
  }
  if (!info || !info.isFile()) return { path: rel, exists: false, text: '' };
  return { path: rel, exists: true, text: await readIf(file) };
}

function configCandidates(repo) {
  return [
    path.join(repo, 'config.yaml'),
    path.join(repo, '.cursor', 'skills', 'data-workflow', 'config.yaml'),
    path.join(repo, '.qoder', 'skills', 'data-workflow', 'config.yaml'),
    path.join(repo, '.claude', 'skills', 'data-workflow', 'config.yaml'),
    path.join(repo, '.agents', 'skills', 'data-workflow', 'config.yaml'),
    path.join(repo, '.codex', 'skills', 'data-workflow', 'config.yaml'),
  ];
}

async function readRepoConfig(repo) {
  for (const file of configCandidates(repo)) {
    const text = await readIf(file);
    if (text) return text;
  }
  return '';
}

async function scanRuns(repo) {
  const specs = path.join(repo, 'specs');
  let entries;
  try {
    entries = await readdir(specs, { withFileTypes: true });
  } catch {
    return [];
  }

  const records = [];
  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    const file = path.join(specs, entry.name, 'run.json');
    try {
      const [text, info] = await Promise.all([readFile(file, 'utf8'), stat(file)]);
      const data = JSON.parse(text);
      if (!data || typeof data !== 'object' || Array.isArray(data)) continue;
      records.push({
        ...data,
        feature:
          typeof data.feature === 'string' && data.feature ? data.feature : entry.name,
        path: file,
        mtimeMs: info.mtimeMs,
      });
    } catch {
      // A half-written run.json is skipped until the next read.
    }
  }
  return records;
}

function workflowCandidates(repo, workflow, includeHome = true) {
  const file = `${workflow}.json`;
  const roots = [
    path.join(repo, 'skills', 'data-workflow', 'workflows'),
    path.join(repo, '.cursor', 'skills', 'data-workflow', 'workflows'),
    path.join(repo, '.claude', 'skills', 'data-workflow', 'workflows'),
    path.join(repo, '.qoder', 'skills', 'data-workflow', 'workflows'),
    path.join(repo, '.agents', 'skills', 'data-workflow', 'workflows'),
    path.join(repo, '.codex', 'skills', 'data-workflow', 'workflows'),
  ];
  if (includeHome) {
    roots.push(path.join(homedir(), '.agents', 'skills', 'data-workflow', 'workflows'));
  }
  return roots.map((root) => path.join(root, file));
}

const workflowCache = new Map();

/** Read one installed workflow definition, with an mtime cache. */
async function readWorkflowFile(repo, workflow, includeHome = true) {
  const id = normalizeWorkflow(workflow);
  if (!id) return null;
  for (const file of workflowCandidates(repo, id, includeHome)) {
    try {
      const info = await stat(file);
      const cached = workflowCache.get(file);
      if (cached && cached.mtimeMs === info.mtimeMs) return cached.value;
      const document = JSON.parse(await readFile(file, 'utf8'));
      const stages = stagesFromWorkflow(document, id);
      if (!stages.length) continue;
      const value = {
        stages,
        kind: typeof document?.kind === 'string' ? document.kind : '',
        file,
      };
      workflowCache.set(file, { mtimeMs: info.mtimeMs, value });
      return value;
    } catch {
      // Try the next installation location.
    }
  }
  return null;
}

/**
 * The workflow definitions installed for a repository.
 *
 * Used when a conversation has no run yet: the card can still show the whole
 * workflow shape instead of a blank "no run" box. Only definitions that
 * resolve to a real file are listed, so a repository without the skill does
 * not get a fabricated catalog.
 */
export async function catalogFromRepo(repo, options = {}) {
  const includeHome = options.homeFallback !== false;
  const out = [];
  for (const id of CATALOG_WORKFLOWS) {
    const found = await readWorkflowFile(repo, id, includeHome);
    if (!found) continue;
    out.push({
      id,
      kind: found.kind,
      stages: found.stages.map((stage) => ({
        id: stage.id,
        name: stage.name,
        requires: stage.requires.length,
        artifacts: stage.artifacts.length,
        code: stage.code,
        gateCheck: Boolean(stage.gateCheck),
      })),
    });
  }
  return out;
}

function comparable(view) {
  return JSON.stringify({
    sessionId: view.sessionId,
    repo: view.repo,
    requestedFeature: view.requestedFeature,
    feature: view.feature,
    workflow: view.workflow,
    stage: view.stage,
    status: view.status,
    question: view.question,
    association: view.association,
    runState: view.runState,
    active: view.active,
    stages: view.stages,
    gate: view.gate,
    stageGates: view.stageGates,
    catalog: view.catalog,
  });
}

function emptyState(sessionId = '') {
  return {
    type: 'state',
    sessionId,
    repo: '',
    repoName: '',
    requestedFeature: '',
    feature: '',
    workflow: '',
    stage: 0,
    status: '',
    question: '',
    rawRequest: '',
    stages: [],
    stageGates: [],
    catalog: [],
    association: 'none',
    runState: 'none',
    gate: null,
    active: false,
    updatedAt: Date.now(),
  };
}

function safeEvidence(row) {
  if (!row) return null;
  return {
    stage: Number(row.stage) || 0,
    requires: row.requires || '',
    gateCheck: row.gate_check || '',
    verify: row.verify || '',
    head: row.head || '',
    at: row.at || '',
    line: row.line || '',
  };
}

export async function analyzeGates(repo, feature, run, stages, runState) {
  const specDir = path.join(repo, 'specs', feature);
  const docs = await Promise.all(
    ['spec.md', 'plan.md', 'decision.md', 'check.md'].map(async (fileName) => {
      const rel = path.relative(repo, path.join(specDir, fileName));
      return { path: rel, text: (await readArtifact(repo, rel)).text };
    }),
  );
  const [evidenceText, verifyText, configText, passesText] = await Promise.all([
    readIf(path.join(specDir, 'evidence.log')),
    readIf(path.join(specDir, 'verify.log')),
    readRepoConfig(repo),
    readIf(path.join(repo, 'specs', 'passes.log')),
  ]);

  const rows = parseEvidenceRows(evidenceText);
  const currentId = Number(run?.stage) || 0;
  const verifyExit = parseExit(verifyText);
  const verifyCmd = verifyCommand(verifyText);
  const protectedPaths = configList(configText, 'protected_paths');
  const waiverGlobs = configList(configText, 'waiver_globs');
  const passes = passesText
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith('#'));

  const allOpenDocs = new Set(
    docs.filter((doc) => hasOpenDecision(doc.text)).map((doc) => doc.path),
  );
  const stageGates = [];

  for (let index = 0; index < (stages || []).length; index += 1) {
    const item = stages[index] || {};
    const id = Number(item.id) || index + 1;
    const artifactPaths = stageArtifacts(item, feature);
    const artifacts = await Promise.all(
      artifactPaths.map((rel) => readArtifact(repo, rel)),
    );
    const artifactText = artifacts.map((entry) => entry.text).join('\n');
    const requires = (item.requires || []).map((label) => ({
      label,
      found: artifactText.includes(label),
    }));
    const missingArtifacts = artifacts.filter((entry) => !entry.exists);
    const missingRequires = requires.filter((entry) => !entry.found);
    const openDocs = artifacts
      .filter((entry) => hasOpenDecision(entry.text))
      .map((entry) => entry.path);
    for (const docPath of openDocs) allOpenDocs.add(docPath);

    const row = rows
      .filter((entry) => Number(entry.stage) === id)
      .slice(-1)[0] || null;
    const evidence = safeEvidence(row);
    const gateCheck = item.gateCheck || '';
    const evidenceBad = Boolean(
      evidence &&
        ((evidence.verify && !['none', 'exit0'].includes(evidence.verify)) ||
          (evidence.gateCheck && !['none', 'ok'].includes(evidence.gateCheck))),
    );
    const evidencePassed = Boolean(
      evidence &&
        evidence.requires === 'ok' &&
        (!gateCheck || evidence.gateCheck === 'ok'),
    );
    const isCurrent = id === currentId;
    const isPast = id < currentId;
    const isFuture = id > currentId;

    let tone = 'pending';
    let label = '未开始';
    let reason = '';

    if (isFuture) {
      // A not-yet-reached stage has no artifacts by definition. Reporting it as
      // "missing artifacts" would paint the whole remaining workflow red.
      tone = 'pending';
      label = '未开始';
      reason = '等待前序阶段完成';
    } else if (isCurrent && runState === 'active' && openDocs.length) {
      tone = 'open';
      label = '待拍板';
      reason = `${openDocs.join('、')} 里还有未决问题`;
    } else if (missingArtifacts.length) {
      tone = isCurrent && runState === 'active' ? 'waiting' : 'fail';
      label = isCurrent && runState === 'active' ? '待产出' : '缺产物';
      reason = `缺产物 ${missingArtifacts.map((entry) => entry.path).join('、')}`;
    } else if (missingRequires.length) {
      tone = 'fail';
      label = '缺必填项';
      reason = `缺 ${missingRequires.map((entry) => entry.label).join('、')}`;
    } else if (evidenceBad) {
      tone = 'fail';
      label = '门禁未过';
      reason = `evidence 记录 requires=${evidence.requires} gate_check=${evidence.gateCheck} verify=${evidence.verify}`;
    } else if (evidencePassed) {
      tone = isCurrent && runState === 'active' ? 'active' : 'pass';
      label = isCurrent
        ? runState === 'active'
          ? '阶段门已过'
          : '已留证据'
        : '阶段已过';
      reason = 'evidence 记录通过';
    } else if (evidence) {
      tone = 'pass';
      label = '已留证据';
      reason = evidence.line;
    } else if (isCurrent && runState === 'active') {
      if (item.code) {
        tone = 'working';
        label = '待 done';
        reason = '改完后运行 done；会检查工作树是否真的动过，并跑仓库的 verify';
      } else if (gateCheck) {
        tone = 'unverified';
        label = '待门禁';
        reason = '本阶段要求 gate_check，evidence 里还没有 ok';
      } else {
        tone = 'ready';
        label = '待推进';
        reason = '产物齐全；运行 next 推进并记录 evidence';
      }
    } else if (isCurrent && runState === 'finished') {
      tone = 'unverified';
      label = '无证据';
      reason = '运行完成了，但本阶段没有 evidence 行';
    } else if (isPast) {
      tone = 'unverified';
      label = '无证据';
      reason = '阶段已过但没有 evidence 行';
    }

    if (runState === 'abandoned') {
      tone = 'none';
      label = '已放弃';
      reason = '这次运行不再注入，也不再作为当前任务';
    }

    stageGates.push({
      id,
      name: typeof item.name === 'string' ? item.name : `阶段 ${index + 1}`,
      code: item.code === true,
      gateCheck,
      tone,
      label,
      reason,
      requires,
      artifacts: artifacts.map((entry) => ({
        path: entry.path,
        exists: entry.exists,
      })),
      // A future stage has not been asked to produce anything yet, so its
      // missing lists stay empty; only reached stages report missing items.
      missingArtifacts: isFuture ? [] : missingArtifacts.map((entry) => entry.path),
      missingRequires: isFuture ? [] : missingRequires.map((entry) => entry.label),
      openDecision: openDocs.length > 0,
      openDocs,
      evidence,
    });
  }

  const current = stageGates.find((entry) => entry.id === currentId) || null;
  const stage = (stages || []).find((item) => Number(item.id) === currentId) || null;
  const openDocs = [...allOpenDocs];
  const openDecision = openDocs.length > 0;
  const evidence = current?.evidence || null;
  const artifacts = current?.artifacts || [];
  const requires = current?.requires || [];
  const missingArtifacts = current?.missingArtifacts || [];
  const missingRequires = current?.missingRequires || [];
  const declaredGate = Boolean(current?.gateCheck);
  const lastEvidence = safeEvidence(rows.slice(-1)[0]);

  // Durable gate history: the runner only re-emits live verdicts, but every
  // passed stage leaves an evidence row. Rebuilding it here keeps the gate log
  // meaningful after a page reload or a Host restart.
  const history = rows.map((row) => {
    const entry = safeEvidence(row);
    const bad =
      (entry.verify && !['none', 'exit0'].includes(entry.verify)) ||
      (entry.gateCheck && !['none', 'ok'].includes(entry.gateCheck));
    return {
      kind: bad ? 'FAIL' : 'PASS',
      stageId: entry.stage,
      detail: `STAGE ${entry.stage} · requires=${entry.requires} gate_check=${entry.gateCheck} verify=${entry.verify}`,
      line: entry.line,
      at: entry.at,
    };
  });

  let tone = 'none';
  let label = '无运行';
  let reason = '';
  if (current) {
    tone = current.tone === 'active' ? 'pass' : current.tone;
    label = current.label;
    reason = current.reason;
    if (tone === 'pending') {
      tone = 'unverified';
      label = '无证据';
      reason = '运行里没有可判定的阶段记录';
    }
  } else if (runState === 'abandoned') {
    label = '已放弃';
  } else {
    tone = 'unverified';
    label = '阶段记录缺失';
    reason = 'run.json 的 stage 不在当前工作流的阶段表里';
  }

  const NEXT_BY_TONE = {
    open: '回答问题后运行 answer，再运行 next',
    waiting: '把缺失的产物写进 specs 目录，然后运行 next',
    fail: '补齐缺失内容或修正 evidence 后重新运行 next / done',
    unverified: '运行 next 或 done，让 runner 记录 evidence',
    ready: '运行 next 推进到下一阶段',
    working: '改完后运行 done：会检查工作树并跑仓库 verify',
    pass: '本阶段已过，运行 next 继续',
    active: '本阶段已过，运行 next 继续',
    none: '',
  };
  let next = '';
  if (runState === 'finished') {
    next = '运行已结束；需要新工作时运行 start 开启下一次';
  } else if (runState === 'abandoned') {
    next = '';
  } else {
    next = NEXT_BY_TONE[tone] || '';
  }

  return {
    state: tone,
    label,
    reason,
    next,
    stageId: currentId,
    stageName: current?.name || stage?.name || '',
    artifacts,
    requires,
    openDecision,
    openDocs,
    evidence,
    lastEvidence,
    verify: { exit: verifyExit, cmd: verifyCmd },
    gateCheckDeclared: declaredGate,
    protection: {
      configured: protectedPaths.length > 0 || waiverGlobs.length > 0,
      protectedPaths,
      waiverGlobs,
      passesCount: passes.length,
      passes: passes.slice(-2),
    },
    history,
    stageGates,
  };
}

function mount(ctx) {
  const connections = new Set();
  const pendingCalls = new Map();
  const states = new Map();
  const verdicts = new Map();
  const sessionFeatures = new Map();
  const refreshAt = new Map();
  const refreshTimers = new Map();
  let latestSessionId = '';

  const broadcast = (frame) => {
    const payload = sse(frame);
    for (const response of connections) {
      try {
        response.write(payload);
      } catch {
        // The close handler removes dead responses.
      }
    }
  };

  const broadcastSessionData = (sessionId) => {
    broadcast({
      type: 'verdicts',
      sessionId,
      items: verdicts.get(sessionId) || [],
    });
  };

  function rememberVerdict(sessionId, verdict) {
    if (!sessionId || !verdict) return;
    const list = verdicts.get(sessionId) || [];
    list.push(verdict);
    verdicts.set(sessionId, list.slice(-MAX_VERDICTS));
  }

  function repoOf(session) {
    try {
      const directory = ctx.get('workingDirectory');
      const resolved = directory?.get?.(session);
      if (typeof resolved === 'string' && resolved) return resolved;
    } catch {
      // Fall through to the recorded cwd.
    }
    const cwd = session?.header?.cwd;
    if (typeof cwd === 'string' && cwd) return cwd;
    try {
      const directory = ctx.get('workingDirectory');
      if (typeof directory?.defaultDirectory === 'string') return directory.defaultDirectory;
    } catch {
      // Fall through.
    }
    return process.cwd();
  }

  async function loadStages(repo, workflow) {
    const id = normalizeWorkflow(workflow);
    if (!id) return { stages: [], kind: '', source: 'none' };
    const found = await readWorkflowFile(repo, id);
    if (found) return { stages: found.stages, kind: found.kind, source: 'file' };
    return { stages: stagesFromWorkflow(null, id), kind: '', source: 'fallback' };
  }

  function featureFromSessionLog(session, sessionId) {
    try {
      const end = Number(session?.seq) || 0;
      const start = Math.max(0, end - 500);
      const events = session.snapshotEvents(start);
      const found = featureFromEvents(events);
      if (found) sessionFeatures.set(sessionId, found);
      return found;
    } catch {
      return '';
    }
  }

  async function refresh(session, sessionId) {
    const repo = repoOf(session);
    const requestedFeature =
      sessionFeatures.get(sessionId) || featureFromSessionLog(session, sessionId);
    const records = await scanRuns(repo);
    // Strictly session-scoped: another conversation's run must never appear
    // here, even while it is still open.
    const selected = selectRunRecord(records, requestedFeature);
    const run = selected?.record || null;
    const workflow = run ? normalizeWorkflow(run.workflow) : '';
    const loaded = run
      ? await loadStages(repo, workflow)
      : { stages: [], kind: '' };
    const status = run ? String(run.status || '') : '';
    const runState = run ? runStateOf(run) : 'none';
    const association = selected?.association || (requestedFeature ? 'missing' : 'none');
    const gate = run
      ? await analyzeGates(repo, run.feature, run, loaded.stages, runState)
      : null;
    // Without a run, still show what this repository can do: every installed
    // workflow definition and its stage chain.
    const catalog = run ? [] : await catalogFromRepo(repo);

    const view = {
      type: 'state',
      sessionId,
      repo,
      repoName: path.basename(repo) || repo,
      requestedFeature,
      feature: run ? String(run.feature || '') : '',
      workflow,
      stage: Number(run?.stage) || 0,
      status,
      question: typeof run?.question === 'string' ? run.question : '',
      rawRequest: typeof run?.raw_request === 'string' ? run.raw_request : '',
      stages: loaded.stages,
      stageGates: gate?.stageGates || [],
      catalog,
      association,
      runState,
      gate,
      active: ACTIVE_STATUSES.has(status),
      updatedAt: Date.now(),
    };

    // A refresh that started before the user switched conversations must not
    // publish its result over the newer conversation's card.
    if (latestSessionId !== sessionId) return view;
    const previous = states.get(sessionId);
    states.set(sessionId, view);
    if (!previous || comparable(previous) !== comparable(view)) broadcast(view);
    return view;
  }

  function scheduleRefresh(session, sessionId, delay) {
    const now = Date.now();
    const last = refreshAt.get(sessionId) || 0;
    const wait = Number.isFinite(delay) ? delay : REFRESH_MS;
    if (now - last >= wait) {
      refreshAt.set(sessionId, now);
      void refresh(session, sessionId).catch(() => {});
      return;
    }
    if (refreshTimers.has(sessionId)) return;
    const timer = setTimeout(() => {
      refreshTimers.delete(sessionId);
      refreshAt.set(sessionId, Date.now());
      void refresh(session, sessionId).catch(() => {});
    }, wait - (now - last));
    timer.unref?.();
    refreshTimers.set(sessionId, timer);
  }

  function activate(session, sessionId) {
    if (latestSessionId !== sessionId) {
      latestSessionId = sessionId;
      broadcastSessionData(sessionId);
    }
  }

  async function prime() {
    if (latestSessionId) return;
    let live = [];
    try {
      live = ctx.sessions.list();
    } catch {
      return;
    }
    const candidates = live.filter((session) => !isSubagent(session));
    const session = candidates[candidates.length - 1] || live[live.length - 1];
    if (!session) return;
    activate(session, session.id);
    await refresh(session, session.id);
  }

  const disposers = [];

  disposers.push(
    ctx.on(
      'session/created',
      (session) => {
        const sessionId = String(session?.id || '');
        if (!sessionId || isSubagent(session)) return;
        latestSessionId = sessionId;
        states.delete(sessionId);
        verdicts.delete(sessionId);
        const repo = repoOf(session);
        const view = {
          ...emptyState(sessionId),
          repo,
          repoName: path.basename(repo) || repo,
        };
        states.set(sessionId, view);
        broadcast(view);
        broadcastSessionData(sessionId);
        void refresh(session, sessionId).catch(() => {});
      },
      { global: true },
    ),
  );

  disposers.push(
    ctx.on(
      'session/event',
      (session, event) => {
        const sessionId = String(session?.id || '');
        if (!sessionId) return;
        if (isSubagent(session)) return;

        if (!latestSessionId) {
          latestSessionId = sessionId;
          broadcastSessionData(sessionId);
        }

        if (event.type === 'turn/start') {
          activate(session, sessionId);
          scheduleRefresh(session, sessionId, 0);
          return;
        }

        if (sessionId !== latestSessionId) return;

        if (event.type === 'tool/call') {
          const args = parseArgs(event.data?.arguments);
          const calledName = String(event.data?.name || 'tool');
          const callId = String(event.data?.callId || '');
          const feature = featureFromTool(calledName, args);
          if (feature) sessionFeatures.set(sessionId, feature);
          const gate = /workflow_next\.py|check_runner\.py|verify\.sh|git status/.test(
            String(args?.command || ''),
          );
          if (callId) {
            pendingCalls.set(`${sessionId}:${callId}`, {
              name: calledName,
              gate,
              startedAt: Date.now(),
            });
          }
          if (
            feature ||
            isWorkflowSignal(calledName, args) ||
            (calledName === 'skill' && String(args?.name || '').includes('data-workflow'))
          ) {
            scheduleRefresh(session, sessionId, 0);
          }
          return;
        }

        if (event.type === 'tool/result') {
          const callId = String(
            event.data?.message?.toolCallId || event.data?.message?.source?.callId || '',
          );
          const key = `${sessionId}:${callId}`;
          const entry = callId ? pendingCalls.get(key) : undefined;
          if (callId) pendingCalls.delete(key);

          const text = contentText(event.data?.message?.content);
          const fromRunner = entry?.name === 'bash';
          const outputFeature = fromRunner ? featureFromOutput(text) : '';
          if (outputFeature) sessionFeatures.set(sessionId, outputFeature);
          const verdict = fromRunner ? parseVerdict(text) : null;
          if (verdict) rememberVerdict(sessionId, verdict);

          if (verdict) {
            broadcast({
              type: 'verdicts',
              sessionId,
              items: verdicts.get(sessionId) || [],
            });
          }
          if (entry?.gate || outputFeature || verdict) {
            scheduleRefresh(session, sessionId, 0);
          }
          return;
        }

        if (event.type === 'turn/end') {
          scheduleRefresh(session, sessionId, 0);
        }
      },
      { global: true },
    ),
  );

  disposers.push(
    ctx.on(
      'session/disposed',
      (session) => {
        const sessionId = String(session?.id || '');
        states.delete(sessionId);
        verdicts.delete(sessionId);
        sessionFeatures.delete(sessionId);
        refreshAt.delete(sessionId);
        const timer = refreshTimers.get(sessionId);
        if (timer) clearTimeout(timer);
        refreshTimers.delete(sessionId);
        for (const key of pendingCalls.keys()) {
          if (key.startsWith(`${sessionId}:`)) pendingCalls.delete(key);
        }
        if (latestSessionId === sessionId) {
          latestSessionId = '';
          broadcast(emptyState(''));
          const timer2 = setTimeout(() => {
            void prime();
          }, 50);
          timer2.unref?.();
        }
      },
      { global: true },
    ),
  );

  ctx.effect(() => {
    const disposeRoute = ctx.webServer.register({
      kind: 'exact',
      path: EVENTS_ENDPOINT,
      handler: (req, res) => {
        if (req.method !== 'GET') {
          res.writeHead(405);
          res.end();
          return;
        }
        if (!isLoopback(req.socket?.remoteAddress)) {
          res.writeHead(403);
          res.end();
          return;
        }
        const origin = req.headers?.origin;
        if (origin) {
          let originHost = '';
          try {
            originHost = new URL(origin).host;
          } catch {
            // Malformed origins are rejected below.
          }
          if (!originHost || originHost !== req.headers.host) {
            res.writeHead(403);
            res.end();
            return;
          }
        }

        res.writeHead(200, {
          'content-type': 'text/event-stream; charset=utf-8',
          'cache-control': 'no-cache, no-transform',
          connection: 'keep-alive',
          'x-accel-buffering': 'no',
        });
        res.write(': workflow-hud connected\n\n');
        res.write(sse({ type: 'hello', version: 3 }));

        const current = latestSessionId ? states.get(latestSessionId) : null;
        res.write(sse(current || emptyState('')));
        if (latestSessionId) {
          res.write(
            sse({
              type: 'verdicts',
              sessionId: latestSessionId,
              items: verdicts.get(latestSessionId) || [],
            }),
          );
        }
        connections.add(res);
        res.on('close', () => {
          connections.delete(res);
        });
        if (!current) void prime();
      },
    });

    const keepalive = setInterval(() => {
      for (const response of connections) {
        try {
          response.write(': keepalive\n\n');
        } catch {
          // Ignored; the response closes on its own.
        }
      }
    }, KEEPALIVE_MS);
    keepalive.unref?.();

    void prime();

    return () => {
      clearInterval(keepalive);
      disposeRoute();
      for (const response of connections) {
        try {
          response.end();
        } catch {
          // Already closed.
        }
      }
      connections.clear();
      for (const timer of refreshTimers.values()) clearTimeout(timer);
      refreshTimers.clear();
      for (const dispose of disposers) {
        try {
          dispose();
        } catch {
          // Already disposed with the fiber.
        }
      }
      disposers.length = 0;
    };
  }, 'workflow-hud: SSE channel');
}

export function apply(ctx) {
  if (typeof ctx.inject === 'function') {
    ctx.inject(['webServer'], (serviceCtx) => mount(serviceCtx));
    return;
  }
  mount(ctx);
}
