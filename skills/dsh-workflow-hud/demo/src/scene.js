/**
 * Cyber Web Mascot — scene layer.
 *
 * Runs in the browser (and inside the single-file build). It consumes the
 * identifiers defined by `core.js` (no imports): EventBus, Activity,
 * PoseMixer, VerletChain, Pointer, ScenarioPlayer, buildScenario, STAGES,
 * POSES, clamp, lerp, formatClock.
 *
 * The layer is deliberately read-only with respect to the workflow: it never
 * touches run.json, never awaits anything from the host, and every listener is
 * wrapped so a rendering error cannot propagate back into the gate pipeline.
 */
function initCyberDemo() {
  'use strict';

  const $ = (selector, root) => (root || document).querySelector(selector);
  const $$ = (selector, root) => Array.from((root || document).querySelectorAll(selector));

  const dom = {
    canvas: $('#web'),
    track: $('#track'),
    arena: $('#arena'),
    gateCard: $('#gate-card'),
    gateCode: $('#gate-code'),
    gateTitle: $('#gate-title'),
    gateState: $('#gate-state'),
    gateHint: $('#gate-hint'),
    missingWrap: $('#missing-wrap'),
    verifyCard: $('#verify-card'),
    verifyCmd: $('#verify-cmd'),
    verifyLines: $('#verify-lines'),
    verifyResult: $('#verify-result'),
    log: $('#log'),
    statePill: $('#state-pill'),
    runChip: $('#run-chip'),
    clock: $('#clock'),
    stressBar: $('#stress-bar'),
    stressVal: $('#stress-val'),
    urgencyBar: $('#urgency-bar'),
    urgencyVal: $('#urgency-val'),
    tickVal: $('#tick-val'),
    tremorVal: $('#tremor-val'),
    modeBtn: $('#btn-mode'),
    speedBtn: $('#btn-speed'),
    pauseBtn: $('#btn-pause'),
    replayBtn: $('#btn-replay'),
    fault: $('#fault'),
  };

  if (!dom.canvas || !dom.track || !dom.arena) {
    // A partial template: stay silent rather than throwing during page boot.
    return;
  }

  const ctx = dom.canvas.getContext('2d');
  if (!ctx) {
    if (dom.fault) dom.fault.hidden = false;
    return;
  }

  const PALETTE = {
    idle: '#7c9cff',
    running: '#38e1ff',
    blocked: '#ffb347',
    verify: '#c084fc',
    fail: '#ff4d6d',
    done: '#46e6a0',
    pass: '#46e6a0',
    log: '#9fd8ff',
  };

  const STATE_TEXT = {
    idle: '待命',
    running: '运行中',
    blocked: '门禁拦截',
    verify: '验证中',
    fail: '已失败',
    done: '已完成',
  };

  const LEGS = 6;
  const STAGE_COUNT = STAGES.length;
  const reducedMotion =
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  let mode = 'green';
  let speed = 1;
  let paused = false;
  let lastTs = 0;
  let raf = 0;
  let resetToken = 0;
  let errors = 0;
  let stopped = false;
  let lastState = 'idle';
  let slump = 0;
  let landPulse = 0;
  let lastStageIndex = 0;
  let currentSnap = {
    state: 'idle',
    stageIndex: 0,
    stress: 0,
    urgency: 0,
    timeScale: 1,
    tremor: 0.4,
  };

  let activity;
  let mixer;
  let player;
  let posX;
  let posY;
  let bus;
  let legs = [];
  let gateMissing = [];
  let verifyLines = [];
  let particles = [];
  let waves = [];
  let stageStates = [];
  let anchorCache = { at: 0, list: [] };
  let bodyImage = null;
  let imageReady = false;
  let lastBody = { x: 0, y: 0, tilt: 0, sx: 1, sy: 1 };

  const timers = new Set();

  function queueTimer(fn, delay, token) {
    const id = setTimeout(() => {
      timers.delete(id);
      if (token === resetToken) fn();
    }, delay);
    timers.add(id);
    return id;
  }

  function clearTimers() {
    for (const id of timers) clearTimeout(id);
    timers.clear();
  }

  function setText(node, text) {
    if (node && node.textContent !== text) node.textContent = text;
  }

  /* ── stage track ─────────────────────────────────────────────────────── */

  function buildTrack() {
    dom.track.innerHTML = '';
    for (let index = 0; index < STAGE_COUNT; index += 1) {
      const stage = STAGES[index];
      const node = document.createElement('div');
      node.className = 'stage-node';
      node.dataset.index = String(index);
      node.dataset.gate = stage.gate;

      const dot = document.createElement('span');
      dot.className = 'stage-dot';
      dot.setAttribute('aria-hidden', 'true');

      const name = document.createElement('span');
      name.className = 'stage-name';
      name.textContent = stage.name;

      const gate = document.createElement('span');
      gate.className = 'stage-gate';
      gate.textContent = stage.gate;

      node.append(dot, name, gate);
      node.title = `${index + 1}. ${stage.name} · ${stage.gate} · ${stage.hint}`;
      dom.track.appendChild(node);
    }
  }

  function refreshStages() {
    const nodes = $$('.stage-node', dom.track);
    nodes.forEach((node, index) => {
      const state = stageStates[index] || 'pending';
      node.className = `stage-node is-${state}`;
      node.dataset.state = state;
      node.setAttribute('aria-current', state === 'active' ? 'step' : 'false');
    });
  }

  function setStage(index, state) {
    if (index < 0 || index >= STAGE_COUNT) return;
    stageStates[index] = state;
    refreshStages();
  }

  function markStagePassed(index) {
    if (index < 0 || index >= STAGE_COUNT) return;
    stageStates[index] = 'passed';
    refreshStages();
  }

  /* ── log ─────────────────────────────────────────────────────────────── */

  function addLog(tag, text, tone) {
    const row = document.createElement('div');
    row.className = `log-line is-${tone || 'info'}`;

    const time = document.createElement('span');
    time.className = 'log-time';
    time.textContent = formatClock(player ? player.clock : 0);

    const label = document.createElement('span');
    label.className = 'log-tag';
    label.textContent = tag;

    const body = document.createElement('span');
    body.className = 'log-text';
    body.textContent = text;

    row.append(time, label, body);
    dom.log.appendChild(row);
    while (dom.log.children.length > 90) dom.log.firstElementChild.remove();
    dom.log.scrollTop = dom.log.scrollHeight;
  }

  /* ── gate card ───────────────────────────────────────────────────────── */

  function showGate(stage, stateText, hint, state) {
    setText(dom.gateCode, stage ? stage.gate : '—');
    setText(dom.gateTitle, stage ? stage.name : '等待运行');
    setText(dom.gateState, stateText);
    setText(dom.gateHint, hint || '');
    dom.gateCard.dataset.state = state || 'idle';
    dom.gateCard.classList.add('is-visible');
  }

  function clearMissing() {
    for (const entry of gateMissing) entry.el.remove();
    gateMissing = [];
  }

  function addMissing(items) {
    clearMissing();
    dom.missingWrap.innerHTML = '';
    for (let index = 0; index < items.length; index += 1) {
      const item = items[index];
      const chip = document.createElement('span');
      chip.className = 'missing-chip';
      chip.dataset.anchor = item.anchor || `missing-${index}`;
      chip.dataset.label = item.label || '';

      const icon = document.createElement('b');
      icon.textContent = '✕';
      const label = document.createElement('span');
      label.textContent = item.label || '';

      chip.append(icon, label);
      dom.missingWrap.appendChild(chip);
      gateMissing.push({
        anchor: item.anchor || `missing-${index}`,
        label: item.label || '',
        el: chip,
        fixed: false,
      });
    }
  }

  function fixMissing(anchor) {
    const entry = gateMissing.find((item) => item.anchor === anchor);
    if (!entry || entry.fixed) return;
    entry.fixed = true;
    entry.el.classList.add('is-fixed');
    const icon = entry.el.querySelector('b');
    if (icon) icon.textContent = '✓';
    queueTimer(() => {
      entry.el.remove();
    }, 750, resetToken);
  }

  function passGate() {
    for (const entry of gateMissing) {
      if (!entry.fixed) {
        entry.fixed = true;
        entry.el.classList.add('is-fixed');
        const icon = entry.el.querySelector('b');
        if (icon) icon.textContent = '✓';
      }
    }
    dom.gateCard.dataset.state = 'passed';
    queueTimer(() => {
      dom.missingWrap.innerHTML = '';
      gateMissing = [];
    }, 950, resetToken);
  }

  /* ── verify card ─────────────────────────────────────────────────────── */

  function showVerify(cmd) {
    dom.verifyCard.classList.add('is-visible');
    setText(dom.verifyCmd, cmd || '');
    dom.verifyLines.innerHTML = '';
    dom.verifyResult.textContent = 'running';
    dom.verifyResult.className = 'verify-result is-running';
    verifyLines = [];
  }

  function addVerifyLine(text, ok) {
    const row = document.createElement('div');
    row.className = `verify-line ${ok ? 'is-pass' : 'is-fail'}`;

    const tag = document.createElement('span');
    tag.className = 'verify-tag';
    tag.textContent = ok ? 'PASS' : 'FAIL';

    const body = document.createElement('span');
    body.className = 'verify-text';
    body.textContent = text;

    row.append(tag, body);
    dom.verifyLines.appendChild(row);
    while (dom.verifyLines.children.length > 40) dom.verifyLines.firstElementChild.remove();
    dom.verifyLines.scrollTop = dom.verifyLines.scrollHeight;
    verifyLines.push({ el: row, ok, text });
    while (verifyLines.length > 40) verifyLines.shift();

    // A small label drifting up beside the mascot, not across the page.
    const short = text.length > 18 ? `${text.slice(0, 18)}…` : text;
    spawnText(
      short,
      lastBody.x + 30,
      lastBody.y - 72,
      ok ? '#46e6a0' : '#ff4d6d',
      9,
    );
  }

  function setVerifyResult(code) {
    dom.verifyResult.textContent = `exit ${code}`;
    dom.verifyResult.className = `verify-result ${code === 0 ? 'is-pass' : 'is-fail'}`;
  }

  /* ── particles ───────────────────────────────────────────────────────── */

  function spawnText(text, x, y, color, size) {
    if (reducedMotion) return;
    // Cap concurrent floating labels: they are decoration, and an unbounded
    // stream buries the verify card under overlapping green text.
    const texts = particles.filter((item) => item.kind === 'text');
    if (texts.length >= 3) {
      const oldest = texts[0];
      particles = particles.filter((item) => item !== oldest);
    }
    particles.push({
      kind: 'text',
      text,
      x: clamp(x, 26, window.innerWidth - 150),
      y: clamp(y, 46, window.innerHeight - 70),
      vx: (Math.random() - 0.5) * 10,
      vy: -16 - Math.random() * 8,
      life: 0,
      ttl: 1.6,
      color: color || '#9fd8ff',
      size: size || 10,
    });
  }

  function spawnShard(x, y, color) {
    if (reducedMotion) return;
    const angle = Math.random() * Math.PI * 2;
    const power = 40 + Math.random() * 190;
    particles.push({
      kind: 'shard',
      x,
      y,
      vx: Math.cos(angle) * power,
      vy: Math.sin(angle) * power - 30,
      life: 0,
      ttl: 1.1 + Math.random() * 0.8,
      color,
      size: 1.5 + Math.random() * 2.4,
    });
  }

  function confetti(x, y) {
    const colors = ['#46e6a0', '#38e1ff', '#ff4fd8', '#ffd166', '#9d8cff'];
    for (let i = 0; i < 70; i += 1) {
      spawnShard(x, y, colors[i % colors.length]);
    }
  }

  function shockwave(x, y, color, power) {
    waves.push({
      x,
      y,
      color,
      life: 0,
      ttl: 1.1,
      radius: 10,
      power: power || 1,
    });
  }

  /* ── run lifecycle ───────────────────────────────────────────────────── */

  function setStatePill(state) {
    const pill = dom.statePill;
    if (!pill) return;
    pill.dataset.state = state;
    const label = pill.querySelector('span');
    if (label) label.textContent = STATE_TEXT[state] || state;
  }

  function resetDemo(nextMode) {
    mode = nextMode || mode;
    resetToken += 1;
    clearTimers();

    paused = false;
    stopped = false;
    errors = 0;
    slump = 0;
    landPulse = 0;
    lastState = 'idle';
    lastStageIndex = 0;
    setText(dom.runChip, 'mock stream');
    stageStates = STAGES.map(() => 'pending');
    refreshStages();
    gateMissing = [];
    verifyLines = [];
    particles = [];
    waves = [];
    anchorCache = { at: 0, list: [] };

    dom.missingWrap.innerHTML = '';
    dom.verifyLines.innerHTML = '';
    dom.verifyCard.classList.remove('is-visible');
    dom.log.innerHTML = '';
    dom.gateCard.classList.add('is-visible');
    dom.gateCard.dataset.state = 'idle';
    setText(dom.gateCode, '—');
    setText(dom.gateTitle, '等待运行');
    setText(dom.gateState, '待命');
    setText(dom.gateHint, '门禁状态会在这里展开；触手只锁定当前错误。');
    setStatePill('idle');
    if (dom.fault) dom.fault.hidden = true;

    activity = new Activity();
    mixer = new PoseMixer();
    mixer.snap('idle');
    player = new ScenarioPlayer(buildScenario(mode), { speed });
    player.start(performance.now());
    currentSnap = activity.snapshot();

    legs = [];
    for (let index = 0; index < LEGS; index += 1) {
      legs.push({
        chain: new VerletChain({
          segments: 20,
          segment: 20,
          gravity: 2350,
          damping: 0.976,
          wind: 4800,
          pullStrength: 4,
          seed: index * 1.7,
          x: window.innerWidth * 0.5,
          y: window.innerHeight * 0.5,
        }),
        tone: 'idle',
        target: null,
      });
    }

    posX = new Pointer(window.innerWidth * 0.5);
    posY = new Pointer(window.innerHeight * 0.5);
    const start = targetPosition();
    posX.snap(start.x);
    posY.snap(start.y);
    for (const leg of legs) {
      leg.chain.reset(start.x, start.y);
    }

    setStage(0, 'active');
    if (dom.modeBtn) dom.modeBtn.textContent = mode === 'green' ? '切到 RED' : '切到 GREEN';
    if (dom.pauseBtn) dom.pauseBtn.textContent = '暂停';
    addLog('BOOT', `scenario ${mode} · ${STAGE_COUNT} stages · mock event bus`, 'info');
    addLog('RUN', 'data-workflow change · cyber-gate', 'info');
    startLoop();
  }

  /** One bus handler: the scene never reads the runner directly. */
  function applyEvent(type, payload) {
    const data = payload || {};
    activity.ingest({ type, data }, player.clock);
    if (mixer && POSES[activity.state]) mixer.set(activity.state);

    switch (type) {
      case 'run.start':
        setStatePill('running');
        setText(dom.runChip, `${data.workflow || 'change'} · ${data.feature || 'run'}`);
        addLog(
          'RUN',
          `${data.workflow || 'change'} · ${data.feature || 'run'} · ${data.raw || ''}`,
          'info',
        );
        break;

      case 'stage.enter': {
        const index = data.index || 0;
        if (lastStageIndex !== index) {
          markStagePassed(lastStageIndex);
          lastStageIndex = index;
        }
        setStage(index, 'active');
        landPulse = 1;
        const node = $$('.stage-node', dom.track)[index];
        if (node) {
          const rect = node.getBoundingClientRect();
          if (rect.width) shockwave(rect.left + rect.width / 2, rect.top + rect.height / 2, '#38e1ff', 0.7);
        }
        addLog('STAGE', `${index + 1}/${STAGE_COUNT} ${STAGES[index].name} · ${STAGES[index].gate}`, 'stage');
        showGate(STAGES[index], '进入阶段', STAGES[index].hint, 'running');
        if (dom.gateCard) dom.gateCard.dataset.state = 'running';
        break;
      }

      case 'tool.call':
        addLog('TOOL', `${data.name || 'tool'} · ${data.detail || ''}`, 'tool');
        break;

      case 'tool.result':
        addLog(data.ok === false ? 'ERR' : 'OK', `${data.name || 'tool'} ${data.ok === false ? 'failed' : 'done'}`, data.ok === false ? 'fail' : 'ok');
        break;

      case 'log.line':
        addLog(data.tag || 'READ', data.text || '', 'read');
        break;

      case 'gate.blocked': {
        const stage = STAGES[currentSnap.stageIndex] || STAGES[0];
        setStage(currentSnap.stageIndex, 'blocked');
        showGate(stage, '门禁拦截', '触手正在锁定缺失项', 'blocked');
        if (dom.gateCard) dom.gateCard.dataset.state = 'blocked';
        const cardRect = dom.gateCard.getBoundingClientRect();
        shockwave(cardRect.left + cardRect.width / 2, cardRect.top + 20, '#ffb347', 1.3);
        addMissing(Array.isArray(data.missing) ? data.missing : []);
        addLog(
          'GATE',
          `拦截 · 缺 ${(data.missing || []).map((item) => item.label).join('、')}`,
          'warn',
        );
        break;
      }

      case 'anchor.fix':
        fixMissing(data.anchor);
        addLog('FIX', `${data.label || data.anchor} 已补`, 'ok');
        break;

      case 'gate.passed':
        passGate();
        markStagePassed(currentSnap.stageIndex);
        showGate(STAGES[currentSnap.stageIndex], '门禁通过', '证据已写入，推进下一阶段', 'passed');
        if (dom.gateCard) dom.gateCard.dataset.state = 'passed';
        addLog('PASS', `阶段 ${currentSnap.stageIndex + 1} 门禁通过`, 'ok');
        break;

      case 'verify.start':
        showVerify(data.cmd);
        setStage(4, 'active');
        setStatePill('verify');
        showGate(
          STAGES[4],
          '质量门',
          '逐条核对 verify.log；退出码决定是否放行',
          'verify',
        );
        addLog('VERIFY', data.cmd || 'verify', 'verify');
        break;

      case 'verify.line':
        addVerifyLine(data.text || '', data.ok !== false);
        addLog(data.ok === false ? 'FAIL' : 'PASS', data.text || '', data.ok === false ? 'fail' : 'ok');
        break;

      case 'verify.exit': {
        const code = Number(data.code);
        setVerifyResult(code);
        const rect = dom.verifyCard.getBoundingClientRect();
        if (code === 0) {
          markStagePassed(4);
          showGate(STAGES[4], '质量门通过', 'verify exit 0 · 允许提交', 'passed');
          confetti(lastBody.x, lastBody.y);
          shockwave(rect.left + rect.width / 2, rect.top + 30, '#46e6a0', 1.4);
          spawnText('GATE CLEARED', lastBody.x, lastBody.y - 60, '#46e6a0', 18);
          addLog('PASS', `verify exit 0 · 质量门通过`, 'ok');
        } else {
          stageStates[4] = 'failed';
          refreshStages();
          slump = 1;
          showGate(STAGES[4], '质量门失败', `verify exit ${code}`, 'fail');
          shockwave(rect.left + rect.width / 2, rect.top + 30, '#ff4d6d', 1.8);
          spawnText('RED', lastBody.x, lastBody.y - 50, '#ff4d6d', 26);
          addLog('FAIL', `verify exit ${code} · 模型瘫软`, 'fail');
        }
        break;
      }

      case 'run.done':
        setStatePill('done');
        stageStates = STAGES.map(() => 'passed');
        refreshStages();
        showGate(STAGES[5], '已提交', 'run complete · 证据与提交记录已落盘', 'passed');
        addLog('DONE', `run complete · ${data.feature || ''}`, 'ok');
        break;

      case 'run.aborted':
        setStatePill('fail');
        stageStates[currentSnap.stageIndex] = 'failed';
        refreshStages();
        showGate(STAGES[currentSnap.stageIndex], '运行中止', data.reason || 'run aborted', 'fail');
        addLog('ABORT', data.reason || 'run aborted', 'fail');
        break;

      default:
        break;
    }

    anchorCache.at = 0;
    currentSnap = activity.snapshot();
    refreshMeters(currentSnap);
  }

  /* ── targets and anchors ─────────────────────────────────────────────── */

  function rectOf(el) {
    if (!el || !el.isConnected) return null;
    const rect = el.getBoundingClientRect();
    if (rect.width < 1 && rect.height < 1) return null;
    // A target that scrolls out of the viewport is not a usable perch: the
    // tentacle falls back to the idle orbit instead of reaching off-screen.
    if (
      rect.right < -24 ||
      rect.left > window.innerWidth + 24 ||
      rect.bottom < -24 ||
      rect.top > window.innerHeight + 24
    ) {
      return null;
    }
    return {
      x: rect.left + rect.width / 2,
      y: rect.top + rect.height / 2,
      w: rect.width,
      h: rect.height,
    };
  }

  function toneOfStage(index) {
    const state = stageStates[index];
    if (state === 'failed') return 'fail';
    if (state === 'blocked') return 'blocked';
    if (state === 'passed') return 'pass';
    if (state === 'active') return 'running';
    return 'idle';
  }

  function collectAnchors(now) {
    if (now - anchorCache.at < 120 && anchorCache.list.length) {
      const live = anchorCache.list.filter((item) => item.el && item.el.isConnected);
      if (live.length) return live;
    }
    const list = [];
    const seen = new Set();
    const push = (el, tone, kind) => {
      if (!el || seen.has(el)) return;
      const rect = rectOf(el);
      if (!rect) return;
      seen.add(el);
      list.push({ el, tone, kind, x: rect.x, y: rect.y });
    };

    for (const entry of gateMissing) {
      if (!entry.fixed) push(entry.el, 'fail', 'missing');
    }
    for (const line of verifyLines) {
      if (!line.ok) push(line.el, 'fail', 'verify-line');
    }
    if (dom.verifyCard.classList.contains('is-visible')) {
      push(dom.verifyCard, 'verify', 'verify-card');
    }
    const gateTone =
      dom.gateCard.dataset.state === 'passed'
        ? 'pass'
        : dom.gateCard.dataset.state === 'blocked'
          ? 'blocked'
          : dom.gateCard.dataset.state === 'running'
            ? 'running'
            : 'idle';
    push(dom.gateCard, gateTone, 'gate');
    const activeNode = $$('.stage-node', dom.track)[currentSnap.stageIndex];
    push(activeNode, toneOfStage(currentSnap.stageIndex), 'stage');
    // Deliberately no log-row anchor: a thread from the body to the far-right
    // log panel draws a line across the whole viewport and reads as noise.

    anchorCache = { at: now, list };
    return list;
  }

  function targetPosition() {
    const nodes = $$('.stage-node', dom.track);
    const node = nodes[currentSnap.stageIndex] || nodes[0];
    if (!node) return { x: window.innerWidth / 2, y: window.innerHeight / 2 };
    const nodeRect = node.getBoundingClientRect();
    const trackRect = dom.track.getBoundingClientRect();
    // Stay near the centre column while drifting with the active stage, so the
    // creature crawls *over* the page instead of hiding behind a panel.
    let x = nodeRect.left + nodeRect.width / 2;
    x = x * 0.45 + window.innerWidth * 0.5 * 0.55;
    const y =
      trackRect.bottom +
      Math.max(96, Math.min(168, (window.innerHeight - trackRect.bottom) * 0.3));

    if (currentSnap.state === 'blocked' || currentSnap.state === 'verify') {
      const cardRect = (currentSnap.state === 'verify' ? dom.verifyCard : dom.gateCard).getBoundingClientRect();
      if (cardRect.width) x = x * 0.62 + (cardRect.left + cardRect.width / 2) * 0.38;
    }
    return {
      x: clamp(x, 110, window.innerWidth - 110),
      y: clamp(y, 110, window.innerHeight - 110),
    };
  }

  /* ── update ──────────────────────────────────────────────────────────── */

  function updateBody(dt, now) {
    const target = targetPosition();
    posX.moveTo(target.x);
    posY.moveTo(target.y);
    const x = posX.step(dt);
    const y = posY.step(dt);
    const t = now / 1000;
    const pose = mixer.step(dt);
    const tremor = reducedMotion ? 0 : currentSnap.tremor;
    const bob = reducedMotion
      ? 0
      : Math.sin(t * (1.9 + currentSnap.stress * 2.4)) *
        (2.4 + currentSnap.stress * 4.2) *
        pose.bob;

    lastBody = {
      x: x + Math.sin(t * 36) * tremor * 0.5,
      y: y + bob + Math.sin(t * 41) * tremor * 0.4 + slump * 12,
      tilt: pose.tilt,
      sx: pose.sx,
      sy: pose.sy,
    };

    if (landPulse > 0) landPulse = Math.max(0, landPulse - dt * 3.4);
    if (slump > 0) slump = Math.max(0, slump - dt * 0.42);
  }

  function updateLegs(dt, now) {
    const anchors = collectAnchors(now);
    const t = now / 1000;
    const body = lastBody;
    const strengthBase =
      currentSnap.state === 'blocked'
        ? 28
        : currentSnap.state === 'verify'
          ? 22
          : currentSnap.state === 'fail'
            ? 5
            : 7;
    const gravity = currentSnap.state === 'fail' || slump > 0.25 ? 3600 : 2350;
    const damping = currentSnap.state === 'fail' || slump > 0.25 ? 0.94 : 0.976;

    for (let index = 0; index < legs.length; index += 1) {
      const leg = legs[index];
      const angle = Math.PI * (0.16 + (0.68 * index) / (legs.length - 1));
      const hip = {
        x: body.x + Math.cos(angle) * 44,
        y: body.y + Math.sin(angle) * 24 + 12,
      };
      leg.chain.setRoot(hip.x, hip.y);

      if (anchors.length) {
        const target = anchors[index % anchors.length];
        leg.tone = target.tone;
        leg.target = {
          x: target.x + Math.cos(t * 0.7 + index) * 2,
          y: target.y + Math.sin(t * 0.9 + index) * 2,
        };
        leg.chain.setTarget(
          leg.target,
          strengthBase * (target.kind === 'missing' ? 1.35 : 1),
        );
      } else {
        leg.tone = 'idle';
        leg.target = {
          x: body.x + Math.cos(t * 0.7 + index * 1.3) * (78 + Math.sin(t * 1.7 + index) * 16),
          y: body.y + 96 + Math.sin(t * 0.9 + index * 0.8) * 26,
        };
        leg.chain.setTarget(leg.target, 2.6);
      }

      leg.chain.step(dt * currentSnap.timeScale, { gravity, damping });
    }
  }

  function updateParticles(dt) {
    const gravity = 38;
    particles = particles.filter((item) => {
      item.life += dt;
      item.x += item.vx * dt;
      item.y += item.vy * dt;
      if (item.kind === 'shard') item.vy += gravity * dt;
      else item.vy -= 3 * dt;
      return item.life < item.ttl;
    });
    waves = waves.filter((item) => {
      item.life += dt;
      item.radius += (220 + item.power * 130) * dt;
      return item.life < item.ttl;
    });
  }

  function refreshMeters(snap) {
    setText(dom.stressVal, snap.stress.toFixed(2));
    setText(dom.urgencyVal, snap.urgency.toFixed(2));
    setText(dom.tickVal, `${snap.timeScale.toFixed(2)}×`);
    setText(dom.tremorVal, snap.tremor.toFixed(2));
    if (dom.stressBar) dom.stressBar.style.width = `${Math.round(snap.stress * 100)}%`;
    if (dom.urgencyBar) dom.urgencyBar.style.width = `${Math.round(snap.urgency * 100)}%`;
    if (dom.stressBar) dom.stressBar.dataset.level = snap.stress > 0.66 ? 'high' : snap.stress > 0.33 ? 'mid' : 'low';
    if (dom.urgencyBar) dom.urgencyBar.dataset.level = snap.urgency > 0.66 ? 'high' : snap.urgency > 0.33 ? 'mid' : 'low';
    setText(dom.clock, formatClock(player ? player.clock : 0));
  }

  /* ── drawing ─────────────────────────────────────────────────────────── */

  function resizeCanvas() {
    const dpr = Math.min(2, Math.max(1, window.devicePixelRatio || 1));
    dom.canvas.width = Math.round(window.innerWidth * dpr);
    dom.canvas.height = Math.round(window.innerHeight * dpr);
    dom.canvas.style.width = `${window.innerWidth}px`;
    dom.canvas.style.height = `${window.innerHeight}px`;
  }

  function drawWeb(anchors, t) {
    const body = lastBody;
    for (const anchor of anchors) {
      const dx = anchor.x - body.x;
      const dy = anchor.y - body.y;
      const length = Math.hypot(dx, dy) || 1;
      const offset = Math.sin(anchor.x * 0.01 + t * 0.5) * Math.min(70, length * 0.28);
      const c1 = {
        x: body.x + dx * 0.32 - (dy / length) * offset,
        y: body.y + dy * 0.32 + (dx / length) * offset,
      };
      const c2 = {
        x: body.x + dx * 0.7 - (dy / length) * offset * 0.5,
        y: body.y + dy * 0.7 + (dx / length) * offset * 0.5,
      };
      const color = PALETTE[anchor.tone] || PALETTE.idle;

      ctx.save();
      ctx.strokeStyle = color;
      ctx.globalAlpha = anchor.kind === 'log' ? 0.08 : 0.16;
      ctx.lineWidth = 1;
      ctx.shadowColor = color;
      ctx.shadowBlur = 6;
      ctx.beginPath();
      ctx.moveTo(body.x, body.y);
      ctx.bezierCurveTo(c1.x, c1.y, c2.x, c2.y, anchor.x, anchor.y);
      ctx.stroke();
      ctx.restore();

      for (let p = 0; p < 2; p += 1) {
        const progress = (t * 0.3 + anchor.x * 0.001 + p * 0.5) % 1;
        const u = 1 - progress;
        const px =
          u * u * u * body.x +
          3 * u * u * progress * c1.x +
          3 * u * progress * progress * c2.x +
          progress * progress * progress * anchor.x;
        const py =
          u * u * u * body.y +
          3 * u * u * progress * c1.y +
          3 * u * progress * progress * c2.y +
          progress * progress * progress * anchor.y;
        ctx.save();
        ctx.fillStyle = color;
        ctx.globalAlpha = 0.6;
        ctx.shadowColor = color;
        ctx.shadowBlur = 8;
        ctx.beginPath();
        ctx.arc(px, py, 1.7, 0, Math.PI * 2);
        ctx.fill();
        ctx.restore();
      }

      ctx.save();
      ctx.strokeStyle = color;
      ctx.fillStyle = 'rgba(5, 12, 24, 0.9)';
      ctx.globalAlpha = 0.85;
      ctx.shadowColor = color;
      ctx.shadowBlur = 10;
      ctx.beginPath();
      ctx.arc(anchor.x, anchor.y, 3.2, 0, Math.PI * 2);
      ctx.fill();
      ctx.stroke();
      ctx.restore();
    }
  }

  function drawLegs(t) {
    for (let index = 0; index < legs.length; index += 1) {
      const leg = legs[index];
      const points = leg.chain.points;
      if (!points.length) continue;
      const color = PALETTE[leg.tone] || PALETTE.idle;
      const isLocked =
        leg.target &&
        Math.hypot(points[points.length - 1].x - leg.target.x, points[points.length - 1].y - leg.target.y) < 18;

      ctx.save();
      ctx.strokeStyle = color;
      ctx.globalAlpha = 0.82;
      ctx.lineWidth = index % 2 === 0 ? 1.7 : 1.2;
      ctx.lineCap = 'round';
      ctx.lineJoin = 'round';
      ctx.shadowColor = color;
      ctx.shadowBlur = 8;
      ctx.beginPath();
      ctx.moveTo(points[0].x, points[0].y);
      for (let p = 1; p < points.length; p += 1) {
        const prev = points[p - 1];
        const curr = points[p];
        const midX = (prev.x + curr.x) / 2;
        const midY = (prev.y + curr.y) / 2;
        ctx.quadraticCurveTo(prev.x, prev.y, midX, midY);
      }
      const tip = points[points.length - 1];
      ctx.lineTo(tip.x, tip.y);
      ctx.stroke();
      ctx.restore();

      // Joint beads keep the chain readable without an animation sprite.
      ctx.save();
      for (let p = 4; p < points.length - 1; p += 4) {
        const point = points[p];
        ctx.fillStyle = color;
        ctx.globalAlpha = 0.45;
        ctx.beginPath();
        ctx.arc(point.x, point.y, 1.5, 0, Math.PI * 2);
        ctx.fill();
      }
      ctx.restore();

      if (isLocked) {
        const target = leg.target;
        ctx.save();
        ctx.strokeStyle = color;
        ctx.globalAlpha = 0.55;
        ctx.lineWidth = 1.2;
        ctx.shadowColor = color;
        ctx.shadowBlur = 9;
        ctx.beginPath();
        ctx.arc(target.x, target.y, 8 + Math.sin(t * 5 + index) * 1.8, 0, Math.PI * 2);
        ctx.stroke();
        ctx.restore();
      }

      const hip = points[0];
      ctx.save();
      ctx.fillStyle = color;
      ctx.shadowColor = color;
      ctx.shadowBlur = 7;
      ctx.beginPath();
      ctx.arc(hip.x, hip.y, 2.2, 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();
    }
  }

  function drawBody(t) {
    const body = lastBody;
    const color = PALETTE[currentSnap.state] || PALETTE.idle;
    const pulse = 1 + Math.sin(t * 2.6) * 0.02;

    ctx.save();
    const halo = ctx.createRadialGradient(body.x, body.y, 8, body.x, body.y, 150);
    halo.addColorStop(0, `${color}33`);
    halo.addColorStop(0.55, `${color}12`);
    halo.addColorStop(1, 'rgba(0,0,0,0)');
    ctx.fillStyle = halo;
    ctx.beginPath();
    ctx.arc(body.x, body.y, 150, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();

    const bounce = landPulse * 0.09;
    const height = window.innerWidth < 860 ? 128 : 172;
    const width = (height * 336) / 384;

    ctx.save();
    ctx.translate(body.x, body.y + Math.sin(t * 3.1) * 1.5);
    ctx.rotate(body.tilt);
    ctx.scale(body.sx * (1 + bounce) * pulse, body.sy * (1 - bounce * 0.7) * pulse);
    ctx.shadowColor = color;
    ctx.shadowBlur = 18;

    if (imageReady) {
      ctx.globalAlpha = 0.98;
      ctx.drawImage(bodyImage, -width / 2, -height * 0.58, width, height);
    } else {
      // No artwork loaded: draw a neon whale core so the demo still runs.
      ctx.fillStyle = 'rgba(10, 22, 40, 0.95)';
      ctx.strokeStyle = color;
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.ellipse(0, 0, 54, 38, 0, 0, Math.PI * 2);
      ctx.fill();
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(48, 0);
      ctx.quadraticCurveTo(88, -26, 96, 0);
      ctx.quadraticCurveTo(88, 26, 48, 0);
      ctx.fill();
      ctx.stroke();
      ctx.fillStyle = '#d8fbff';
      ctx.beginPath();
      ctx.arc(20, -8, 3.2, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.restore();

    // A small ring marks the tentacle root without hiding the artwork.
    ctx.save();
    ctx.strokeStyle = color;
    ctx.globalAlpha = 0.45;
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.arc(body.x, body.y + 16, 26 + Math.sin(t * 2.2) * 2, 0, Math.PI * 2);
    ctx.stroke();
    ctx.restore();
  }

  function drawParticles() {
    for (const item of particles) {
      const fade = Math.max(0, 1 - item.life / item.ttl);
      ctx.save();
      ctx.globalAlpha = fade;
      if (item.kind === 'text') {
        ctx.fillStyle = item.color;
        ctx.shadowColor = item.color;
        ctx.shadowBlur = 10;
        ctx.font = `700 ${item.size}px "SF Mono", ui-monospace, Menlo, Consolas, monospace`;
        ctx.fillText(item.text, item.x, item.y);
      } else {
        ctx.fillStyle = item.color;
        ctx.shadowColor = item.color;
        ctx.shadowBlur = 6;
        ctx.beginPath();
        ctx.arc(item.x, item.y, item.size, 0, Math.PI * 2);
        ctx.fill();
      }
      ctx.restore();
    }

    for (const wave of waves) {
      const fade = Math.max(0, 1 - wave.life / wave.ttl);
      ctx.save();
      ctx.strokeStyle = wave.color;
      ctx.globalAlpha = fade * 0.75;
      ctx.lineWidth = 1 + fade * 2;
      ctx.shadowColor = wave.color;
      ctx.shadowBlur = 12;
      ctx.beginPath();
      ctx.arc(wave.x, wave.y, wave.radius, 0, Math.PI * 2);
      ctx.stroke();
      ctx.restore();
    }
  }

  function draw(now) {
    const dpr = Math.min(2, Math.max(1, window.devicePixelRatio || 1));
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, window.innerWidth, window.innerHeight);

    const t = now / 1000;
    const anchors = collectAnchors(now);
    drawWeb(anchors, t);
    drawLegs(t);
    drawBody(t);
    drawParticles();
  }

  /* ── loop ────────────────────────────────────────────────────────────── */

  function frame(ts) {
    raf = 0;
    if (stopped) return;
    try {
      if (paused) {
        lastTs = ts;
        raf = requestAnimationFrame(frame);
        return;
      }
      const dt = lastTs ? Math.min(0.05, (ts - lastTs) / 1000) : 1 / 60;
      lastTs = ts;

      if (player) {
        for (const ev of player.tick(ts)) bus.emit(ev.type, ev.data || {});
      }

      currentSnap = activity.tick(dt, player ? player.clock : 0);
      if (mixer && POSES[currentSnap.state]) mixer.set(currentSnap.state);
      if (currentSnap.state !== lastState) {
        lastState = currentSnap.state;
        setStatePill(currentSnap.state);
      }

      updateBody(dt, ts);
      updateLegs(dt, ts);
      updateParticles(dt);
      draw(ts);
      refreshMeters(currentSnap);
    } catch (error) {
      errors += 1;
      if (typeof console !== 'undefined' && console.warn) {
        console.warn('[cyber-web] frame failed:', error);
      }
      if (errors > 6) {
        stopped = true;
        if (dom.fault) dom.fault.hidden = false;
        if (raf) cancelAnimationFrame(raf);
        raf = 0;
        return;
      }
    }
    raf = requestAnimationFrame(frame);
  }

  function startLoop() {
    if (!raf && !stopped) raf = requestAnimationFrame(frame);
  }

  /* ── controls ────────────────────────────────────────────────────────── */

  function togglePause() {
    paused = !paused;
    if (paused) {
      player.pause(performance.now());
      if (dom.pauseBtn) dom.pauseBtn.textContent = '继续';
      addLog('PAUSE', '时间线已暂停', 'info');
    } else {
      player.resume(performance.now());
      lastTs = 0;
      if (dom.pauseBtn) dom.pauseBtn.textContent = '暂停';
      addLog('RESUME', '时间线继续', 'info');
    }
  }

  function cycleSpeed() {
    const options = [0.5, 1, 1.5, 2];
    const index = options.indexOf(speed);
    speed = options[(index + 1) % options.length];
    if (player) player.setSpeed(speed);
    if (dom.speedBtn) dom.speedBtn.textContent = `速度 ${speed}×`;
  }

  function bindControls() {
    if (dom.replayBtn) {
      dom.replayBtn.addEventListener('click', () => resetDemo(mode));
    }
    if (dom.modeBtn) {
      dom.modeBtn.addEventListener('click', () => {
        resetDemo(mode === 'green' ? 'red' : 'green');
      });
    }
    if (dom.speedBtn) {
      dom.speedBtn.textContent = `速度 ${speed}×`;
      dom.speedBtn.addEventListener('click', cycleSpeed);
    }
    if (dom.pauseBtn) {
      dom.pauseBtn.addEventListener('click', togglePause);
    }
    window.addEventListener('keydown', (event) => {
      if (event.target && /^(INPUT|TEXTAREA|BUTTON)$/.test(event.target.tagName)) return;
      if (event.code === 'Space') {
        event.preventDefault();
        togglePause();
      } else if (event.key === 'r' || event.key === 'R') {
        resetDemo(mode);
      }
    });
    window.addEventListener('resize', resizeCanvas);
    document.addEventListener('visibilitychange', () => {
      if (document.hidden) {
        if (raf) cancelAnimationFrame(raf);
        raf = 0;
      } else {
        lastTs = 0;
        startLoop();
      }
    });
  }

  /**
   * Deterministically jump the mock timeline to `at` ms, then settle the
   * physics for a few fixed steps and freeze. Used for screenshots and debug
   * replay (`?at=2500&mode=red`); it never waits on wall-clock time.
   */
  function seek(at) {
    paused = false;
    if (!player) return;
    for (const ev of player.skipTo(at)) bus.emit(ev.type, ev.data || {});
    currentSnap = activity.tick(0, player.clock);
    if (mixer && POSES[currentSnap.state]) mixer.snap(currentSnap.state);
    mixer.step(1 / 30);
    mixer.step(1 / 30);
    // Let the tentacles reach their new anchors and the pointer settle.
    const stepMs = 1000 / 60;
    for (let i = 0; i < 90; i += 1) {
      const now = performance.now() + i * stepMs;
      updateBody(stepMs / 1000, now);
      updateLegs(stepMs / 1000, now);
      updateParticles(stepMs / 1000);
    }
    anchorCache.at = 0;
    draw(performance.now());
    refreshMeters(currentSnap);
    setStatePill(currentSnap.state);
    player.pause(performance.now());
    paused = true;
    if (dom.pauseBtn) dom.pauseBtn.textContent = '继续';
  }

  /* ── boot ────────────────────────────────────────────────────────────── */

  const asset =
    typeof window.__CW_ASSET__ === 'string' && window.__CW_ASSET__
      ? window.__CW_ASSET__
      : 'assets/pet-idle.png';
  bodyImage = new Image();
  bodyImage.onload = () => {
    imageReady = true;
    // A `?at=` freeze draws before the data URL decodes; repaint once it does.
    if (paused) draw(performance.now());
  };
  bodyImage.onerror = () => {
    imageReady = false;
    if (dom.fault) {
      dom.fault.hidden = false;
      dom.fault.textContent =
        '角色立绘未加载：演示继续使用内置霓虹鲸轮廓。把 assets/pet-idle.png 放到同目录，或运行构建脚本内联图片。';
    }
  };
  bodyImage.src = asset;

  buildTrack();
  bindControls();
  bus = new EventBus();
  bus.on('*', (payload, type) => applyEvent(type, payload));
  resizeCanvas();

  function readParams() {
    try {
      const search = (window.location && window.location.search) || '';
      const params = new URLSearchParams(search);
      return {
        mode: params.get('mode') === 'red' ? 'red' : 'green',
        at: Number(params.get('at')),
      };
    } catch {
      return { mode: 'green', at: Number.NaN };
    }
  }

  const initial = readParams();
  resetDemo(initial.mode);
  startLoop();
  if (Number.isFinite(initial.at) && initial.at > 0) {
    // One frame first: the anchors need layout before the physics can settle.
    requestAnimationFrame(() => seek(initial.at));
  }

  if (reducedMotion && dom.arena) {
    dom.arena.classList.add('is-reduced');
  }

  // Exposed for debugging only; the workflow never calls into this module.
  window.cyberWebDemo = {
    reset: resetDemo,
    seek,
    pause: () => {
      if (!paused) togglePause();
    },
    resume: () => {
      if (paused) togglePause();
    },
    get snapshot() {
      return { ...currentSnap, clock: player ? player.clock : 0, mode, speed };
    },
    get debug() {
      return {
        legs: legs.length,
        anchors: anchorCache.list.length,
        particles: particles.length,
        waves: waves.length,
        gateMissing: gateMissing.length,
        verifyLines: verifyLines.length,
        imageReady,
      };
    },
    get body() {
      return { ...lastBody };
    },
  };
}

initCyberDemo();
