/**
 * Pure helpers for the workflow HUD Host plugin.
 *
 * Nothing in this module touches the filesystem, Cordis, or a session, so it
 * can be tested directly with `node --test`.
 */

/** Stage names used when the repository has no data-workflow workflow file. */
export const FALLBACK_STAGES = Object.freeze({
  change: ['写下决定', '按决定改', '核对'],
  feature: [
    '需求与负债评估',
    '影响面评估',
    '技术方案',
    '编码实现（Go 栈）',
    '改动审核（三轴）',
    '质量与接口门',
    '文档审计与提交',
  ],
  bugfix: [
    '根因定位（只读）',
    '修复影响面',
    '回归测试先行',
    '改动审核（三轴）',
    '质量与接口门',
    '知识回写与提交',
  ],
  hotfix: [
    '止血决策（只读，5 分钟内出结论）',
    '回滚预案（先于任何代码改动）',
    '最小修复',
    '必过门（不可跳过）',
    '提交与记账',
  ],
  'review-only': ['固定 diff 与指纹', '三轴并行审核', '接口影响取证', '合并裁决'],
});

const WORKFLOW_STATUSES = new Set(['working', 'waiting', 'done', 'abandoned']);

/** Parse a tool call's JSON string while tolerating an already-parsed object. */
export function parseArgs(raw) {
  if (raw && typeof raw === 'object' && !Array.isArray(raw)) return raw;
  if (typeof raw !== 'string' || raw.trim() === '') return {};
  try {
    const value = JSON.parse(raw);
    return value && typeof value === 'object' && !Array.isArray(value) ? value : {};
  } catch {
    return {};
  }
}

/** Keep the last two path segments so traces stay readable inside the card. */
export function shortPath(value) {
  if (typeof value !== 'string' || value === '') return '';
  const parts = value.split(/[\\/]+/).filter(Boolean);
  if (parts.length <= 2) return parts.join('/');
  return parts.slice(-2).join('/');
}

const SECRET_PATTERNS = [
  [/sk-[A-Za-z0-9_-]{8,}/g, 'sk-***'],
  [/(Bearer\s+)[A-Za-z0-9._~+/-]{8,}=*/g, '$1***'],
  [/gh[pousr]_[A-Za-z0-9]{8,}/g, 'gh-***'],
];

/** Remove obvious credentials before a command or argument reaches the page. */
export function redact(value) {
  if (typeof value !== 'string' || value === '') return '';
  let text = value;
  for (const [pattern, replacement] of SECRET_PATTERNS) text = text.replace(pattern, replacement);
  return text;
}

function oneLine(value, limit = 92) {
  if (typeof value !== 'string') return '';
  const text = redact(value).replace(/\s+/g, ' ').trim();
  if (text.length <= limit) return text;
  return `${text.slice(0, Math.max(0, limit - 1))}…`;
}

function firstString(value) {
  if (typeof value === 'string' && value) return value;
  if (Array.isArray(value)) {
    for (const item of value) {
      if (typeof item === 'string' && item) return item;
    }
  }
  return '';
}

/** A short, human-readable argument summary for the live trace. */
export function toolDetail(name, args = {}) {
  const a = args && typeof args === 'object' ? args : {};
  switch (name) {
    case 'bash':
      return oneLine(firstString(a.command), 96);
    case 'read':
    case 'read_image':
    case 'edit':
    case 'write':
      return shortPath(firstString(a.file_path) || firstString(a.path));
    case 'glob':
      return oneLine(firstString(a.pattern));
    case 'grep': {
      const pattern = oneLine(firstString(a.pattern), 56);
      const where = shortPath(firstString(a.path));
      return where ? `${where} · ${pattern}` : pattern;
    }
    case 'skill':
      return oneLine(firstString(a.name) || firstString(a.skill));
    case 'web_search': {
      const queries = Array.isArray(a.queries) ? a.queries : [a.query];
      return oneLine(queries.filter((q) => typeof q === 'string').join(' · '));
    }
    case 'web_fetch': {
      const raw = firstString(a.url);
      try {
        const url = new URL(raw);
        return `${url.host}${url.pathname}`;
      } catch {
        return oneLine(raw);
      }
    }
    case 'present': {
      const first = Array.isArray(a.files) ? a.files[0] : null;
      return first ? shortPath(firstString(first.path)) : '';
    }
    case 'todo_write':
    case 'todo': {
      const todos = Array.isArray(a.todos) ? a.todos : [];
      const doing = todos.find((todo) => todo && todo.status === 'in_progress');
      return doing && typeof doing.content === 'string'
        ? oneLine(doing.content, 72)
        : `${todos.length} 项`;
    }
    case 'subagent':
    case 'subagent_fork':
      return oneLine(firstString(a.description) || firstString(a.prompt), 76);
    case 'workflow':
      return oneLine(firstString(a.meta && a.meta.name) || firstString(a.name));
    case 'ask_user_question':
      return '等待用户输入';
    default: {
      for (const value of Object.values(a)) {
        if (typeof value === 'string' && value) return oneLine(value, 76);
      }
      return '';
    }
  }
}

/** Coarse category used for the trace glyph and colour. */
export function toolKind(name) {
  switch (name) {
    case 'read':
    case 'read_image':
      return 'read';
    case 'edit':
    case 'write':
      return 'write';
    case 'grep':
    case 'glob':
      return 'search';
    case 'bash':
      return 'exec';
    case 'web_search':
    case 'web_fetch':
      return 'net';
    case 'skill':
      return 'skill';
    case 'todo_write':
    case 'present':
      return 'plan';
    default:
      return 'other';
  }
}

/**
 * Whether this call can reveal or change workflow state, meaning the host
 * should re-read the run immediately instead of waiting for the next tick.
 */
export function isWorkflowSignal(name, argsOrText) {
  const text =
    typeof argsOrText === 'string'
      ? argsOrText
      : JSON.stringify(argsOrText && typeof argsOrText === 'object' ? argsOrText : {});
  if (name === 'skill' && /data-workflow/.test(text)) return true;
  if (name === 'bash' && /workflow_next\.py|run\.json|specs\//.test(text)) return true;
  if (name === 'read' || name === 'edit' || name === 'write') {
    if (/run\.json|[\\/]specs[\\/]/.test(text)) return true;
  }
  return false;
}

/** Normalize a workflow id read from run.json; unknown ids keep the raw value. */
export function normalizeWorkflow(value) {
  if (typeof value !== 'string') return '';
  const id = value.trim().toLowerCase();
  return id === 'review_only' ? 'review-only' : id;
}

/** A feature name the runner would accept; anything else becomes empty. */
export function normalizeFeature(value) {
  if (typeof value !== 'string') return '';
  const id = value.trim();
  return /^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(id) ? id : '';
}

/** Whether a run still has work in progress. */
export function isOpenRun(record) {
  const status = String(record?.status || '');
  return status === 'working' || status === 'waiting';
}

/** Normalize a run's status into the HUD's three display states. */
export function runStateOf(record) {
  const status = String(record?.status || '');
  if (status === 'working' || status === 'waiting') return 'active';
  if (status === 'done') return 'finished';
  if (status === 'abandoned') return 'abandoned';
  return 'unknown';
}

function byMtimeDesc(a, b) {
  return (Number(b?.mtimeMs) || 0) - (Number(a?.mtimeMs) || 0);
}

/**
 * Pick the run this session should display.
 *
 * A run named by this session wins, even after it finishes, so the card can
 * show the history of the run the user actually started. Without a session
 * association nothing qualifies by default: a finished historical run must
 * never impersonate the current conversation.
 *
 * `mode: 'open'` and `mode: 'history'` are explicit alternatives for a caller
 * that wants a repository-wide view. The HUD's default path does not use them.
 */
export function selectRunRecord(records, feature = '', mode = 'session') {
  const list = (Array.isArray(records) ? records : []).filter(
    (record) => record && typeof record === 'object',
  );
  const wanted = normalizeFeature(feature);
  if (wanted) {
    const own = list
      .filter((record) => String(record.feature || '') === wanted)
      .sort(byMtimeDesc)[0];
    if (own) {
      return { record: own, runState: runStateOf(own), association: 'session' };
    }
    // The session named a feature whose record is gone. Do not substitute an
    // unrelated run; an empty card is more honest than a wrong one.
    return null;
  }
  if (mode === 'open') {
    const open = list.filter(isOpenRun).sort(byMtimeDesc)[0];
    if (open) return { record: open, runState: 'active', association: 'repo' };
    return null;
  }
  if (mode === 'history') {
    const newest = [...list].sort(byMtimeDesc)[0];
    if (newest) {
      return { record: newest, runState: runStateOf(newest), association: 'history' };
    }
  }
  return null;
}

/**
 * Strong session-to-feature evidence.
 *
 * Reads are intentionally excluded: browsing another run's files must not
 * claim that run as the current conversation's work. Only runner commands and
 * writes under `specs/<feature>/` count.
 */
export function featureFromTool(name, args = {}) {
  const a = args && typeof args === 'object' ? args : {};
  if (name === 'bash') {
    const command = String(a.command || '');
    const match = command.match(
      /workflow_next\.py\s+(?:start\s+\S+\s+|(?:next|done|answer|wait|approve|abandon|show|check)\s+)([A-Za-z0-9][A-Za-z0-9._-]*)/,
    );
    return normalizeFeature(match ? match[1] : '');
  }
  if (name === 'write' || name === 'edit' || name === 'apply_patch') {
    const direct = String(a.file_path || a.path || a.filePath || '');
    const fromPath = direct.match(/(?:^|[\\/])specs[\\/]([A-Za-z0-9][A-Za-z0-9._-]*)[\\/]/);
    if (fromPath) return normalizeFeature(fromPath[1]);
    const patch = String(a.patch || a.input || a.diff || '');
    const fromPatch = patch.match(/specs[\\/]([A-Za-z0-9][A-Za-z0-9._-]*)[\\/]/);
    if (fromPatch) return normalizeFeature(fromPatch[1]);
  }
  return '';
}

/**
 * Feature named by the runner's own output.
 *
 * `RUN <feature> <workflow> <status>` and `RECORD specs/<feature>/run.json`
 * are stable runner lines, so they associate a session even when the command
 * itself omitted the feature (for example a bare `next --repo ...`).
 */
export function featureFromOutput(text) {
  const source = String(text || '');
  // Require a real runner RUN line: feature, a known workflow id, and a known
  // status. Free-form text that merely starts with "RUN" must not associate.
  const run = source.match(
    /^\s*RUN\s+([A-Za-z0-9][A-Za-z0-9._-]*)\s+(change|feature|bugfix|hotfix|review-only)\s+(working|waiting|done|abandoned)\b/m,
  );
  if (run) return normalizeFeature(run[1]);
  // Only the runner's own RECORD line counts. A generic `ls specs/x/run.json`
  // or a `cat` of the file must never associate the session with that run.
  const record = source.match(
    /^\s*RECORD\s+specs[\\/]([A-Za-z0-9][A-Za-z0-9._-]*)[\\/]run\.json/m,
  );
  return normalizeFeature(record ? record[1] : '');
}

/**
 * Recover the session's feature association from its own durable log.
 *
 * Live `session/event` listeners miss everything that happened before this
 * process attached (page reload, HUD install, Host restart). Prefer the newest
 * evidence so a session that started a second run reports that run, not the
 * first one. Reads are ignored, matching `featureFromTool`.
 */
export function featureFromEvents(events) {
  const list = Array.isArray(events) ? events : [];
  for (let index = list.length - 1; index >= 0; index -= 1) {
    const event = list[index];
    if (!event || typeof event !== 'object') continue;
    if (event.type === 'tool/call') {
      const found = featureFromTool(
        String(event.data?.name || ''),
        parseArgs(event.data?.arguments),
      );
      if (found) return found;
      continue;
    }
    if (event.type === 'tool/result') {
      const found = featureFromOutput(contentText(event.data?.message?.content));
      if (found) return found;
    }
  }
  return '';
}

/** Flatten a tool-result content array into plain text. */
export function contentText(content) {
  if (!Array.isArray(content)) return '';
  return content
    .map((block) =>
      block && block.type === 'text' && typeof block.text === 'string' ? block.text : '',
    )
    .filter(Boolean)
    .join('\n');
}

function fieldsOf(line) {
  const fields = {};
  for (const part of line.split(/\s+/)) {
    const eq = part.indexOf('=');
    if (eq > 0) fields[part.slice(0, eq)] = part.slice(eq + 1);
  }
  return fields;
}

/** Parse every runner evidence row so one stage can be looked up directly. */
export function parseEvidenceRows(text) {
  return String(text || '')
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => ({ line, ...fieldsOf(line) }));
}

/** Parse the last runner evidence row into fields. */
export function parseEvidence(text) {
  const rows = parseEvidenceRows(text);
  const last = rows[rows.length - 1];
  return last || { line: '' };
}

/** Read the exit code from the first `exit N` line of verify.log. */
export function parseExit(text) {
  const match = String(text || '').match(/^\s*exit\s+(-?\d+)/m);
  return match ? Number(match[1]) : null;
}

/** The repository's own verify command from verify.log, when recorded. */
export function verifyCommand(text) {
  const match = String(text || '').match(/^\s*cmd\s+(.+)$/m);
  return match ? match[1].trim() : '';
}

/** Whether a decision/check document still has an unanswered 待拍板 marker. */
export function hasOpenDecision(text) {
  const source = String(text || '');
  const open = /^\s*(?:[-*>]\s*)?待拍板/gm;
  const closed = /^\s*(?:[-*>]\s*)?已拍板[：:]/gm;
  let openAt = -1;
  let closedAt = -1;
  let match;
  while ((match = open.exec(source))) openAt = match.index;
  while ((match = closed.exec(source))) closedAt = match.index;
  return openAt >= 0 && openAt > closedAt;
}

/**
 * Turn one stage artifact template into concrete repo-relative files.
 * Mirrors `artifact_paths()` in skills/data-workflow/scripts/workflow_next.py.
 */
export function stageArtifacts(stage, feature = '') {
  const items = Array.isArray(stage?.artifacts) ? stage.artifacts : [];
  const out = [];
  const id = normalizeFeature(feature);
  for (const item of items) {
    if (typeof item !== 'string' || !item) continue;
    let text = item.replaceAll('<feature>', id).replaceAll('<artifacts_root>', `specs/${id}`);
    text = text.split('（')[0].trim();
    if (!text || text.startsWith('$')) continue;
    if (text.includes(' ')) text = text.split(/\s+/)[0];
    if (/[^\x00-\x7F]/.test(text)) continue;
    if (text.startsWith('specs/')) {
      if (!out.includes(text)) out.push(text);
      continue;
    }
    if (/[*?]/.test(text)) continue;
    if (/^[\w./~-]+$/.test(text) && /\.(md|patch)$/.test(text)) {
      if (!id) continue;
      const rel = `specs/${id}/${text.replace(/^\/+/, '')}`;
      if (!out.includes(rel)) out.push(rel);
    }
  }
  return out;
}

/** Read one scalar from a simple `key: value` YAML file without a YAML parser. */
export function configScalar(text, key) {
  if (typeof text !== 'string' || !key) return '';
  const match = text.match(new RegExp(`^\\s*${key}\\s*:\\s*(.*)$`, 'm'));
  if (!match) return '';
  // Strip a trailing comment before unquoting so `"x" # note` yields `x`.
  let value = match[1].replace(/\s+#.*$/, '').trim();
  if (
    value.length >= 2 &&
    value[0] === value[value.length - 1] &&
    (value[0] === '"' || value[0] === "'")
  ) {
    value = value.slice(1, -1);
  }
  return value.trim();
}

/** Split a comma-separated config value into non-empty entries. */
export function configList(text, key) {
  const value = configScalar(text, key);
  if (!value) return [];
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
}

/**
 * Parse the first runner verdict line from a command's output.
 * The runner prints exactly one of READ/NEED/WAIT/OPEN/DO/DONE/GATE/...
 */
const VERDICT_PRIORITY = [
  'GATE',
  'OPEN',
  'NEED_ANSWER',
  'NEED',
  'WAIT',
  'DO',
  'DONE',
  'RED',
  'BLOCK',
  'MISMATCH',
  'NO_RUN',
  'LAST',
  'ALREADY',
  'READ',
];

export function parseVerdict(text) {
  const lines = String(text || '')
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  const markers = [];
  for (const line of lines) {
    const match = line.match(
      /^(NEED_ANSWER|READ|NEED|WAIT|OPEN|DO|DONE|GATE|LAST|ALREADY|RED|NO_RUN|BLOCK|MISMATCH)\b/,
    );
    if (match) markers.push(match[1]);
  }
  if (!markers.length) return null;

  // A `next` on a stage with missing files prints READ first and NEED after it.
  // Prefer the diagnosis (GATE/OPEN/NEED/WAIT) over the generic instruction.
  const kind = VERDICT_PRIORITY.find((item) => markers.includes(item)) || markers[0];
  const stage = lines.find((line) => /^STAGE\s+/.test(line)) || '';
  const missing = lines.find((line) => line.startsWith('缺：')) || '';
  const needPaths = [
    ...new Set(
      lines
        .filter((line) => /^NEED\s+/.test(line))
        .map((line) => line.replace(/^NEED\s+/, '')),
    ),
  ];
  const check = lines.find((line) => /^CHECK\s+/.test(line)) || '';
  const reply = lines.find((line) => /^REPLY\s+/.test(line)) || '';
  const question = lines.find((line) => /^QUESTION\s+/.test(line)) || '';
  const verifyLine = lines.find((line) => /^VERIFY\s+/.test(line)) || '';
  const doneLine = lines.find((line) => /^DONE\s+/.test(line)) || '';
  const detail = [
    stage,
    missing,
    needPaths.length ? `NEED ${needPaths.join('、')}` : '',
    check,
    verifyLine,
    question,
    reply,
    kind === 'DONE' || kind === 'LAST' ? doneLine : '',
  ]
    .filter(Boolean)
    .join(' · ')
    .slice(0, 320);
  return { kind, detail, at: Date.now() };
}

/** Translate a workflow JSON file into the HUD's stage rows. */
export function stagesFromWorkflow(document, workflow = '') {
  if (document && Array.isArray(document.stages)) {
    return document.stages
      .filter((stage) => stage && typeof stage === 'object')
      .map((stage, index) => ({
        id: Number.isFinite(Number(stage.id)) ? Number(stage.id) : index + 1,
        name: typeof stage.name === 'string' && stage.name ? stage.name : `阶段 ${index + 1}`,
        requires: Array.isArray(stage.requires)
          ? stage.requires.filter((item) => typeof item === 'string')
          : [],
        artifacts: Array.isArray(stage.artifacts)
          ? stage.artifacts.filter((item) => typeof item === 'string')
          : [],
        code: stage.code === true,
        gateCheck: typeof stage.gate_check === 'string' ? stage.gate_check : '',
      }));
  }
  const fallback = FALLBACK_STAGES[normalizeWorkflow(workflow)] || [];
  return fallback.map((name, index) => ({
    id: index + 1,
    name,
    requires: [],
    artifacts: [],
    code: false,
    gateCheck: '',
  }));
}

export { WORKFLOW_STATUSES };
