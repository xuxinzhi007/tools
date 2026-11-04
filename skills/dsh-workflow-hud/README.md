# Workflow HUD

A profile-level DeepSeek Harness plugin that floats a workflow-state cockpit
over the Web GUI while a `data-workflow` run is active: all stages, each one's
gate state, required fields, artifacts, and evidence, plus a gate log of the
runner's own verdicts. Raw tool commands are not displayed — the card answers
"which gate is this run at, why, and what happens next".

The card is scoped to **one conversation**. It only shows a `specs/<feature>/run.json`
when that conversation itself started, advanced, or wrote that run — through a
`workflow_next.py` command or a write under `specs/<feature>/`. Run history from
another conversation, including a still-open run, never appears here. A new
conversation therefore starts at “no run” until it invokes the skill.

## What it shows

- The conversation's workflow, feature, current stage, and run state
  (`working` / `waiting` / `done` / `abandoned`).
- The real stage list and per-stage `requires` / `gate_check` from
  `skills/data-workflow/workflows/<workflow>.json`.
- Every stage in the workflow, not only the current one. Each started or
  completed stage expands to its required fields, artifacts, evidence row, and
  gate verdict; not-yet-reached stages stay compact and are never reported as
  missing artifacts.
- A summary verdict for the current stage: passed, missing artifacts, missing
  required fields, open decision, unverified, or code stage awaiting `done`.
- Chips for each required field and each missing artifact.
- The runner's latest `evidence.log` row and the `verify.log` exit code + command.
- Whether protected paths are configured, plus the recent `specs/passes.log`
  approval count. It does **not** claim the gate is enforced when
  `protected_paths` is empty.
- A gate log of the runner's own verdicts — `READ / NEED / WAIT / OPEN / DO /
  DONE / GATE / RED` — each with its `STAGE` line, missing fields (`缺：…`),
  `CHECK`, and `VERIFY exit N` detail. This is the "triggered a gate → stopped
  at the gate with the log → passed → moved to the next stage" trail.
- When this conversation has no run yet, the installed workflow catalog: every
  `change` / `feature` / `bugfix` / `hotfix` / `review-only` definition and its
  full stage chain, read from `skills/data-workflow/workflows/*.json`.

- A full-viewport **cyber web-crawler layer** (toggle `⌁` in the card header):
  an eight-legged procedural spider walks a tour built from the HUD's own
  rendered anchors (stage rows, gate banner, workflow templates, gate-log rows).
  Each anchor is joined to the spider's body by a glowing Bézier thread whose
  colour follows that anchor's gate tone; particles flow along the threads, and
  a gate transition (open/fail/pass) or a new runner verdict fires an expanding
  shockwave. The layer is `pointer-events: none`, draws only in its own canvas,
  and never reads the host page or another plugin's DOM. With
  `prefers-reduced-motion` it renders one static frame instead of animating.

The HUD only observes. It does not write `run.json`, change tool policy, append
session events, or ask the model for status.

## Data flow

```
session/event (tool/call, tool/result, turn/*)
        │
        ├── associate feature from workflow_next.py calls, specs/<feature> writes,
        │   or the runner's own RUN / RECORD lines (reads are ignored)
        ├── recover the same association from this session's own log after reload
        ├── read specs/<feature>/{run.json,evidence.log,verify.log,decision.md,check.md}
        ├── read skills/data-workflow/workflows/<workflow>.json
        ├── read config.yaml protected_paths / waiver_globs and specs/passes.log
        │
        └── SSE /plugins/workflow-hud/events → shell.overlay component
```

The SSE endpoint is loopback-only and rejects a cross-origin `Origin` header.
The gate log carries runner output lines only, never raw tool commands; obvious
`sk-*`, `Bearer`, and GitHub tokens are still redacted before they leave the
Host. Tool events are consumed solely to associate the conversation with its
`specs/<feature>/` run and to trigger a state re-read.

## Important limits

- A stage only advances when the model actually runs `workflow_next.py
  start/next/done/answer`; the skill does not emit events. Without a runner
  call, the card shows the conversation as unlinked instead of guessing.
- `evidence.log` records the literal gate result. Missing evidence is shown as
  unverified, never as green.
- The gate checks literals, not architecture quality. A green stage means the
  required fields were written and the declared commands exited zero; it does
  not prove the design is correct.
- The HUD is DSH-only. The data-workflow skill itself installs into `.cursor`,
  `.claude`, `.qoder`, `.codex`, and `.agents`; the enforced write hook exists
  only for Cursor. A Cursor visualization would consume the same files through
  a different renderer.

## Install

The bundle is installed into the `desktop` profile from this directory and
linked as `@local/dsh-workflow-hud`:

```text
/Users/xuxinzhi/skills/dsh-workflow-hud
```

Reinstall with the Harness plugin manager by passing this directory to
`install_bundle`. Replacing an installed package loads new Host code only after
the entry restarts, and a page refresh is required for the browser module.

## Test

```bash
npm test
```

The suite covers the pure data helpers and `analyzeGates` against temporary
repositories: missing required fields, an open decision, a passing evidence row,
a non-zero verify, the code stage, protected-path configuration, the installed
workflow catalog, and verdict parsing including `VERIFY` lines. Render tests
assert the card shows the stage/gate view with no command trace, that the
crawler canvas exists, that every stage / gate / gate-log row exposes an anchor
with its tone, and that switching the layer off keeps the card intact.

## Cyber web-crawler implementation

- **Anchors** — the card marks its own nodes with `data-dwh-anchor` and
  `data-dwh-tone`. The canvas reads exactly those elements; it never queries the
  host application or other plugins.
- **Gait** — eight legs, two bones each, solved with the law-of-cosines
  (`crawlerIK` in `client.js`). Feet target a per-leg home point and ease back
  when displaced, producing a stepping gait instead of a canned animation.
- **Web** — for every anchor the canvas draws a cubic Bézier from body to
  anchor, with gate-tone colour, dash animation for open/failing gates, and two
  travelling particles per thread.
- **Events** — a change in the current gate tone, or a new `NEED / GATE / OPEN /
  DO / DONE` verdict, spawns a shockwave at the gate anchor; passing gates ripple
  across the remaining anchors.
- **Lifecycle** — the effect runs only while the card is expanded and the layer
  is enabled; the toggle persists in `localStorage`; resize and
  `visibilitychange` are handled; unmount cancels the animation frame.
