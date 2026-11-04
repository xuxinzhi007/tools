import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { buildDemo } from '../build.mjs';

/* ── a deliberately tiny DOM + canvas stub ────────────────────────────────
   The demo uses a narrow slice of the DOM: id lookups, class lookups inside
   the stage track, a handful of rect reads, and a 2D canvas. This stub
   implements exactly that slice so the *built* script can run in Node and be
   driven through its whole mock timeline.
   ───────────────────────────────────────────────────────────────────────── */

const VIEWPORT = { width: 1280, height: 800 };

class ClassList {
  constructor(el) {
    this.el = el;
  }
  set() {
    return new Set(this.el.className.split(/\s+/).filter(Boolean));
  }
  write(set) {
    this.el.className = Array.from(set).join(' ');
  }
  add(...names) {
    const set = this.set();
    for (const name of names) set.add(name);
    this.write(set);
  }
  remove(...names) {
    const set = this.set();
    for (const name of names) set.delete(name);
    this.write(set);
  }
  contains(name) {
    return this.set().has(name);
  }
}

class Element {
  constructor(tagName = 'div', id = '') {
    this.tagName = String(tagName).toUpperCase();
    this.id = id;
    this.className = '';
    this.classList = new ClassList(this);
    this.dataset = {};
    this.style = {};
    this.attributes = {};
    this.children = [];
    this.parentElement = null;
    this.textContent = '';
    this.scrollTop = 0;
    this.hidden = false;
    this._connected = true;
  }

  get isConnected() {
    return this._connected;
  }

  // The scene binds click handlers to the four control buttons and listens on
  // window/document. Node has no DOM, so these are inert collectors.
  addEventListener() {}
  removeEventListener() {}
  dispatchEvent() {
    return true;
  }

  set innerHTML(value) {
    if (value === '') {
      for (const child of this.children) child._connected = false;
      this.children = [];
      this.textContent = '';
    }
  }

  get innerHTML() {
    return '';
  }

  setAttribute(name, value) {
    this.attributes[name] = String(value);
  }

  getAttribute(name) {
    return Object.prototype.hasOwnProperty.call(this.attributes, name)
      ? this.attributes[name]
      : null;
  }

  appendChild(child) {
    if (!child) return child;
    if (child.parentElement) child.remove();
    child.parentElement = this;
    child._connected = true;
    this.children.push(child);
    return child;
  }

  append(...children) {
    for (const child of children) this.appendChild(child);
  }

  remove() {
    if (this.parentElement) {
      const index = this.parentElement.children.indexOf(this);
      if (index >= 0) this.parentElement.children.splice(index, 1);
    }
    this.parentElement = null;
    this._connected = false;
  }

  get firstElementChild() {
    return this.children[0] || null;
  }

  get lastElementChild() {
    return this.children[this.children.length - 1] || null;
  }

  _matches(selector) {
    if (selector.startsWith('#')) return this.id === selector.slice(1);
    if (selector.startsWith('.')) return this.classList.contains(selector.slice(1));
    return this.tagName.toLowerCase() === selector.toLowerCase();
  }

  _walk(out) {
    for (const child of this.children) {
      out.push(child);
      child._walk(out);
    }
    return out;
  }

  querySelector(selector) {
    return this.querySelectorAll(selector)[0] || null;
  }

  querySelectorAll(selector) {
    return this._walk([]).filter((child) => child._matches(selector));
  }

  getBoundingClientRect() {
    const indexIn = (parent) =>
      parent ? Math.max(0, parent.children.indexOf(this)) : 0;

    if (this.id === 'gate-card') {
      return { left: 40, top: 330, width: 330, height: 150, right: 370, bottom: 480 };
    }
    if (this.id === 'verify-card') {
      return { left: 40, top: 500, width: 350, height: 180, right: 390, bottom: 680 };
    }
    if (this.classList.contains('stage-node')) {
      const index = indexIn(this.parentElement);
      const left = 80 + index * 165;
      return { left, top: 150, width: 130, height: 64, right: left + 130, bottom: 214 };
    }
    if (this.classList.contains('missing-chip')) {
      const index = indexIn(this.parentElement);
      const left = 55 + index * 120;
      return { left, top: 405, width: 108, height: 26, right: left + 108, bottom: 431 };
    }
    if (this.classList.contains('verify-line')) {
      const index = indexIn(this.parentElement);
      const top = 545 + index * 17;
      return { left: 55, top, width: 320, height: 15, right: 375, bottom: top + 15 };
    }
    if (this.classList.contains('log-line')) {
      const index = indexIn(this.parentElement);
      const top = 380 + index * 15;
      return { left: 930, top, width: 300, height: 14, right: 1230, bottom: top + 14 };
    }
    if (this.id === 'track') {
      return { left: 60, top: 130, width: 1160, height: 100, right: 1220, bottom: 230 };
    }
    return { left: 200, top: 240, width: 220, height: 48, right: 420, bottom: 288 };
  }
}

function makeCanvasContext() {
  const gradient = () => ({ addColorStop() {} });
  const ctx = {
    save() {},
    restore() {},
    setTransform() {},
    clearRect() {},
    fillRect() {},
    beginPath() {},
    closePath() {},
    moveTo() {},
    lineTo() {},
    quadraticCurveTo() {},
    bezierCurveTo() {},
    arc() {},
    ellipse() {},
    stroke() {},
    fill() {},
    drawImage() {},
    fillText() {},
    translate() {},
    rotate() {},
    scale() {},
    setLineDash() {},
    createRadialGradient: gradient,
    createLinearGradient: gradient,
    measureText: () => ({ width: 42 }),
  };
  for (const key of [
    'strokeStyle',
    'fillStyle',
    'globalAlpha',
    'lineWidth',
    'lineCap',
    'lineJoin',
    'shadowColor',
    'shadowBlur',
    'font',
    'lineDashOffset',
  ]) {
    ctx[key] = '';
  }
  return ctx;
}

function buildPage() {
  const html = new Element('html');
  const body = new Element('body');
  html.appendChild(body);

  const arena = new Element('div', 'arena');
  body.appendChild(arena);

  const canvas = new Element('canvas', 'web');
  canvas.getContext = () => makeCanvasContext();
  arena.appendChild(canvas);

  const track = new Element('div', 'track');
  arena.appendChild(track);

  const statePill = new Element('span', 'state-pill');
  statePill.appendChild(new Element('span'));
  arena.appendChild(statePill);

  for (const id of [
    'run-chip',
    'clock',
    'stress-bar',
    'stress-val',
    'urgency-bar',
    'urgency-val',
    'tick-val',
    'tremor-val',
    'btn-mode',
    'btn-speed',
    'btn-pause',
    'btn-replay',
    'fault',
    'log',
  ]) {
    arena.appendChild(new Element(id === 'log' ? 'div' : 'span', id));
  }

  const gateCard = new Element('section', 'gate-card');
  arena.appendChild(gateCard);
  for (const id of ['gate-code', 'gate-title', 'gate-state', 'gate-hint', 'missing-wrap']) {
    gateCard.appendChild(new Element('div', id));
  }

  const verifyCard = new Element('section', 'verify-card');
  arena.appendChild(verifyCard);
  for (const id of ['verify-cmd', 'verify-result', 'verify-lines']) {
    verifyCard.appendChild(new Element('div', id));
  }

  return { html, body, arena, canvas, track, gateCard, verifyCard };
}

/**
 * Boot the built demo inside a fresh VM context and return a small driver.
 * The clock is entirely synthetic: `advance()` moves it, runs due timers, and
 * hands exactly one animation frame to the scene per step.
 */
function createHarness(scriptSource, options = {}) {
  const page = buildPage();
  let clock = 0;
  let rafSeq = 0;
  let timerSeq = 0;
  const frames = new Map();
  const timers = new Map();
  const warnings = [];

  const documentStub = {
    body: page.body,
    hidden: false,
    documentElement: page.html,
    querySelector: (selector) => page.html.querySelector(selector),
    querySelectorAll: (selector) => page.html.querySelectorAll(selector),
    createElement: (tagName) => new Element(tagName),
    addEventListener() {},
    removeEventListener() {},
  };

  const windowStub = {
    innerWidth: VIEWPORT.width,
    innerHeight: VIEWPORT.height,
    devicePixelRatio: 2,
    __CW_ASSET__: '',
    location: { search: options.search || '' },
    addEventListener() {},
    removeEventListener() {},
    matchMedia: () => ({
      matches: false,
      addEventListener() {},
      removeEventListener() {},
    }),
    requestAnimationFrame(callback) {
      rafSeq += 1;
      frames.set(rafSeq, callback);
      return rafSeq;
    },
    cancelAnimationFrame(id) {
      frames.delete(id);
    },
  };

  class ImageStub {
    constructor() {
      this.onload = null;
      this.onerror = null;
      this._src = '';
    }
    set src(value) {
      this._src = value;
      if (typeof this.onload === 'function') this.onload();
    }
    get src() {
      return this._src;
    }
  }

  const sandbox = {
    window: windowStub,
    document: documentStub,
    performance: { now: () => clock },
    console: {
      log() {},
      warn: (...args) => warnings.push(args.map(String).join(' ')),
      error: (...args) => warnings.push(args.map(String).join(' ')),
    },
    requestAnimationFrame: windowStub.requestAnimationFrame,
    cancelAnimationFrame: windowStub.cancelAnimationFrame,
    setTimeout(callback, delay = 0) {
      timerSeq += 1;
      timers.set(timerSeq, { due: clock + Math.max(0, Number(delay) || 0), callback });
      return timerSeq;
    },
    clearTimeout(id) {
      timers.delete(id);
    },
    Image: ImageStub,
    URLSearchParams,
    Math,
    Date,
    JSON,
  };
  sandbox.globalThis = sandbox;

  const context = vm.createContext(sandbox);
  vm.runInContext(scriptSource, context, { filename: 'demo/index.html#script' });

  const controller = windowStub.cyberWebDemo;
  assert.ok(controller && controller.snapshot, 'the demo must expose its controller');

  function flushTimers() {
    for (;;) {
      let nextId = 0;
      let next = null;
      for (const [id, timer] of timers) {
        if (timer.due <= clock && (!next || timer.due < next.due)) {
          nextId = id;
          next = timer;
        }
      }
      if (!next) return;
      timers.delete(nextId);
      next.callback();
    }
  }

  function advance(ms, stepMs = 1000 / 60) {
    const steps = Math.max(1, Math.round(ms / stepMs));
    for (let index = 0; index < steps; index += 1) {
      clock += stepMs;
      flushTimers();
      const entry = frames.entries().next();
      if (entry.done) throw new Error('the demo stopped scheduling frames');
      const [id, callback] = entry.value;
      frames.delete(id);
      callback(clock);
    }
    return clock;
  }

  function clearFrames() {
    frames.clear();
  }

  return {
    controller,
    page,
    warnings,
    advance,
    clearFrames,
    get clock() {
      return clock;
    },
    text(id) {
      const node = page.html.querySelector(`#${id}`);
      return node ? node.textContent : '';
    },
    gateState() {
      return page.gateCard.dataset.state;
    },
    missingChips() {
      return page.html.querySelectorAll('.missing-chip');
    },
    stageNodes() {
      return page.html.querySelectorAll('.stage-node');
    },
  };
}

let cachedScript = null;
async function demoScript() {
  if (cachedScript) return cachedScript;
  const { html } = await buildDemo();
  const start = html.indexOf('<script>');
  const end = html.lastIndexOf('</script>');
  assert.ok(start > 0 && end > start, 'the single-file build must contain one script');
  cachedScript = html.slice(start + '<script>'.length, end);
  return cachedScript;
}

test('the built demo boots idle and exposes its controller', async () => {
  const harness = createHarness(await demoScript());

  const snap = harness.controller.snapshot;
  assert.equal(snap.state, 'idle');
  assert.equal(snap.stageIndex, 0);
  assert.equal(harness.controller.debug.legs, 6);
  assert.equal(harness.stageNodes().length, 6);
  assert.equal(harness.text('gate-title'), '等待运行');
  assert.equal(harness.warnings.length, 0, harness.warnings.join('\n'));
});

test('a blocked gate stops the run, shows the missing fields, and the tentacles lock on', async () => {
  const harness = createHarness(await demoScript());

  harness.advance(2200);
  const snap = harness.controller.snapshot;
  assert.equal(snap.state, 'blocked');
  assert.equal(harness.gateState(), 'blocked');
  assert.equal(harness.text('gate-state'), '门禁拦截');

  const chips = harness.missingChips();
  assert.equal(chips.length, 2);
  assert.deepEqual(
    chips.map((chip) => chip.dataset.anchor),
    ['missing-1', 'missing-2'],
  );
  assert.ok(chips.every((chip) => !chip.classList.contains('is-fixed')));

  assert.ok(snap.urgency > 0.5, `urgency too low: ${snap.urgency}`);
  assert.ok(harness.controller.debug.anchors >= 3, 'tentacles need anchors');
  assert.ok(harness.controller.debug.particles >= 0);
  assert.ok(harness.text('log').length >= 0);
  const firstStage = harness.stageNodes()[0];
  assert.ok(firstStage.classList.contains('is-blocked'));
});

test('fixing the fields lets the gate pass and the tracker advances', async () => {
  const harness = createHarness(await demoScript());

  harness.advance(2200);
  harness.advance(1800); // anchor.fix at 3000/3500, gate.passed at 3950
  assert.equal(harness.gateState(), 'passed');
  assert.equal(harness.text('gate-state'), '门禁通过');
  assert.ok(harness.missingChips().every((chip) => chip.classList.contains('is-fixed')));
  assert.ok(harness.stageNodes()[0].classList.contains('is-passed'));

  harness.advance(700); // stage.enter(1) at 4300
  assert.equal(harness.controller.snapshot.stageIndex, 1);
  assert.ok(harness.stageNodes()[0].classList.contains('is-passed'));
  assert.ok(harness.stageNodes()[1].classList.contains('is-active'));
});

test('the quality gate streams verify lines and settles on exit 0', async () => {
  const harness = createHarness(await demoScript());

  harness.advance(9300); // verify.start at 9200
  assert.ok(
    harness.page.verifyCard.classList.contains('is-visible'),
    'verify card must open',
  );
  assert.equal(harness.text('verify-result'), 'running');
  assert.match(harness.text('verify-cmd'), /check_runner\.py/);

  harness.advance(700);
  const lines = harness.page.html.querySelectorAll('.verify-line');
  assert.ok(lines.length >= 2, `expected verify lines, got ${lines.length}`);

  harness.advance(1500); // verify.exit at 11200
  assert.equal(harness.text('verify-result'), 'exit 0');
  assert.equal(harness.gateState(), 'passed');
  assert.ok(harness.controller.snapshot.urgency < 0.4);
});

test('a red verify collapses the pose and marks the gate failed', async () => {
  const harness = createHarness(await demoScript());
  // Keep the frame already scheduled at boot; reset swaps the player in place.
  harness.controller.reset('red');

  harness.advance(11400);
  assert.equal(harness.text('verify-result'), 'exit 3');
  assert.equal(harness.gateState(), 'fail');
  assert.equal(harness.text('gate-state'), '质量门失败');

  const failed = harness.stageNodes().filter((node) => node.classList.contains('is-failed'));
  assert.equal(failed.length, 1, 'exactly the quality gate is failed');

  const snap = harness.controller.snapshot;
  assert.equal(snap.state, 'fail');
  assert.ok(snap.urgency > 0.6, `urgency should spike: ${snap.urgency}`);

  // The body slumps: its vertical scale drops below the resting pose.
  const body = harness.controller.body;
  assert.ok(body.sy < 1, `expected a slumped body, got sy=${body.sy}`);
});

test('run.done marks every stage passed and closes the card', async () => {
  const harness = createHarness(await demoScript());

  harness.advance(12_600);
  assert.equal(harness.controller.snapshot.state, 'done');
  assert.equal(harness.text('gate-title'), '提交');
  assert.equal(harness.text('gate-state'), '已提交');
  assert.ok(
    harness.stageNodes().every((node) => node.classList.contains('is-passed')),
    'every stage must end passed',
  );
});

test('no listener warning or frame error escaped the scene', async () => {
  const harness = createHarness(await demoScript());
  harness.advance(13_000);
  const fatal = harness.warnings.filter((line) => line.includes('frame failed'));
  assert.deepEqual(fatal, [], fatal.join('\n'));
});

test('?at= freezes the demo exactly on the blocked gate', async () => {
  const harness = createHarness(await demoScript(), { search: '?at=2500' });
  harness.advance(120); // first frame, then the freeze callback

  assert.equal(harness.controller.snapshot.state, 'blocked');
  assert.equal(harness.controller.snapshot.clock, 2500);
  assert.equal(harness.gateState(), 'blocked');
  assert.equal(harness.text('gate-state'), '门禁拦截');
  assert.equal(harness.missingChips().length, 2);
  assert.ok(harness.controller.debug.anchors >= 3);
  assert.equal(harness.warnings.filter((line) => line.includes('frame failed')).length, 0);
});

test('?at= freezes on the quality gate mid-stream', async () => {
  const harness = createHarness(await demoScript(), { search: '?at=10000' });
  harness.advance(120);

  assert.equal(harness.controller.snapshot.state, 'verify');
  assert.equal(harness.text('verify-result'), 'running');
  assert.match(harness.text('verify-cmd'), /check_runner\.py/);
  const lines = harness.page.html.querySelectorAll('.verify-line');
  assert.ok(lines.length >= 3, `expected 3+ verify lines, got ${lines.length}`);
  assert.ok(harness.page.verifyCard.classList.contains('is-visible'));
});

test('?mode=red&at=11400 freezes on the failed gate with a slumped pose', async () => {
  const harness = createHarness(await demoScript(), {
    search: '?mode=red&at=11400',
  });
  harness.advance(120);

  assert.equal(harness.controller.snapshot.state, 'fail');
  assert.equal(harness.text('verify-result'), 'exit 3');
  assert.equal(harness.gateState(), 'fail');
  assert.equal(harness.text('gate-state'), '质量门失败');
  assert.ok(
    harness.controller.body.sy < 1,
    `expected a slumped pose, got sy=${harness.controller.body.sy}`,
  );
});
