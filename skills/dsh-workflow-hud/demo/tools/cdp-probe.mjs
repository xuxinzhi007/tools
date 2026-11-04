// Minimal Chrome DevTools Protocol probe (no puppeteer; Node 26 global WebSocket).
// Usage: node cdp-probe.mjs <url> <out.png> <waitMs>
import { spawn } from 'node:child_process';
import { writeFile } from 'node:fs/promises';

const CHROME = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const PORT = 9229;
const url = process.argv[2];
const out = process.argv[3] || '/tmp/cdp-shot.png';
const waitMs = Number(process.argv[4] || 2500);

const proc = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    '--window-size=1440,900',
    '--hide-scrollbars',
    '--mute-audio',
    'about:blank',
  ],
  { stdio: 'ignore' },
);

async function getJson(p) {
  const r = await fetch(`http://127.0.0.1:${PORT}${p}`);
  return r.json();
}

let targets = null;
for (let i = 0; i < 50; i += 1) {
  try {
    targets = await getJson('/json');
    if (targets && targets.length) break;
  } catch {
    // not ready yet
  }
  await new Promise((r) => setTimeout(r, 200));
}
if (!targets) throw new Error('devtools endpoint never came up');

const page = targets.find((t) => t.type === 'page');
const ws = new WebSocket(page.webSocketDebuggerUrl);
await new Promise((res, rej) => {
  ws.onopen = res;
  ws.onerror = rej;
});

let id = 0;
const pending = new Map();
ws.onmessage = (ev) => {
  const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) {
    pending.get(m.id)(m);
    pending.delete(m.id);
  }
};
const send = (method, params = {}) =>
  new Promise((res) => {
    const i = (id += 1);
    pending.set(i, res);
    ws.send(JSON.stringify({ id: i, method, params }));
  });

await send('Page.enable');
await send('Runtime.enable');
// collect window errors so a failing seek/draw is visible
await send('Page.addScriptToEvaluateOnNewDocument', {
  source:
    'window.__errs=[];window.addEventListener("error",function(e){window.__errs.push(String((e.error&&e.error.stack)||e.message))});',
});
await send('Page.navigate', { url });
await new Promise((r) => setTimeout(r, waitMs));

const evalExpr = `JSON.stringify({
  snap: window.cyberWebDemo && window.cyberWebDemo.snapshot,
  debug: window.cyberWebDemo && window.cyberWebDemo.debug,
  body: window.cyberWebDemo && window.cyberWebDemo.body,
  errs: window.__errs,
  canvasBox: (function(){var c=document.querySelector('#web');var r=c.getBoundingClientRect();return {w:c.width,h:c.height,left:r.left,top:r.top,cssW:r.width,cssH:r.height,z:getComputedStyle(c).zIndex,pe:getComputedStyle(c).pointerEvents};})(),
})`;
const res = await send('Runtime.evaluate', { expression: evalExpr, returnByValue: true });
console.log('STATE:', res.result?.result?.value ?? JSON.stringify(res.result));

const probe = await send('Runtime.evaluate', {
  expression: `(() => {
    const c = document.querySelector('#web');
    const x = c.getContext('2d');
    const d = x.getImageData(0, 0, c.width, c.height).data;
    let n = 0, maxA = 0;
    for (let i = 3; i < d.length; i += 4) { if (d[i] > 10) n++; if (d[i] > maxA) maxA = d[i]; }
    return JSON.stringify({ nonTransparent: n, maxAlpha: maxA, total: d.length / 4 });
  })()`,
  returnByValue: true,
});
console.log('CANVAS:', probe.result?.result?.value ?? JSON.stringify(probe.result));

const shot = await send('Page.captureScreenshot', { format: 'png' });
await writeFile(out, Buffer.from(shot.result.data, 'base64'));
console.log('saved', out);

ws.close();
proc.kill();
process.exit(0);
