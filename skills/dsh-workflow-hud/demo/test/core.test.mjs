import test from 'node:test';
import assert from 'node:assert/strict';
import {
  Activity,
  EventBus,
  POSES,
  Pointer,
  PoseMixer,
  ScenarioPlayer,
  STAGES,
  VerletChain,
  buildScenario,
  clamp,
  formatClock,
  lerp,
} from '../src/core.js';

test('clamp, lerp, and the run clock are stable on bad input', () => {
  assert.equal(clamp(5, 0, 1), 1);
  assert.equal(clamp(-5, 0, 1), 0);
  assert.equal(clamp(Number.NaN, 0, 1), 0);
  assert.equal(lerp(0, 10, 0.25), 2.5);
  assert.equal(formatClock(0), '00:00.0');
  assert.equal(formatClock(61_950), '01:01.9');
  assert.equal(formatClock(-5), '00:00.0');
  assert.equal(formatClock('bad'), '00:00.0');
});

test('EventBus isolates listener failures and supports the wildcard', () => {
  const bus = new EventBus();
  const seen = [];
  const off = bus.on('gate', (payload) => seen.push(`gate:${payload}`));
  bus.on('*', (payload, type) => seen.push(`${type}:${payload}`));
  bus.on('gate', () => {
    throw new Error('listener bug');
  });

  assert.equal(bus.emit('gate', 'blocked'), 2);
  assert.deepEqual(seen.sort(), ['gate:blocked', 'gate:blocked']);
  off();
  assert.equal(bus.emit('gate', 'passed'), 1);
});

test('Activity folds the workflow stream into stress, urgency, and tempo', () => {
  const activity = new Activity();

  activity.ingest({ type: 'run.start', data: {} }, 0);
  assert.equal(activity.state, 'running');

  const blocked = activity.ingest(
    {
      type: 'gate.blocked',
      data: { missing: [{ label: '未知' }] },
    },
    1000,
  );
  assert.equal(blocked.state, 'blocked');
  assert.ok(blocked.stress > 0.2);
  assert.ok(blocked.urgency > 0.5);
  assert.ok(blocked.timeScale > 0.9);

  const failed = activity.ingest({ type: 'verify.exit', data: { code: 3 } }, 2000);
  assert.equal(failed.state, 'fail');
  assert.ok(failed.urgency > 0.8);

  const calmer = activity.tick(2, 4000);
  assert.ok(calmer.stress < failed.stress);
  assert.ok(calmer.urgency < failed.urgency);
});

test('Activity raises error density while errors keep arriving', () => {
  const activity = new Activity();
  activity.ingest({ type: 'run.start', data: {} }, 0);
  for (let i = 0; i < 4; i += 1) {
    activity.ingest(
      { type: 'log.line', data: { text: 'FAIL protected write should be denied' } },
      1000 + i * 100,
    );
  }
  const snap = activity.tick(0.1, 1400);
  assert.ok(snap.urgency >= 0.6);
  assert.ok(activity.errorDensity() > 0.5);
});

test('PoseMixer reaches a new pose in about 0.2s without snapping', () => {
  const mixer = new PoseMixer();
  mixer.set('blocked');
  assert.equal(mixer.targetName, 'blocked');

  const first = mixer.step(1 / 60);
  assert.notDeepEqual(first, POSES.blocked);
  assert.equal(typeof first.sx, 'number');

  for (let i = 0; i < 18; i += 1) mixer.step(1 / 60);
  for (const key of Object.keys(POSES.blocked)) {
    assert.ok(
      Math.abs(mixer.current[key] - POSES.blocked[key]) < 0.12,
      `${key} did not settle: ${mixer.current[key]}`,
    );
  }

  mixer.snap('idle');
  assert.deepEqual(mixer.current, POSES.idle);
});

test('VerletChain keeps the root pinned and stays finite', () => {
  const chain = new VerletChain({
    segments: 8,
    segment: 12,
    gravity: 900,
    damping: 0.98,
    wind: 0,
    seed: 0,
    x: 10,
    y: 20,
  });

  assert.equal(chain.points.length, 9);
  chain.setRoot(30, 40);
  for (let i = 0; i < 30; i += 1) chain.step(1 / 60);
  assert.deepEqual(
    { x: chain.points[0].x, y: chain.points[0].y },
    { x: 30, y: 40 },
  );
  for (const point of chain.points) {
    assert.ok(Number.isFinite(point.x) && Number.isFinite(point.y));
  }
});

test('VerletChain pulls the tip toward a target over time', () => {
  const chain = new VerletChain({
    segments: 12,
    segment: 14,
    gravity: 0,
    damping: 0.99,
    wind: 0,
    seed: 0,
    x: 0,
    y: 0,
  });

  chain.setTarget({ x: 90, y: 60 }, 12);
  const before = Math.hypot(
    chain.points.at(-1).x - 90,
    chain.points.at(-1).y - 60,
  );
  for (let i = 0; i < 90; i += 1) chain.step(1 / 60);
  const after = Math.hypot(
    chain.points.at(-1).x - 90,
    chain.points.at(-1).y - 60,
  );
  assert.ok(after < before, `tip did not approach: ${before} -> ${after}`);
});

test('Pointer springs to its target and can snap', () => {
  const pointer = new Pointer(0);
  pointer.moveTo(100);
  for (let i = 0; i < 60; i += 1) pointer.step(1 / 60);
  assert.ok(Math.abs(pointer.x - 100) < 2);
  pointer.snap(42);
  assert.equal(pointer.x, 42);
  assert.equal(pointer.target, 42);
});

test('ScenarioPlayer releases due events, honors speed, and pauses', () => {
  const events = [
    { at: 100, type: 'b' },
    { at: 0, type: 'a' },
    { at: 200, type: 'c' },
  ];
  const player = new ScenarioPlayer(events, { speed: 2 });
  player.start(1000);

  assert.deepEqual(player.tick(1050).map((event) => event.type), ['a', 'b']);
  assert.equal(player.done, false);

  player.pause(1050);
  assert.deepEqual(player.tick(1200), []);
  player.resume(1200);
  assert.deepEqual(player.tick(1250).map((event) => event.type), ['c']);
  assert.equal(player.done, true);
});

test('buildScenario produces a green run and a red run with the same shape', () => {
  const green = buildScenario('green');
  const red = buildScenario('red');

  assert.ok(green.length > 20 && red.length > 20);
  assert.equal(green.at(-1).type, 'run.done');
  assert.equal(red.at(-1).type, 'run.aborted');

  for (const list of [green, red]) {
    const sorted = list.map((event) => event.at);
    assert.deepEqual(sorted, [...sorted].sort((a, b) => a - b));
    const exit = list.find((event) => event.type === 'verify.exit');
    assert.ok(exit, 'verify.exit is present');
    assert.equal(
      list.filter((event) => event.type === 'verify.line').length,
      5,
      'five verify lines',
    );
  }

  assert.equal(
    green.find((event) => event.type === 'verify.exit').data.code,
    0,
  );
  assert.equal(red.find((event) => event.type === 'verify.exit').data.code, 3);
});

test('STAGES and POSES cover every tone the scene needs', () => {
  assert.equal(STAGES.length, 6);
  for (const stage of STAGES) {
    assert.equal(typeof stage.name, 'string');
    assert.equal(typeof stage.gate, 'string');
  }
  for (const pose of ['idle', 'running', 'blocked', 'verify', 'fail', 'done']) {
    assert.ok(POSES[pose], `missing pose ${pose}`);
  }
});
