/**
 * Cyber Web Mascot — pure core (no DOM, no timers, no canvas).
 *
 * The demo scene imports this module; the single-file build inlines it. Every
 * class here is deterministic enough to be tested in Node (see test/core.test.mjs).
 */

export const TAU = Math.PI * 2;

export const clamp = (value, min, max) => {
  const number = Number(value);
  if (!Number.isFinite(number)) return Number.isFinite(min) ? min : 0;
  return Math.min(max, Math.max(min, number));
};

export const lerp = (a, b, t) => a + (b - a) * t;

const round4 = (value) => Math.round(value * 1e4) / 1e4;

export function formatClock(ms) {
  const total = Math.max(0, Math.floor(Number(ms) || 0));
  const minutes = Math.floor(total / 60000);
  const seconds = Math.floor((total % 60000) / 1000);
  const tenths = Math.floor((total % 1000) / 100);
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}.${tenths}`;
}

/**
 * Minimal wildcard event bus.
 * Handlers receive `(payload, type, originalEvent)`.
 */
export class EventBus {
  constructor() {
    this.handlers = new Map();
  }

  on(type, handler) {
    if (typeof type !== 'string' || typeof handler !== 'function') return () => {};
    const list = this.handlers.get(type) || [];
    list.push(handler);
    this.handlers.set(type, list);
    return () => this.off(type, handler);
  }

  off(type, handler) {
    const list = this.handlers.get(type);
    if (!list) return;
    const rest = list.filter((item) => item !== handler);
    if (rest.length) this.handlers.set(type, rest);
    else this.handlers.delete(type);
  }

  emit(type, payload) {
    let count = 0;
    const fire = (handler) => {
      try {
        handler(payload, type);
        count += 1;
      } catch (error) {
        // A broken listener must never stop the rest of the bus.
        if (typeof console !== 'undefined' && console.warn) {
          console.warn(`[cyber-web] listener failed for ${type}:`, error);
        }
      }
    };
    for (const handler of this.handlers.get(type) || []) fire(handler);
    if (type !== '*') {
      for (const handler of this.handlers.get('*') || []) fire(handler);
    }
    return count;
  }
}

/** The six demo stages mirror the shape of a data-workflow `change` run. */
export const STAGES = Object.freeze([
  { key: 'intake', name: '需求评估', gate: 'REQ', hint: '原话 / 本质 / 未知' },
  { key: 'impact', name: '影响面', gate: 'IMPACT', hint: '四轴取证 / 维护性三查' },
  { key: 'plan', name: '方案门', gate: 'PLAN', hint: '接口 / 数据 / 状态 / 回滚' },
  { key: 'code', name: '编码实现', gate: 'EDIT', hint: '受保护路径 / 最小 diff' },
  { key: 'verify', name: '质量门', gate: 'VERIFY', hint: 'verify.log / exit 0' },
  { key: 'ship', name: '提交', gate: 'SHIP', hint: 'change-note / commit' },
]);

/**
 * Body parameters instead of sprite frames. Every field is interpolated by the
 * PoseMixer, so a state change is a short physical blend, never a frame jump.
 */
export const POSES = Object.freeze({
  idle: { sx: 1, sy: 1, tilt: 0, spread: 0.55, bob: 1 },
  running: { sx: 0.985, sy: 1.015, tilt: 0.022, spread: 0.92, bob: 1.35 },
  blocked: { sx: 0.9, sy: 1.1, tilt: -0.12, spread: 1.2, bob: 0.55 },
  verify: { sx: 0.96, sy: 1.04, tilt: 0.03, spread: 1.06, bob: 1.2 },
  fail: { sx: 1.16, sy: 0.76, tilt: -0.2, spread: 0.34, bob: 0.15 },
  done: { sx: 1.07, sy: 0.94, tilt: 0.05, spread: 0.6, bob: 1.1 },
});

const POSE_KEYS = Object.freeze(Object.keys(POSES.idle));

/**
 * Folds the workflow event stream into camera-friendly scalar weights.
 *
 * - stress rises with any traffic (tool calls, log lines, verify output) and
 *   decays exponentially, driving the animation `timeScale`.
 * - urgency rises with blocked gates / failing output and decays slower; it is
 *   the sum of a decaying value and an error-density window, so "log scrolls
 *   fast" and "errors cluster" both make the creature tremble.
 */
export class Activity {
  constructor() {
    this.stress = 0;
    this.urgency = 0;
    this.state = 'idle';
    this.stageIndex = 0;
    this.errorTimes = [];
    this.clock = 0;
  }

  ingest(event, now = this.clock) {
    const type = (event && event.type) || '';
    const data = (event && event.data) || {};
    const time = Number.isFinite(now) ? now : this.clock;
    this.clock = Math.max(this.clock, time);
    let error = false;
    let bump = 0;

    switch (type) {
      case 'run.start':
        this.state = 'running';
        bump = 0.05;
        break;
      case 'stage.enter':
        this.stageIndex = clamp(Number(data.index) || 0, 0, STAGES.length - 1);
        this.state = 'running';
        bump = 0.12;
        break;
      case 'tool.call':
        bump = 0.08;
        break;
      case 'tool.result':
        bump = data.ok === false ? 0.28 : 0.16;
        error = data.ok === false;
        break;
      case 'log.line': {
        const text = String(data.text || '');
        error = /FAIL|ERROR|RED|denied|拒绝|✕|✗/.test(text);
        bump = error ? 0.22 : 0.07;
        break;
      }
      case 'gate.blocked':
        this.state = 'blocked';
        bump = 0.3;
        this.urgency = clamp(this.urgency + 0.65, 0, 1);
        error = true;
        break;
      case 'anchor.fix':
        bump = 0.04;
        this.urgency = clamp(this.urgency - 0.1, 0, 1);
        break;
      case 'gate.passed':
        this.state = 'running';
        bump = 0.12;
        this.urgency = clamp(this.urgency - 0.3, 0, 1);
        // The gate resolved: previous error samples stop colouring urgency.
        this.errorTimes = [];
        break;
      case 'verify.start':
        this.state = 'verify';
        bump = 0.35;
        break;
      case 'verify.line': {
        const text = String(data.text || '');
        error = /FAIL|ERROR|RED|✕|✗/.test(text);
        bump = error ? 0.26 : 0.11;
        if (error) this.urgency = clamp(this.urgency + 0.12, 0, 1);
        break;
      }
      case 'verify.exit':
        if (Number(data.code) === 0) {
          // The quality gate passed, but the run is still open: the ship stage
          // and run.done follow. `done` belongs to run.done.
          this.state = 'running';
          bump = 0.2;
          this.urgency = clamp(this.urgency - 0.45, 0, 1);
          this.errorTimes = [];
        } else {
          this.state = 'fail';
          bump = 0.45;
          this.urgency = clamp(this.urgency + 0.5, 0, 1);
          error = true;
        }
        break;
      case 'run.done':
        this.state = 'done';
        bump = -0.2;
        this.urgency = clamp(this.urgency - 0.55, 0, 1);
        this.errorTimes = [];
        break;
      case 'run.aborted':
        this.state = 'fail';
        this.urgency = clamp(this.urgency + 0.4, 0, 1);
        break;
      default:
        break;
    }

    this.stress = clamp(this.stress + bump, 0, 1);
    if (error) this.errorTimes.push(this.clock);
    return this.snapshot(time);
  }

  tick(dt, now = this.clock) {
    const seconds = clamp(Number(dt) || 0, 0, 0.1);
    const time = Number.isFinite(now) ? now : this.clock;
    this.clock = Math.max(this.clock, time);
    this.stress = clamp(this.stress - seconds * 0.26, 0, 1);
    this.urgency = clamp(this.urgency - seconds * 0.18, 0, 1);
    const cutoff = this.clock - 6500;
    this.errorTimes = this.errorTimes.filter((stamp) => stamp >= cutoff);
    return this.snapshot(time);
  }

  errorDensity() {
    const cutoff = this.clock - 6500;
    const recent = this.errorTimes.filter((stamp) => stamp >= cutoff).length;
    return clamp(recent / 5, 0, 1);
  }

  snapshot() {
    const density = this.errorDensity();
    const urgency = clamp(Math.max(this.urgency, density * 0.85), 0, 1);
    return {
      state: this.state,
      stageIndex: this.stageIndex,
      stress: round4(this.stress),
      urgency: round4(urgency),
      timeScale: round4(0.85 + this.stress * 1.55),
      tremor: round4(0.35 + urgency * 2.6),
    };
  }
}

/**
 * Critically-damped spring per pose parameter. `omega` is high enough that a
 * full state change visibly settles inside ~0.15–0.2 s, while a small
 * overshoot (zeta < 1) keeps the transition springy instead of mechanical.
 */
export class PoseMixer {
  constructor() {
    this.current = { ...POSES.idle };
    this.velocity = { sx: 0, sy: 0, tilt: 0, spread: 0, bob: 0 };
    this.targetName = 'idle';
  }

  set(name) {
    if (POSES[name]) this.targetName = name;
    return this;
  }

  snap(name) {
    if (!POSES[name]) return this;
    this.targetName = name;
    this.current = { ...POSES[name] };
    for (const key of POSE_KEYS) this.velocity[key] = 0;
    return this;
  }

  step(dt) {
    const h = clamp(Number(dt) || 0, 0, 1 / 30);
    const target = POSES[this.targetName] || POSES.idle;
    const omega = 26;
    const zeta = 0.88;
    for (const key of POSE_KEYS) {
      const value = this.current[key];
      const velocity = this.velocity[key];
      const acceleration =
        (target[key] - value) * omega * omega - 2 * zeta * omega * velocity;
      this.velocity[key] = velocity + acceleration * h;
      this.current[key] = value + this.velocity[key] * h;
    }
    return this.current;
  }
}

/**
 * Verlet chain with distance constraints.
 *
 * The first point is pinned to `root` (the DOM centre of the mascot); every
 * other point integrates gravity + wind, then the constraint solver restores
 * the segment lengths. A soft positional pull on the tip lets it chase a DOM
 * anchor, while the rest of the chain keeps its own inertia.
 */
export class VerletChain {
  constructor(options = {}) {
    this.segments = Math.max(4, Number(options.segments) || 20);
    this.segment = Math.max(2, Number(options.segment) || 17);
    this.gravity = Number.isFinite(options.gravity) ? options.gravity : 2200;
    this.damping = Number.isFinite(options.damping) ? options.damping : 0.978;
    this.wind = Number.isFinite(options.wind) ? options.wind : 5200;
    this.seed = Number.isFinite(options.seed) ? options.seed : Math.random() * TAU;
    this.pullStrength = Number.isFinite(options.pullStrength) ? options.pullStrength : 10;
    this.iterations = Math.max(2, Number(options.iterations) || 7);
    this.target = null;
    this.root = { x: 0, y: 0 };
    this.time = 0;
    this.points = [];
    this.reset(Number(options.x) || 0, Number(options.y) || 0);
  }

  reset(x, y) {
    this.root = { x, y };
    this.points = Array.from({ length: this.segments + 1 }, (_, index) => ({
      x,
      y: y + index * this.segment,
      px: x,
      py: y + index * this.segment,
    }));
    this.time = 0;
    return this;
  }

  setRoot(x, y) {
    if (!Number.isFinite(x) || !Number.isFinite(y)) return this;
    const dx = x - this.root.x;
    const dy = y - this.root.y;
    this.root = { x, y };
    for (const point of this.points) {
      point.x += dx;
      point.y += dy;
      point.px += dx;
      point.py += dy;
    }
    const head = this.points[0];
    if (head) {
      head.x = x;
      head.y = y;
      head.px = x;
      head.py = y;
    }
    return this;
  }

  setTarget(point, pullStrength = this.pullStrength) {
    this.target =
      point && Number.isFinite(point.x) && Number.isFinite(point.y)
        ? { x: point.x, y: point.y }
        : null;
    this.pullStrength = clamp(Number(pullStrength) || 0, 0, 40);
    return this;
  }

  solve() {
    const points = this.points;
    for (let index = 0; index < points.length - 1; index += 1) {
      const a = points[index];
      const b = points[index + 1];
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      const distance = Math.hypot(dx, dy) || 1e-6;
      const correction = (distance - this.segment) / distance;
      if (index === 0) {
        b.x -= dx * correction;
        b.y -= dy * correction;
      } else {
        a.x += dx * correction * 0.5;
        a.y += dy * correction * 0.5;
        b.x -= dx * correction * 0.5;
        b.y -= dy * correction * 0.5;
      }
    }
    const head = points[0];
    head.x = this.root.x;
    head.y = this.root.y;
    head.px = this.root.x;
    head.py = this.root.y;
  }

  step(dt, options = {}) {
    const h = clamp(Number(dt) || 0, 0.001, 1 / 30);
    const substeps = 3;
    const slice = h / substeps;
    const gravity = Number.isFinite(options.gravity) ? options.gravity : this.gravity;
    const damping = Number.isFinite(options.damping) ? options.damping : this.damping;
    this.time += h;

    for (let substep = 0; substep < substeps; substep += 1) {
      for (let index = 1; index < this.points.length; index += 1) {
        const point = this.points[index];
        const vx = (point.x - point.px) * damping;
        const vy = (point.y - point.py) * damping;
        point.px = point.x;
        point.py = point.y;
        const weight = index / this.points.length;
        const wind =
          Math.sin((this.time + this.seed + index * 0.11) * 1.7) *
          this.wind *
          weight;
        point.x += vx + wind * slice * slice;
        point.y += vy + gravity * slice * slice;
      }

      if (this.target && this.pullStrength > 0) {
        const tip = this.points[this.points.length - 1];
        const k = 1 - Math.exp(-this.pullStrength * slice);
        tip.x += (this.target.x - tip.x) * k;
        tip.y += (this.target.y - tip.y) * k;
      }

      for (let iteration = 0; iteration < this.iterations; iteration += 1) {
        this.solve();
      }
    }

    for (const point of this.points) {
      if (!Number.isFinite(point.x) || !Number.isFinite(point.y)) {
        this.reset(this.root.x, this.root.y);
        break;
      }
    }
    return this.points;
  }
}

/** One-dimensional spring used for the vertical progress pointer. */
export class Pointer {
  constructor(x = 0) {
    this.x = Number(x) || 0;
    this.target = this.x;
    this.velocity = 0;
  }

  moveTo(x) {
    if (Number.isFinite(x)) this.target = x;
    return this;
  }

  snap(x) {
    if (!Number.isFinite(x)) return this;
    this.x = x;
    this.target = x;
    this.velocity = 0;
    return this;
  }

  step(dt) {
    const h = clamp(Number(dt) || 0, 0, 1 / 30);
    const stiffness = 95;
    const damping = 13.5;
    const acceleration =
      (this.target - this.x) * stiffness - this.velocity * damping;
    this.velocity += acceleration * h;
    this.x += this.velocity * h;
    return this.x;
  }
}

/** Deterministic timeline player with pause/resume and speed control. */
export class ScenarioPlayer {
  constructor(events, options = {}) {
    this.events = (Array.isArray(events) ? events : [])
      .slice()
      .sort((a, b) => a.at - b.at);
    this.speed = clamp(Number(options.speed) || 1, 0.25, 3);
    this.clock = 0;
    this.index = 0;
    this.last = 0;
    this.playing = false;
  }

  start(now = 0) {
    this.clock = 0;
    this.index = 0;
    this.last = Number(now) || 0;
    this.playing = true;
    return this;
  }

  setSpeed(speed) {
    this.speed = clamp(Number(speed) || 1, 0.25, 3);
    return this;
  }

  pause(now = 0) {
    if (!this.playing) return this;
    this.last = Number(now) || 0;
    this.playing = false;
    return this;
  }

  resume(now = 0) {
    if (this.playing) return this;
    this.last = Number(now) || 0;
    this.playing = true;
    return this;
  }

  tick(now = 0) {
    if (!this.playing) return [];
    const time = Number(now) || 0;
    const delta = Math.max(0, time - this.last);
    this.last = time;
    this.clock += delta * this.speed;
    const due = [];
    while (this.index < this.events.length && this.events[this.index].at <= this.clock) {
      due.push(this.events[this.index]);
      this.index += 1;
    }
    return due;
  }

  /**
   * Jump to an absolute scenario time and return every event that became due.
   * Used by the demo's deterministic `?at=` freeze and by debug replay; it does
   * not consult wall-clock time, so a screenshot is reproducible.
   */
  skipTo(clockMs) {
    const target = Math.max(0, Number(clockMs) || 0);
    this.clock = Math.max(this.clock, target);
    const due = [];
    while (this.index < this.events.length && this.events[this.index].at <= this.clock) {
      due.push(this.events[this.index]);
      this.index += 1;
    }
    this.last = 0;
    return due;
  }

  get done() {
    return this.index >= this.events.length;
  }

  get duration() {
    return this.events.length ? this.events[this.events.length - 1].at : 0;
  }

  get progress() {
    return this.events.length ? this.index / this.events.length : 0;
  }
}

const event = (at, type, data = {}) => ({ at, type, data });

/**
 * Two scripted runs. Both end at the quality gate; the red run exits with a
 * failing verify so the demo can show the damped-slump reaction.
 */
export function buildScenario(mode = 'green') {
  const red = mode === 'red';
  const cmd = 'python3 skills/data-workflow/scripts/check_runner.py';
  const verifyLines = red
    ? [
        { at: 9400, ok: true, text: 'PASS none stays RED' },
        { at: 9700, ok: false, text: 'FAIL protected write should be denied' },
        { at: 10000, ok: true, text: 'PASS README first-screen matches change.json requires' },
        { at: 10300, ok: false, text: 'FAIL verify.log exit is not 0' },
        { at: 10600, ok: true, text: 'PASS run.json schema' },
      ]
    : [
        { at: 9400, ok: true, text: 'PASS none stays RED' },
        { at: 9700, ok: true, text: 'PASS README first-screen matches change.json requires' },
        { at: 10000, ok: true, text: 'PASS scripts.md lists every script' },
        { at: 10300, ok: true, text: 'PASS gate-edit denies protected write' },
        { at: 10600, ok: true, text: 'PASS run.json schema' },
      ];

  const events = [
    event(0, 'run.start', {
      feature: 'cyber-gate',
      workflow: 'change',
      raw: '把门禁状态画成一张会呼吸的网',
    }),
    event(350, 'stage.enter', { index: 0 }),
    event(700, 'log.line', { tag: 'READ', text: 'workflows/change.md' }),
    event(1000, 'tool.call', { name: 'write', detail: 'decision.md' }),
    event(1450, 'tool.result', { name: 'write', ok: true }),
    event(1850, 'gate.blocked', {
      missing: [
        { label: '怎样算满足', anchor: 'missing-1' },
        { label: '未知', anchor: 'missing-2' },
      ],
    }),
    event(3000, 'anchor.fix', { anchor: 'missing-1', label: '怎样算满足' }),
    event(3500, 'anchor.fix', { anchor: 'missing-2', label: '未知' }),
    event(3950, 'gate.passed', { stage: 0 }),
    event(4300, 'stage.enter', { index: 1 }),
    event(4700, 'log.line', { tag: 'IMPACT', text: '四轴取证 / 维护性三查' }),
    event(5100, 'gate.passed', { stage: 1 }),
    event(5400, 'stage.enter', { index: 2 }),
    event(5800, 'log.line', {
      tag: 'PLAN',
      text: '接口 / 数据 / 状态 / 权限 / 验证 / 回滚',
    }),
    event(6200, 'gate.passed', { stage: 2 }),
    event(6500, 'stage.enter', { index: 3 }),
    event(6800, 'tool.call', { name: 'edit', detail: 'client.js' }),
    event(7200, 'tool.result', { name: 'edit', ok: true }),
    event(7500, 'tool.call', { name: 'edit', detail: 'index.js' }),
    event(7900, 'tool.result', { name: 'edit', ok: true }),
    event(8200, 'tool.call', { name: 'bash', detail: 'git status --porcelain' }),
    event(8600, 'tool.result', { name: 'bash', ok: true }),
    event(8900, 'stage.enter', { index: 4 }),
    event(9200, 'verify.start', { cmd }),
    ...verifyLines.map((line) =>
      event(line.at, 'verify.line', { text: line.text, ok: line.ok }),
    ),
    event(11200, 'verify.exit', { code: red ? 3 : 0, cmd }),
  ];

  if (red) {
    events.push(event(12200, 'run.aborted', { reason: 'verify exit 3' }));
  } else {
    events.push(event(11800, 'stage.enter', { index: 5 }));
    events.push(event(12400, 'run.done', { feature: 'cyber-gate' }));
  }

  return events;
}
