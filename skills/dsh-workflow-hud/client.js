window.__ModuleLoader__.load({
  id: '@local/dsh-workflow-hud',
  factory(require) {
    const React = require('react');
    const h = React.createElement;
    const { useEffect, useRef, useState } = React;

    const NS = 'workflow-hud';
    const EN = {
      'status.working': 'Running',
      'status.waiting': 'Waiting',
      'status.done': 'Done',
      'status.abandoned': 'Abandoned',
      'status.idle': 'Listening',
      standby: 'Standby',
      step: 'Stage',
      complete: 'Run complete',
      question: 'Waiting for you',
      collapse: 'Collapse',
      expand: 'Expand',
      title: 'Workflow HUD',
      live: 'Live',
      offline: 'Disconnected',
      noRun: 'No workflow run in this conversation',
      noRunHint: 'Start one with workflow_next.py start. This card then shows the stage gate, required fields, evidence, and verify result.',
      requires: 'Required',
      artifacts: 'Artifacts',
      writeGate: 'Write gate',
      notProtected: 'No protected paths configured',
      protectedPaths: 'protected',
      passes: 'approvals',
      evidence: 'Evidence',
      gatePass: 'Gate passed',
      gateFail: 'Gate failed',
      gateOpen: 'Decision pending',
      gateWaiting: 'Artifacts pending',
      gateWorking: 'Code stage — run done',
      gateUnverified: 'Not verified yet',
      gateReady: 'Ready to advance',
      gateNone: 'No run',
      historyRun: 'history',
      assocSession: 'this conversation',
      assocRepo: 'repo',
      assocMissing: 'record missing',
      assocNone: 'unlinked',
      runActive: 'active',
      runFinished: 'finished',
      runAbandoned: 'abandoned',
      runNone: 'idle',
      verdicts: 'Gate log',
      noVerdicts: 'No gate events yet',
      nextStep: 'Next step',
      webOn: 'Show the cyber web crawler',
      webOff: 'Hide the cyber web crawler',
      gatePending: 'Not started',
      stageList: 'Workflow stages',
      catalog: 'Installed workflows',
    };
    const ZH = {
      'status.working': '运行中',
      'status.waiting': '等待回复',
      'status.done': '已完成',
      'status.abandoned': '已放弃',
      'status.idle': '监听中',
      standby: '待命',
      step: '阶段',
      complete: '运行完成',
      question: '等你拍板',
      collapse: '收起',
      expand: '展开',
      title: '工作流 HUD',
      live: '已连接',
      offline: '连接断开',
      noRun: '本次对话没有关联工作流运行',
      noRunHint: '用 workflow_next.py start 启动运行后，这里会显示阶段门、必填项、evidence 和 verify 结果。',
      requires: '必填',
      artifacts: '产物',
      writeGate: '写入门',
      notProtected: '未配置受保护路径',
      protectedPaths: '保护路径',
      passes: '放行记录',
      evidence: '证据',
      gatePass: '阶段门已过',
      gateFail: '门禁未过',
      gateOpen: '待拍板',
      gateWaiting: '待产出',
      gateWorking: '编码阶段 · 待 done',
      gateUnverified: '尚未验证',
      gateReady: '可以推进',
      gateNone: '无运行',
      historyRun: '历史',
      assocSession: '本会话',
      assocRepo: '仓库',
      assocMissing: '记录缺失',
      assocNone: '未关联',
      runActive: '进行中',
      runFinished: '已结束',
      runAbandoned: '已放弃',
      runNone: '待命',
      verdicts: '门禁日志',
      noVerdicts: '还没有门禁记录',
      nextStep: '下一步',
      webOn: '展开赛博蛛网',
      webOff: '收起赛博蛛网',
      gatePending: '未开始',
      stageList: '工作流阶段',
      catalog: '已安装工作流',
    };

    const STATUS_KEYS = new Set(['working', 'waiting', 'done', 'abandoned', 'idle']);
    const TONE_GLYPH = {
      pass: '✓',
      fail: '✕',
      open: '?',
      waiting: '…',
      working: '▶',
      unverified: '·',
      ready: '·',
      pending: '·',
      active: '▶',
      none: '·',
    };
    const TONE_LABEL = {
      pass: 'gatePass',
      fail: 'gateFail',
      open: 'gateOpen',
      waiting: 'gateWaiting',
      working: 'gateWorking',
      unverified: 'gateUnverified',
      ready: 'gateReady',
      pending: 'gatePending',
      active: 'gatePass',
      none: 'gateNone',
    };
    const VERDICT_TONE = {
      PASS: 'pass',
      FAIL: 'fail',
      DONE: 'pass',
      READ: 'info',
      DO: 'info',
      WAIT: 'warn',
      OPEN: 'warn',
      NEED: 'warn',
      NEED_ANSWER: 'warn',
      GATE: 'fail',
      RED: 'fail',
      BLOCK: 'fail',
      MISMATCH: 'fail',
      NO_RUN: 'muted',
      ALREADY: 'muted',
      LAST: 'muted',
    };

    const CSS = `
      .dwh-root{
        position:fixed;top:64px;right:18px;z-index:2147482995;width:420px;
        max-width:calc(100vw - 28px);pointer-events:none;font-family:inherit;
        color:var(--dsw-alias-label-primary);
      }
      @media (max-width:760px){
        .dwh-root{top:54px;right:10px;width:calc(100vw - 20px);}
      }
      .dwh-card{
        position:relative;overflow:hidden;width:100%;border-radius:16px;
        border:1px solid var(--dsw-alias-border-l1);
        background:var(--dsw-alias-bg-overlay);
        box-shadow:0 18px 50px rgba(4,16,32,.30),inset 0 0 0 1px rgba(56,225,255,.05);
        pointer-events:none;
      }
      .dwh-card::before{
        content:"";position:absolute;inset:0;z-index:0;pointer-events:none;
        background-image:
          linear-gradient(rgba(86,150,255,.07) 1px,transparent 1px),
          linear-gradient(90deg,rgba(86,150,255,.07) 1px,transparent 1px);
        background-size:22px 22px;
        -webkit-mask-image:linear-gradient(to bottom,#000 0%,rgba(0,0,0,.5) 60%,transparent 94%);
        mask-image:linear-gradient(to bottom,#000 0%,rgba(0,0,0,.5) 60%,transparent 94%);
      }
      .dwh-card::after{
        content:"";position:absolute;left:0;right:0;top:0;height:1px;z-index:2;pointer-events:none;
        background:linear-gradient(90deg,transparent,#38e1ff,#7c5cff,transparent);
        opacity:.85;
      }
      .dwh-card > *{position:relative;z-index:1;}
      .dwh-sweep{
        position:absolute!important;z-index:0!important;top:-140px;right:-130px;
        width:320px;height:320px;border-radius:50%;pointer-events:none;
        background:conic-gradient(from 0deg,transparent 0deg,transparent 298deg,rgba(56,225,255,.16) 338deg,transparent 360deg);
        animation:dwh-spin 7s linear infinite;
      }
      .dwh-head{
        display:flex;align-items:center;gap:7px;padding:10px 12px;
        border-bottom:1px solid var(--dsw-alias-border-l1);
        background:color-mix(in srgb,var(--dsw-alias-bg-layer-2) 52%,transparent);
        pointer-events:none;
      }
      .dwh-logo{display:flex;align-items:center;justify-content:center;width:18px;height:18px;color:#38e1ff;filter:drop-shadow(0 0 5px rgba(56,225,255,.55));}
      .dwh-logo svg{display:block;width:18px;height:18px;}
      .dwh-wordmark{font-size:10px;font-weight:800;letter-spacing:.22em;color:var(--dsw-alias-label-primary);}
      .dwh-wf{
        max-width:92px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;
        padding:2px 6px;border-radius:999px;border:1px solid var(--dsw-alias-border-l1);
        background:var(--dsw-alias-bg-layer-2);color:var(--dsw-alias-label-secondary);
        font-size:9px;font-weight:700;letter-spacing:.04em;
      }
      .dwh-spacer{flex:1;}
      .dwh-link{flex:none;width:6px;height:6px;border-radius:50%;background:var(--dsw-alias-state-idle-primary);}
      .dwh-link.is-on{background:var(--dsw-alias-state-success-primary);box-shadow:0 0 7px var(--dsw-alias-state-success-primary);animation:dwh-breathe 2s ease-in-out infinite;}
      .dwh-link.is-off{background:var(--dsw-alias-state-warn-primary);}
      .dwh-status{display:inline-flex;align-items:center;gap:5px;font-size:9.5px;font-weight:750;letter-spacing:.04em;white-space:nowrap;}
      .dwh-status i{width:6px;height:6px;border-radius:50%;background:currentColor;box-shadow:0 0 7px currentColor;}
      .dwh-status-working{color:var(--dsw-alias-brand-primary);}
      .dwh-status-working i{animation:dwh-breathe 1.4s ease-in-out infinite;}
      .dwh-status-waiting{color:var(--dsw-alias-state-warn-primary);}
      .dwh-status-waiting i{animation:dwh-breathe 1s ease-in-out infinite;}
      .dwh-status-done{color:var(--dsw-alias-state-success-primary);}
      .dwh-status-abandoned,.dwh-status-idle{color:var(--dsw-alias-state-idle-primary);}
      .dwh-icon-btn{
        pointer-events:auto;flex:none;display:inline-flex;align-items:center;justify-content:center;
        width:21px;height:21px;border-radius:7px;border:1px solid var(--dsw-alias-border-l1);
        background:transparent;color:var(--dsw-alias-label-secondary);font:inherit;
        font-size:13px;line-height:1;cursor:pointer;padding:0;
      }
      .dwh-icon-btn:hover{background:var(--dsw-alias-bg-layer-2);color:var(--dsw-alias-label-primary);}
      .dwh-icon-btn:focus-visible{outline:2px solid var(--dsw-alias-brand-primary);outline-offset:1px;}
      .dwh-body{padding:6px 12px 11px;}
      .dwh-feature{display:flex;align-items:center;gap:6px;min-width:0;padding-top:3px;}
      .dwh-feature-name{
        min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;
        font-size:14px;font-weight:750;letter-spacing:-.01em;
      }
      .dwh-assoc{
        flex:none;padding:1px 5px;border-radius:5px;font-size:8.5px;font-weight:750;letter-spacing:.03em;
        border:1px solid var(--dsw-alias-border-l1);color:var(--dsw-alias-label-secondary);
      }
      .dwh-assoc.is-session{color:var(--dsw-alias-brand-primary);border-color:color-mix(in srgb,var(--dsw-alias-brand-primary) 38%,transparent);}
      .dwh-assoc.is-repo{color:var(--dsw-alias-state-warn-primary);border-color:color-mix(in srgb,var(--dsw-alias-state-warn-primary) 38%,transparent);}
      .dwh-assoc.is-missing{color:var(--dsw-alias-state-error-primary);border-color:color-mix(in srgb,var(--dsw-alias-state-error-primary) 38%,transparent);}
      .dwh-repo{
        flex:none;max-width:110px;margin-left:auto;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;
        font-size:9px;color:var(--dsw-alias-label-secondary);
      }
      .dwh-rail{display:block;width:100%;height:auto;margin-top:2px;overflow:visible;}
      .dwh-edge{stroke-width:1.35;fill:none;transition:stroke .3s ease,opacity .3s ease;}
      .dwh-edge-done{stroke:var(--dsw-alias-brand-primary);opacity:.82;}
      .dwh-edge-active{
        stroke:#38e1ff;stroke-width:2.1;stroke-dasharray:7 7;opacity:1;
        filter:drop-shadow(0 0 4px rgba(56,225,255,.6));
        animation:dwh-dash 1.15s linear infinite;
      }
      .dwh-edge-todo{stroke:var(--dsw-alias-border-l2);opacity:.55;stroke-dasharray:2 5;}
      .dwh-particle{fill:#e9feff;filter:drop-shadow(0 0 4px rgba(56,225,255,.95));}
      .dwh-node-disc{stroke-width:1.2;transition:fill .3s ease,stroke .3s ease;fill:var(--dsw-alias-bg-layer-1);stroke:var(--dsw-alias-border-l2);}
      .dwh-node-glyph{font-size:8px;font-weight:800;fill:var(--dsw-alias-label-secondary);}
      .dwh-node-label{font-size:8.2px;fill:var(--dsw-alias-label-secondary);opacity:.8;}
      .dwh-node-label.is-current{fill:var(--dsw-alias-label-primary);opacity:1;font-weight:700;}
      .dwh-node.dwh-tone-pass .dwh-node-disc{fill:var(--dsw-alias-state-success-primary);stroke:var(--dsw-alias-state-success-primary);}
      .dwh-node.dwh-tone-pass .dwh-node-glyph{fill:var(--dsw-alias-bg-base);}
      .dwh-node.dwh-tone-fail .dwh-node-disc{fill:var(--dsw-alias-state-error-primary);stroke:var(--dsw-alias-state-error-primary);}
      .dwh-node.dwh-tone-fail .dwh-node-glyph{fill:var(--dsw-alias-bg-base);}
      .dwh-node.dwh-tone-open .dwh-node-disc{fill:var(--dsw-alias-state-warn-primary);stroke:var(--dsw-alias-state-warn-primary);}
      .dwh-node.dwh-tone-open .dwh-node-glyph{fill:var(--dsw-alias-bg-base);}
      .dwh-node.dwh-tone-waiting .dwh-node-disc{fill:var(--dsw-alias-state-warn-primary);stroke:var(--dsw-alias-state-warn-primary);opacity:.85;}
      .dwh-node.dwh-tone-waiting .dwh-node-glyph{fill:var(--dsw-alias-bg-base);}
      .dwh-node.dwh-tone-working .dwh-node-disc,.dwh-node.dwh-tone-active .dwh-node-disc{fill:#38e1ff;stroke:#d1fbff;stroke-width:1.6;}
      .dwh-node.dwh-tone-working .dwh-node-glyph,.dwh-node.dwh-tone-active .dwh-node-glyph{fill:#06131a;}
      .dwh-node.dwh-tone-unverified .dwh-node-disc{fill:var(--dsw-alias-bg-layer-1);stroke:var(--dsw-alias-state-warn-primary);stroke-dasharray:2 2;}
      .dwh-node.dwh-tone-ready .dwh-node-disc{fill:var(--dsw-alias-bg-layer-1);stroke:var(--dsw-alias-brand-primary);stroke-dasharray:2 2;}
      .dwh-node.dwh-tone-pending .dwh-node-disc,.dwh-node.dwh-tone-none .dwh-node-disc{fill:var(--dsw-alias-bg-layer-1);stroke:var(--dsw-alias-border-l2);opacity:.75;}
      .dwh-node-halo{
        fill:none;stroke:#38e1ff;stroke-width:1.4;opacity:.5;
        transform-box:fill-box;transform-origin:center;
        animation:dwh-halo 1.8s ease-out infinite;
      }
      .dwh-gate{
        margin-top:5px;padding:8px 9px;border-radius:10px;
        border:1px solid var(--dsw-alias-border-l1);
        background:color-mix(in srgb,var(--dsw-alias-bg-layer-2) 42%,transparent);
      }
      .dwh-gate.dwh-tone-pass{border-color:color-mix(in srgb,var(--dsw-alias-state-success-primary) 34%,transparent);}
      .dwh-gate.dwh-tone-fail{border-color:color-mix(in srgb,var(--dsw-alias-state-error-primary) 42%,transparent);background:color-mix(in srgb,var(--dsw-alias-state-error-primary) 7%,transparent);}
      .dwh-gate.dwh-tone-open,.dwh-gate.dwh-tone-waiting{border-color:color-mix(in srgb,var(--dsw-alias-state-warn-primary) 38%,transparent);background:color-mix(in srgb,var(--dsw-alias-state-warn-primary) 7%,transparent);}
      .dwh-gate.dwh-tone-working,.dwh-gate.dwh-tone-active{border-color:color-mix(in srgb,#38e1ff 36%,transparent);}
      .dwh-gate-head{display:flex;align-items:center;gap:6px;min-width:0;}
      .dwh-gate-dot{flex:none;width:7px;height:7px;border-radius:50%;background:var(--dsw-alias-label-secondary);}
      .dwh-tone-pass .dwh-gate-dot{background:var(--dsw-alias-state-success-primary);box-shadow:0 0 7px var(--dsw-alias-state-success-primary);}
      .dwh-tone-fail .dwh-gate-dot{background:var(--dsw-alias-state-error-primary);box-shadow:0 0 7px var(--dsw-alias-state-error-primary);}
      .dwh-tone-open .dwh-gate-dot,.dwh-tone-waiting .dwh-gate-dot{background:var(--dsw-alias-state-warn-primary);box-shadow:0 0 7px var(--dsw-alias-state-warn-primary);animation:dwh-breathe 1.2s ease-in-out infinite;}
      .dwh-tone-working .dwh-gate-dot,.dwh-tone-active .dwh-gate-dot{background:#38e1ff;box-shadow:0 0 7px rgba(56,225,255,.8);animation:dwh-breathe 1.4s ease-in-out infinite;}
      .dwh-gate-label{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:11.5px;font-weight:780;}
      .dwh-gate-scope{flex:none;margin-left:auto;font-size:8.5px;color:var(--dsw-alias-label-secondary);font-variant-numeric:tabular-nums;}
      .dwh-gate-reason{
        margin-top:3px;font-size:9.5px;line-height:1.45;color:var(--dsw-alias-label-secondary);
        display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;
      }
      .dwh-gate-next{
        display:flex;align-items:baseline;gap:5px;margin-top:5px;padding-top:5px;
        border-top:1px dashed var(--dsw-alias-border-l1);
        font-size:9.5px;line-height:1.45;color:var(--dsw-alias-label-secondary);
      }
      .dwh-gate-next b{flex:none;font-size:8.5px;font-weight:800;letter-spacing:.06em;text-transform:uppercase;color:var(--dsw-alias-label-primary);}
      .dwh-gate-next span{min-width:0;}
      .dwh-facts{display:flex;flex-wrap:wrap;gap:4px;margin-top:6px;}
      .dwh-fact{
        padding:1.5px 5px;border-radius:5px;font-size:8.5px;font-weight:650;
        border:1px solid var(--dsw-alias-border-l1);color:var(--dsw-alias-label-secondary);
        background:var(--dsw-alias-bg-layer-1);font-variant-numeric:tabular-nums;
      }
      .dwh-fact.is-pass{color:var(--dsw-alias-state-success-primary);border-color:color-mix(in srgb,var(--dsw-alias-state-success-primary) 32%,transparent);}
      .dwh-fact.is-fail{color:var(--dsw-alias-state-error-primary);border-color:color-mix(in srgb,var(--dsw-alias-state-error-primary) 34%,transparent);}
      .dwh-fact.is-warn{color:var(--dsw-alias-state-warn-primary);border-color:color-mix(in srgb,var(--dsw-alias-state-warn-primary) 34%,transparent);}
      .dwh-chips{display:flex;flex-wrap:wrap;gap:4px;margin-top:5px;}
      .dwh-chip{
        display:inline-flex;align-items:center;gap:3px;max-width:100%;
        padding:1.5px 5px;border-radius:5px;font-size:8.5px;
        border:1px solid var(--dsw-alias-border-l1);color:var(--dsw-alias-label-secondary);
        white-space:nowrap;overflow:hidden;text-overflow:ellipsis;
      }
      .dwh-chip.is-ok{color:var(--dsw-alias-state-success-primary);border-color:color-mix(in srgb,var(--dsw-alias-state-success-primary) 28%,transparent);}
      .dwh-chip.is-bad{color:var(--dsw-alias-state-error-primary);border-color:color-mix(in srgb,var(--dsw-alias-state-error-primary) 32%,transparent);}
      .dwh-evidence{
        margin-top:5px;padding-top:4px;border-top:1px dashed var(--dsw-alias-border-l1);
        font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
        font-size:8px;color:var(--dsw-alias-label-secondary);white-space:nowrap;overflow:hidden;text-overflow:ellipsis;
      }
      .dwh-protect{
        display:flex;align-items:center;gap:5px;margin-top:4px;padding-top:4px;
        border-top:1px dashed var(--dsw-alias-border-l1);font-size:8.5px;color:var(--dsw-alias-label-secondary);
      }
      .dwh-protect-key{flex:none;font-weight:750;}
      .dwh-protect-value{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;}
      .dwh-stages{
        display:flex;flex-direction:column;gap:4px;margin-top:6px;
        max-height:230px;overflow-y:auto;pointer-events:auto;padding-right:2px;
        scrollbar-width:thin;
      }
      .dwh-stages::-webkit-scrollbar{width:4px;}
      .dwh-stages::-webkit-scrollbar-thumb{background:var(--dsw-alias-border-l2);border-radius:999px;}
      .dwh-stage{
        border:1px solid var(--dsw-alias-border-l1);border-radius:9px;
        background:color-mix(in srgb,var(--dsw-alias-bg-layer-2) 30%,transparent);
        padding:4px 6px;
      }
      .dwh-stage.is-current{border-color:color-mix(in srgb,#38e1ff 30%,transparent);background:color-mix(in srgb,#38e1ff 6%,transparent);}
      .dwh-stage.dwh-tone-fail{border-color:color-mix(in srgb,var(--dsw-alias-state-error-primary) 34%,transparent);}
      .dwh-stage.dwh-tone-open,.dwh-stage.dwh-tone-waiting{border-color:color-mix(in srgb,var(--dsw-alias-state-warn-primary) 32%,transparent);}
      .dwh-stage-head{display:flex;align-items:center;gap:5px;min-width:0;}
      .dwh-stage-dot{flex:none;width:7px;height:7px;border-radius:50%;background:var(--dsw-alias-label-secondary);}
      .dwh-stage.dwh-tone-pass .dwh-stage-dot{background:var(--dsw-alias-state-success-primary);box-shadow:0 0 6px var(--dsw-alias-state-success-primary);}
      .dwh-stage.dwh-tone-active .dwh-stage-dot{background:#38e1ff;box-shadow:0 0 7px rgba(56,225,255,.8);animation:dwh-breathe 1.4s ease-in-out infinite;}
      .dwh-stage.dwh-tone-fail .dwh-stage-dot{background:var(--dsw-alias-state-error-primary);box-shadow:0 0 6px var(--dsw-alias-state-error-primary);}
      .dwh-stage.dwh-tone-open .dwh-stage-dot,.dwh-stage.dwh-tone-waiting .dwh-stage-dot{background:var(--dsw-alias-state-warn-primary);box-shadow:0 0 6px var(--dsw-alias-state-warn-primary);}
      .dwh-stage.dwh-tone-working .dwh-stage-dot{background:#38e1ff;box-shadow:0 0 7px rgba(56,225,255,.8);animation:dwh-breathe 1.4s ease-in-out infinite;}
      .dwh-stage.dwh-tone-unverified .dwh-stage-dot{background:var(--dsw-alias-bg-layer-1);border:1px solid var(--dsw-alias-state-warn-primary);}
      .dwh-stage.dwh-tone-ready .dwh-stage-dot{background:var(--dsw-alias-bg-layer-1);border:1px solid var(--dsw-alias-brand-primary);}
      .dwh-stage.dwh-tone-pending .dwh-stage-dot,.dwh-stage.dwh-tone-none .dwh-stage-dot{background:var(--dsw-alias-border-l2);box-shadow:none;}
      .dwh-stage-num{flex:none;font-size:8px;font-weight:800;color:var(--dsw-alias-label-secondary);font-variant-numeric:tabular-nums;}
      .dwh-stage-name{flex:1;min-width:0;font-size:9.6px;font-weight:700;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;color:var(--dsw-alias-label-primary);}
      .dwh-stage-counts{flex:none;font-size:8px;color:var(--dsw-alias-label-secondary);font-variant-numeric:tabular-nums;}
      .dwh-stage-ev{flex:none;font-size:7.5px;font-weight:800;letter-spacing:.04em;padding:0 3px;border-radius:3px;border:1px solid currentColor;}
      .dwh-stage-ev.is-ok{color:var(--dsw-alias-state-success-primary);}
      .dwh-stage-ev.is-bad{color:var(--dsw-alias-state-error-primary);}
      .dwh-stage-ev.is-none{color:var(--dsw-alias-label-secondary);opacity:.7;}
      .dwh-stage-label{flex:none;font-size:8.2px;font-weight:700;color:var(--dsw-alias-label-secondary);}
      .dwh-stage.dwh-tone-fail .dwh-stage-label{color:var(--dsw-alias-state-error-primary);}
      .dwh-stage.dwh-tone-open .dwh-stage-label,.dwh-stage.dwh-tone-waiting .dwh-stage-label{color:var(--dsw-alias-state-warn-primary);}
      .dwh-stage.dwh-tone-pass .dwh-stage-label{color:var(--dsw-alias-state-success-primary);}
      .dwh-stage.dwh-tone-active .dwh-stage-label,.dwh-stage.dwh-tone-working .dwh-stage-label{color:#38e1ff;}
      .dwh-stage-detail{margin-top:4px;padding-top:4px;border-top:1px dashed var(--dsw-alias-border-l1);}
      .dwh-stage-reason{font-size:8.8px;line-height:1.45;color:var(--dsw-alias-label-secondary);display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;}
      .dwh-catalog{display:flex;flex-direction:column;gap:6px;margin-top:6px;}
      .dwh-workflow{
        border:1px solid var(--dsw-alias-border-l1);border-radius:9px;
        background:color-mix(in srgb,var(--dsw-alias-bg-layer-2) 30%,transparent);
        padding:5px 7px;
      }
      .dwh-workflow-head{display:flex;align-items:baseline;justify-content:space-between;gap:8px;}
      .dwh-workflow-name{font-size:9.5px;font-weight:800;letter-spacing:.05em;color:var(--dsw-alias-label-primary);}
      .dwh-workflow-count{flex:none;font-size:8px;color:var(--dsw-alias-label-secondary);font-variant-numeric:tabular-nums;}
      .dwh-workflow-chain{display:flex;flex-wrap:wrap;gap:3px;margin-top:4px;}
      .dwh-workflow-stage{
        display:inline-flex;align-items:center;gap:3px;min-width:0;
        padding:1px 5px;border-radius:5px;font-size:8.5px;
        border:1px solid var(--dsw-alias-border-l1);color:var(--dsw-alias-label-secondary);
        white-space:nowrap;overflow:hidden;text-overflow:ellipsis;
      }
      .dwh-workflow-num{flex:none;font-size:7.5px;font-weight:800;opacity:.7;}
      .dwh-workflow-stage-name{overflow:hidden;text-overflow:ellipsis;}
      .dwh-question{
        margin:0 0 7px;padding:6px 8px;border-left:2px solid var(--dsw-alias-state-warn-primary);
        border-radius:0 8px 8px 0;background:color-mix(in srgb,var(--dsw-alias-state-warn-primary) 10%,transparent);
        font-size:10px;line-height:1.45;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;
      }
      .dwh-question b{color:var(--dsw-alias-state-warn-primary);font-weight:750;margin-right:4px;}
      .dwh-caption{display:flex;align-items:baseline;justify-content:space-between;gap:8px;margin-top:2px;}
      .dwh-caption-label{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12px;font-weight:700;}
      .dwh-caption-meta{flex:none;font-size:9.5px;color:var(--dsw-alias-label-secondary);font-variant-numeric:tabular-nums;}
      .dwh-progress{height:3px;border-radius:999px;background:var(--dsw-alias-border-l1);overflow:hidden;margin:6px 0 7px;}
      .dwh-progress i{display:block;height:100%;border-radius:999px;background:linear-gradient(90deg,#38e1ff,#7c5cff);box-shadow:0 0 8px rgba(56,225,255,.45);transition:width .4s ease;}
      .dwh-section{border-top:1px solid var(--dsw-alias-border-l1);padding-top:7px;margin-top:7px;}
      .dwh-section-head{display:flex;justify-content:space-between;align-items:center;margin-bottom:5px;font-size:8.5px;font-weight:750;letter-spacing:.14em;text-transform:uppercase;color:var(--dsw-alias-label-secondary);}
      .dwh-section-count{font-variant-numeric:tabular-nums;letter-spacing:0;opacity:.8;}
      .dwh-verdicts{display:flex;flex-direction:column;gap:4px;max-height:180px;overflow-y:auto;pointer-events:auto;scrollbar-width:thin;}
      .dwh-verdict{
        display:grid;grid-template-columns:auto minmax(0,1fr);align-items:center;gap:6px;
        padding:3px 5px;border-radius:7px;background:color-mix(in srgb,var(--dsw-alias-bg-layer-2) 30%,transparent);
      }
      .dwh-v-kind{
        padding:1px 5px;border-radius:4px;font-size:8px;font-weight:800;letter-spacing:.05em;
        border:1px solid currentColor;
      }
      .dwh-v-kind.is-pass{color:var(--dsw-alias-state-success-primary);}
      .dwh-v-kind.is-fail{color:var(--dsw-alias-state-error-primary);}
      .dwh-v-kind.is-warn{color:var(--dsw-alias-state-warn-primary);}
      .dwh-v-kind.is-info{color:#38e1ff;}
      .dwh-v-kind.is-muted{color:var(--dsw-alias-label-secondary);}
      .dwh-v-detail{min-width:0;font-size:8.8px;color:var(--dsw-alias-label-secondary);white-space:nowrap;overflow:hidden;text-overflow:ellipsis;}
      .dwh-empty{font-size:9px;color:var(--dsw-alias-label-secondary);padding:2px 0 0;line-height:1.5;}
      .dwh-pill{
        pointer-events:auto;display:flex;align-items:center;gap:7px;margin-left:auto;width:fit-content;max-width:100%;
        padding:7px 10px;border-radius:999px;border:1px solid var(--dsw-alias-border-l1);
        background:var(--dsw-alias-bg-overlay);box-shadow:0 10px 28px rgba(4,16,32,.22);
        color:var(--dsw-alias-label-primary);font:inherit;font-size:10.5px;cursor:pointer;
      }
      .dwh-pill.is-static{pointer-events:none;cursor:default;opacity:.9;}
      .dwh-pill:hover{border-color:color-mix(in srgb,var(--dsw-alias-brand-primary) 42%,var(--dsw-alias-border-l1));}
      .dwh-pill:focus-visible{outline:2px solid var(--dsw-alias-brand-primary);outline-offset:2px;}
      .dwh-pill-name{min-width:0;max-width:250px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-weight:700;}
      .dwh-pill-status{flex:none;font-size:9px;color:var(--dsw-alias-label-secondary);}
      .dwh-crawler{
        position:fixed;inset:0;width:100vw;height:100vh;z-index:2;
        pointer-events:none;display:block;
      }
      .dwh-card{z-index:1;}
      .dwh-icon-btn.is-on{
        color:#38e1ff;border-color:color-mix(in srgb,#38e1ff 45%,transparent);
        background:color-mix(in srgb,#38e1ff 12%,transparent);
        box-shadow:0 0 8px rgba(56,225,255,.35);
      }
      @keyframes dwh-spin{to{transform:rotate(360deg);}}
      @keyframes dwh-dash{to{stroke-dashoffset:-28;}}
      @keyframes dwh-breathe{0%,100%{opacity:1;}50%{opacity:.38;}}
      @keyframes dwh-halo{0%{opacity:.62;transform:scale(.62);}70%{opacity:0;transform:scale(1.55);}100%{opacity:0;transform:scale(1.55);}}
      @media (prefers-reduced-motion:reduce){
        .dwh-sweep,.dwh-status i,.dwh-link.is-on,.dwh-node-halo,.dwh-tone-open .dwh-gate-dot,.dwh-tone-waiting .dwh-gate-dot,.dwh-tone-working .dwh-gate-dot,.dwh-tone-active .dwh-gate-dot{animation:none!important;}
        .dwh-edge-active{animation:none!important;stroke-dasharray:none;}
        .dwh-progress i{transition:none;}
      }
    `;

    function shortStage(name, limit) {
      const text = String(name || '').replace(/[（(].*?[）)]/g, '').trim() || String(name || '');
      const max = limit || 6;
      return text.length > max ? `${text.slice(0, max)}…` : text;
    }

    function shortFile(value) {
      const parts = String(value || '').split(/[\\/]+/).filter(Boolean);
      if (parts.length <= 2) return parts.join('/');
      return parts.slice(-2).join('/');
    }

    function useReducedMotion() {
      const [reduced, setReduced] = useState(
        () =>
          typeof window !== 'undefined' &&
          typeof window.matchMedia === 'function' &&
          window.matchMedia('(prefers-reduced-motion: reduce)').matches,
      );
      useEffect(() => {
        if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return undefined;
        const query = window.matchMedia('(prefers-reduced-motion: reduce)');
        const update = () => setReduced(query.matches);
        update();
        if (typeof query.addEventListener === 'function') {
          query.addEventListener('change', update);
          return () => query.removeEventListener('change', update);
        }
        if (typeof query.addListener === 'function') {
          query.addListener(update);
          return () => query.removeListener(update);
        }
        return undefined;
      }, []);
      return reduced;
    }

    function toneLabel(tone, t) {
      const key = TONE_LABEL[tone] || 'gateUnverified';
      return t(key);
    }

    function StageRail({ stages, gates, current, finished, reduced, t }) {
      const total = stages.length;
      if (!total) return null;
      const width = 372;
      const height = 126;
      const left = 24;
      const right = 24;
      const baseY = 46;
      const amp = 14;
      const toneById = new Map(
        (Array.isArray(gates) ? gates : []).map((item) => [Number(item.id), item.tone]),
      );
      const points = stages.map((stage, index) => {
        const x = total === 1 ? width / 2 : left + (index * (width - left - right)) / (total - 1);
        const y = baseY + Math.sin(index * 1.15) * amp;
        return { x, y, stage, index };
      });

      const edges = [];
      for (let index = 0; index < total - 1; index += 1) {
        const from = points[index];
        const to = points[index + 1];
        const middle = (from.x + to.x) / 2;
        const d = `M ${from.x} ${from.y} C ${middle} ${from.y}, ${middle} ${to.y}, ${to.x} ${to.y}`;
        let state = 'todo';
        if (finished) state = 'done';
        else if (index < current - 1) state = 'done';
        else if (index === current - 1) state = 'active';
        edges.push({ d, state, key: index });
      }

      return h(
        'svg',
        {
          className: 'dwh-rail',
          viewBox: `0 0 ${width} ${height}`,
          'aria-hidden': true,
          focusable: 'false',
        },
        edges.map((edge) =>
          h(
            'g',
            { key: `edge-${edge.key}` },
            h('path', { className: `dwh-edge dwh-edge-${edge.state}`, d: edge.d }),
            edge.state === 'active' && !reduced
              ? h(
                  'circle',
                  { key: 'particle', className: 'dwh-particle', r: 2.4 },
                  h('animateMotion', {
                    dur: '1.8s',
                    repeatCount: 'indefinite',
                    path: edge.d,
                  }),
                )
              : null,
          ),
        ),
        points.map((point) => {
          const gateTone = toneById.get(Number(point.stage.id));
          let tone = gateTone || 'pending';
          if (!finished && point.index === current) {
            if (tone === 'pass') tone = 'active';
            else if (tone === 'pending') tone = 'working';
          }
          const radius = tone === 'active' || tone === 'working' ? 9.5 : 7;
          return h(
            'g',
            {
              key: `node-${point.index}`,
              className: `dwh-node dwh-tone-${tone}`,
            },
            h('title', null, `${point.index + 1}. ${point.stage.name} · ${toneLabel(tone, t)}`),
            (tone === 'active' || tone === 'working') && !reduced
              ? h('circle', {
                  cx: point.x,
                  cy: point.y,
                  r: 16,
                  className: 'dwh-node-halo',
                })
              : null,
            h('circle', {
              cx: point.x,
              cy: point.y,
              r: radius,
              className: 'dwh-node-disc',
            }),
            h(
              'text',
              {
                x: point.x,
                y: point.y + 3,
                textAnchor: 'middle',
                className: 'dwh-node-glyph',
              },
              TONE_GLYPH[tone] || '·',
            ),
            h(
              'text',
              {
                x: point.x,
                y: height - 8,
                textAnchor: 'middle',
                className: `dwh-node-label${point.index === current ? ' is-current' : ''}`,
              },
              shortStage(point.stage.name),
            ),
          );
        }),
      );
    }

    function StageList({ gates, currentId, t }) {
      const list = Array.isArray(gates) ? gates : [];
      if (!list.length) return null;
      return h(
        'div',
        { className: 'dwh-stages', role: 'list', 'aria-label': t('stageList') },
        list.map((gate, index) => {
          const requires = Array.isArray(gate.requires) ? gate.requires : [];
          const artifacts = Array.isArray(gate.artifacts) ? gate.artifacts : [];
          const missingArtifacts = Array.isArray(gate.missingArtifacts)
            ? gate.missingArtifacts
            : [];
          const isCurrent = Number(gate.id) === Number(currentId);
          const tone = gate.tone || 'pending';
          const isFuture = tone === 'pending' || tone === 'none';
          const foundReq = requires.filter((item) => item.found).length;
          const foundArt = artifacts.filter((item) => item.exists).length;
          // A future stage has not produced anything yet; printing 0/2 would
          // read as a failure. Its totals are still visible in the title.
          const counts = isFuture
            ? ''
            : [
                requires.length ? `${foundReq}/${requires.length}` : '',
                artifacts.length ? `${foundArt}/${artifacts.length}` : '',
              ]
                .filter(Boolean)
                .join(' · ');
          const countsTitle = [
            requires.length ? `${t('requires')} ${requires.length}` : '',
            artifacts.length ? `${t('artifacts')} ${artifacts.length}` : '',
          ]
            .filter(Boolean)
            .join(' · ');
          const ev = gate.evidence;
          const evBad = Boolean(
            ev &&
              ((ev.verify && !['none', 'exit0'].includes(ev.verify)) ||
                (ev.gateCheck && !['none', 'ok'].includes(ev.gateCheck))),
          );
          // Completed and in-progress stages carry their full gate detail.
          // Future stages stay compact until they become reachable.
          const showDetail = !isFuture;
          return h(
            'div',
            {
              key: gate.id || index,
              className: `dwh-stage dwh-tone-${tone}${isCurrent ? ' is-current' : ''}`,
              role: 'listitem',
              'data-dwh-anchor': `stage-${gate.id || index + 1}`,
              'data-dwh-tone': tone,
            },
            h(
              'div',
              { className: 'dwh-stage-head' },
              h('span', { className: 'dwh-stage-dot', 'aria-hidden': true }),
              h('span', { className: 'dwh-stage-num' }, String(index + 1)),
              h(
                'span',
                { className: 'dwh-stage-name', title: gate.name || '' },
                gate.name || `阶段 ${index + 1}`,
              ),
              counts
                ? h(
                    'span',
                    { className: 'dwh-stage-counts', title: countsTitle },
                    counts,
                  )
                : null,
              ev
                ? h(
                    'span',
                    {
                      className: `dwh-stage-ev ${evBad ? 'is-bad' : 'is-ok'}`,
                      title: ev.line || '',
                    },
                    'EV',
                  )
                : null,
              h(
                'span',
                { className: 'dwh-stage-label' },
                gate.label || toneLabel(tone, t),
              ),
            ),
            showDetail
              ? h(
                  'div',
                  { className: 'dwh-stage-detail' },
                  gate.reason
                    ? h('div', { className: 'dwh-stage-reason' }, gate.reason)
                    : null,
                  requires.length
                    ? h(
                        'div',
                        { className: 'dwh-chips' },
                        requires.map((item, i) =>
                          h(
                            'span',
                            {
                              key: `req-${i}`,
                              className: `dwh-chip ${item.found ? 'is-ok' : 'is-bad'}`,
                            },
                            `${item.found ? '✓' : '✕'} ${item.label}`,
                          ),
                        ),
                      )
                    : null,
                  missingArtifacts.length
                    ? h(
                        'div',
                        { className: 'dwh-chips' },
                        missingArtifacts.map((item) =>
                          h(
                            'span',
                            {
                              key: item,
                              className: 'dwh-chip is-bad',
                              title: item,
                            },
                            `✕ ${shortFile(item)}`,
                          ),
                        ),
                      )
                    : null,
                  ev && ev.line
                    ? h('div', { className: 'dwh-evidence', title: ev.line }, ev.line)
                    : null,
                )
              : null,
          );
        }),
      );
    }

    function SummaryBar({ gate, t }) {
      if (!gate) return null;
      const verify = gate.verify || {};
      const prot = gate.protection || {};
      const protectedPaths = Array.isArray(prot.protectedPaths) ? prot.protectedPaths : [];
      const waiverGlobs = Array.isArray(prot.waiverGlobs) ? prot.waiverGlobs : [];
      const passes = Array.isArray(prot.passes) ? prot.passes : [];
      const title = [
        verify.cmd ? `verify: ${verify.cmd}` : '',
        protectedPaths.length ? `protected: ${protectedPaths.join(', ')}` : '',
        waiverGlobs.length ? `waiver: ${waiverGlobs.join(', ')}` : '',
        ...passes,
      ]
        .filter(Boolean)
        .join('\n');
      const hasExit = verify.exit !== null && verify.exit !== undefined;
      return h(
        'div',
        { className: 'dwh-protect', title: title || undefined },
        h('span', { className: 'dwh-protect-key' }, t('writeGate')),
        h(
          'span',
          { className: 'dwh-protect-value' },
          prot.configured
            ? `${t('protectedPaths')} ${protectedPaths.length} · ${t('passes')} ${
                prot.passesCount || 0
              }`
            : t('notProtected'),
        ),
        hasExit
          ? h(
              'span',
              {
                className: `dwh-fact is-${verify.exit === 0 ? 'pass' : 'fail'}`,
                style: { marginLeft: 'auto', flex: 'none' },
              },
              `verify ${verify.exit}`,
            )
          : null,
      );
    }

    function GateBanner({ gate, t }) {
      if (!gate) return null;
      return h(
        'div',
        {
          className: `dwh-gate dwh-tone-${gate.state || 'none'}`,
          'data-dwh-anchor': 'gate',
          'data-dwh-tone': gate.state || 'none',
        },
        h(
          'div',
          { className: 'dwh-gate-head' },
          h('span', { className: 'dwh-gate-dot', 'aria-hidden': true }),
          h('span', { className: 'dwh-gate-label' }, gate.label || toneLabel(gate.state, t)),
          gate.stageId
            ? h(
                'span',
                { className: 'dwh-gate-scope' },
                `STAGE ${gate.stageId}${gate.stageName ? ` · ${gate.stageName}` : ''}`,
              )
            : null,
        ),
        gate.reason ? h('div', { className: 'dwh-gate-reason' }, gate.reason) : null,
        gate.next
          ? h(
              'div',
              { className: 'dwh-gate-next' },
              h('b', null, t('nextStep')),
              h('span', null, gate.next),
            )
          : null,
      );
    }

    function Catalog({ catalog, t }) {
      const list = Array.isArray(catalog) ? catalog : [];
      if (!list.length) return null;
      return h(
        'div',
        { className: 'dwh-section' },
        h(
          'div',
          { className: 'dwh-section-head' },
          h('span', null, t('catalog')),
          h('span', { className: 'dwh-section-count' }, String(list.length)),
        ),
        h(
          'div',
          { className: 'dwh-catalog' },
          list.map((workflow) => {
            const stages = Array.isArray(workflow.stages) ? workflow.stages : [];
            return h(
              'div',
              {
                className: 'dwh-workflow',
                key: workflow.id,
                'data-dwh-anchor': `workflow-${workflow.id}`,
                'data-dwh-tone': 'pending',
              },
              h(
                'div',
                { className: 'dwh-workflow-head' },
                h('span', { className: 'dwh-workflow-name' }, workflow.id),
                h(
                  'span',
                  { className: 'dwh-workflow-count' },
                  `${stages.length} ${t('step')}`,
                ),
              ),
              h(
                'div',
                { className: 'dwh-workflow-chain' },
                stages.map((stage, index) =>
                  h(
                    'span',
                    {
                      className: 'dwh-workflow-stage',
                      key: stage.id || index,
                      title: [
                        stage.name || '',
                        stage.code ? 'code' : '',
                        stage.gateCheck ? 'gate_check' : '',
                      ]
                        .filter(Boolean)
                        .join(' · '),
                    },
                    h('span', { className: 'dwh-workflow-num' }, String(index + 1)),
                    h(
                      'span',
                      { className: 'dwh-workflow-stage-name' },
                      stage.name || `阶段 ${index + 1}`,
                    ),
                    stage.code ? h('span', { className: 'dwh-workflow-num' }, 'C') : null,
                    stage.gateCheck
                      ? h('span', { className: 'dwh-workflow-num' }, 'G')
                      : null,
                  ),
                ),
              ),
            );
          }),
        ),
      );
    }

    function Verdicts({ items, t }) {
      if (!items.length) {
        return h('div', { className: 'dwh-empty' }, t('noVerdicts'));
      }
      return h(
        'div',
        { className: 'dwh-verdicts' },
        items.slice(-8).map((item, index) =>
          h(
            'div',
            {
              className: 'dwh-verdict',
              key: `${item.kind}-${item.stageId || ''}-${item.at || index}`,
              'data-dwh-anchor': `log-${index}`,
              'data-dwh-tone': crawlerVerdictTone(item.kind),
            },
            h(
              'span',
              {
                className: `dwh-v-kind is-${VERDICT_TONE[item.kind] || 'muted'}`,
              },
              item.kind,
            ),
            h(
              'span',
              { className: 'dwh-v-detail', title: item.detail || '' },
              item.detail || '—',
            ),
          ),
        ),
      );
    }

    /* ── cyber web-crawler layer ──────────────────────────────────────────
       A full-viewport canvas that crawls only the plugin's own rendered
       anchors: stage rows, the gate banner, and the gate log. It never reads
       the host page or another plugin's DOM, and it is pointer-transparent.
    ─────────────────────────────────────────────────────────────────────── */
    const CRAWL_COLORS = {
      pass: '#46e6a0',
      fail: '#ff4d6d',
      open: '#ffb347',
      waiting: '#ffb347',
      working: '#38e1ff',
      active: '#38e1ff',
      ready: '#7c5cff',
      unverified: '#8fa3bf',
      pending: '#42566f',
      none: '#42566f',
    };

    function crawlerToneColor(tone) {
      return CRAWL_COLORS[tone] || CRAWL_COLORS.pending;
    }

    const CRAWL_VERDICT = {
      pass: 'pass',
      fail: 'fail',
      warn: 'open',
      info: 'working',
      muted: 'pending',
    };

    /** Map a verdict chip tone onto a crawler tone. */
    function crawlerVerdictTone(kind) {
      return CRAWL_VERDICT[VERDICT_TONE[kind]] || 'pending';
    }

    function crawlerBezier(p0, p1, p2, p3, t) {
      const u = 1 - t;
      const a = u * u * u;
      const b = 3 * u * u * t;
      const c = 3 * u * t * t;
      const d = t * t * t;
      return {
        x: a * p0.x + b * p1.x + c * p2.x + d * p3.x,
        y: a * p0.y + b * p1.y + c * p2.y + d * p3.y,
      };
    }

    /** Two-bone IK: returns the knee and the (clamped) foot. */
    function crawlerIK(hip, foot, lenA, lenB, bend) {
      const dx = foot.x - hip.x;
      const dy = foot.y - hip.y;
      const dist = Math.hypot(dx, dy) || 0.001;
      const min = Math.abs(lenA - lenB) + 0.01;
      const max = lenA + lenB - 0.01;
      const reach = Math.min(Math.max(dist, min), max);
      const ux = dx / dist;
      const uy = dy / dist;
      const a = (lenA * lenA - lenB * lenB + reach * reach) / (2 * reach);
      const h = Math.sqrt(Math.max(0, lenA * lenA - a * a));
      const sign = bend >= 0 ? 1 : -1;
      return {
        knee: {
          x: hip.x + ux * a - uy * h * sign,
          y: hip.y + uy * a + ux * h * sign,
        },
        foot: { x: hip.x + ux * reach, y: hip.y + uy * reach },
        reachable: dist <= lenA + lenB,
        dist,
      };
    }

    /** Perch points the spider tours, ordered by anchor position. */
    function crawlerTour(anchors, width, height) {
      const list = (Array.isArray(anchors) ? anchors : []).slice().sort((a, b) => a.y - b.y);
      return list.map((node, index) => {
        const side = index % 2 === 0 ? -1 : 1;
        return {
          x: Math.min(width - 36, Math.max(36, node.x - 150 + side * 54)),
          y: Math.min(height - 40, Math.max(56, node.y - 34 + (index % 3) * 22)),
          anchor: node,
        };
      });
    }

    /** Read the plugin's own anchors. Falls back to the card edges. */
    function crawlerReadAnchors(container) {
      if (!container || typeof container.querySelectorAll !== 'function') return [];
      const nodes = container.querySelectorAll('[data-dwh-anchor]');
      const out = [];
      for (const node of nodes) {
        if (!node || typeof node.getBoundingClientRect !== 'function') continue;
        const rect = node.getBoundingClientRect();
        if (rect.width <= 0 && rect.height <= 0) continue;
        out.push({
          id: node.getAttribute('data-dwh-anchor') || 'anchor',
          tone: node.getAttribute('data-dwh-tone') || 'pending',
          x: Math.max(18, rect.left - 10),
          y: rect.top + rect.height / 2,
          w: rect.width,
          h: rect.height,
        });
      }
      if (!out.length) {
        const rect = container.getBoundingClientRect();
        if (rect.width > 0) {
          out.push({ id: 'gate', tone: 'none', x: Math.max(18, rect.left - 10), y: rect.top + 80, w: rect.width, h: 1 });
          out.push({ id: 'card', tone: 'none', x: Math.max(18, rect.left - 10), y: rect.bottom - 80, w: rect.width, h: 1 });
        }
      }
      return out;
    }

    function Crawler({ state, verdicts, enabled, reduced }) {
      const canvasRef = useRef(null);
      const dataRef = useRef({ state, verdicts });

      useEffect(() => {
        dataRef.current = { state, verdicts };
      }, [state, verdicts]);

      useEffect(() => {
        if (!enabled) return undefined;
        const canvas = canvasRef.current;
        if (!canvas || typeof canvas.getContext !== 'function') return undefined;
        const ctx = canvas.getContext('2d');
        if (!ctx) return undefined;

        let alive = true;
        let raf = 0;
        let last = 0;
        const model = {
          body: { x: 0, y: 0 },
          angle: 0,
          target: null,
          dwellUntil: 0,
          tour: [],
          tourIndex: 0,
          legs: [],
          bursts: [],
          seen: new Set(),
          seeded: false,
          lastGateTone: '',
        };
        const view = { w: 0, h: 0, dpr: 1, anchors: [], lastRead: 0, signature: '' };

        const resize = () => {
          const dpr = Math.min(2, Math.max(1, window.devicePixelRatio || 1));
          view.w = window.innerWidth;
          view.h = window.innerHeight;
          view.dpr = dpr;
          canvas.width = Math.round(view.w * dpr);
          canvas.height = Math.round(view.h * dpr);
          canvas.style.width = `${view.w}px`;
          canvas.style.height = `${view.h}px`;
        };

        const anchorsNow = () => {
          const now = Date.now();
          if (now - view.lastRead < 220 && view.anchors.length) return view.anchors;
          view.lastRead = now;
          const anchors = crawlerReadAnchors(canvas.parentElement);
          const signature = anchors
            .map((a) => `${a.id}:${a.tone}:${Math.round(a.x)}:${Math.round(a.y)}`)
            .join('|');
          view.anchors = anchors;
          if (signature !== view.signature) {
            view.signature = signature;
            model.tour = crawlerTour(anchors, view.w, view.h);
            if (!model.target || !model.tour.includes(model.target)) {
              model.tourIndex = 0;
              model.target = model.tour[0] || null;
            }
          }
          return anchors;
        };

        const spawnBurst = (anchor, color, power, delay) => {
          if (!anchor) return;
          const now = Date.now();
          model.bursts.push({
            x: anchor.x,
            y: anchor.y,
            color,
            power: power || 1,
            t0: now,
            startAt: now + (delay || 0),
            life: 900 + (power || 0) * 500,
          });
          if (model.bursts.length > 28) model.bursts.splice(0, model.bursts.length - 28);
        };

        const sync = () => {
          const data = dataRef.current || {};
          const gate = data.state && data.state.gate ? data.state.gate : null;
          const tone = gate ? gate.state || 'none' : 'none';
          const gateAnchor =
            view.anchors.find((a) => a.id === 'gate') || view.anchors[0] || null;
          if (model.lastGateTone && tone !== model.lastGateTone) {
            spawnBurst(gateAnchor, crawlerToneColor(tone), tone === 'fail' ? 1.5 : 1, 0);
            if (tone === 'pass' || tone === 'ready') {
              view.anchors
                .filter((a) => a.id !== 'gate')
                .forEach((a, i) => spawnBurst(a, crawlerToneColor(tone), 0.7, i * 120));
            }
          }
          model.lastGateTone = tone;

          const items = Array.isArray(data.verdicts) ? data.verdicts : [];
          for (const item of items) {
            const key = `${item.kind}|${item.at || ''}|${item.stageId || ''}`;
            if (model.seen.has(key)) continue;
            model.seen.add(key);
            if (!model.seeded) continue;
            const toneOfVerdict = VERDICT_TONE[item.kind] || 'muted';
            const color =
              toneOfVerdict === 'fail'
                ? CRAWL_COLORS.fail
                : toneOfVerdict === 'warn'
                  ? CRAWL_COLORS.open
                  : toneOfVerdict === 'pass'
                    ? CRAWL_COLORS.pass
                    : CRAWL_COLORS.working;
            spawnBurst(gateAnchor, color, 1, 0);
          }
          model.seeded = true;
        };

        const step = (dt, now) => {
          const anchors = view.anchors;
          if (!anchors.length) return;
          if (!model.body.x && !model.body.y && model.tour.length) {
            model.body.x = model.tour[0].x;
            model.body.y = model.tour[0].y;
          }
          if (!model.target && model.tour.length) model.target = model.tour[0];
          if (model.target && now > model.dwellUntil) {
            const dx = model.target.x - model.body.x;
            const dy = model.target.y - model.body.y;
            const dist = Math.hypot(dx, dy);
            if (dist < 6) {
              model.tourIndex = (model.tourIndex + 1) % Math.max(1, model.tour.length);
              model.target = model.tour[model.tourIndex] || null;
              model.dwellUntil = now + 1100 + Math.random() * 1600;
            } else {
              const stepLen = Math.min(dist, 96 * dt);
              model.body.x += (dx / dist) * stepLen;
              model.body.y += (dy / dist) * stepLen;
              model.angle = Math.atan2(dy, dx);
            }
          }

          const f = { x: Math.cos(model.angle), y: Math.sin(model.angle) };
          const n = { x: -f.y, y: f.x };
          const t = now / 1000;
          if (model.legs.length !== 8) {
            model.legs = Array.from({ length: 8 }, () => ({ foot: null, lift: 0 }));
          }
          for (let side = -1; side <= 1; side += 2) {
            for (let j = 0; j < 4; j += 1) {
              const idx = (side < 0 ? 0 : 4) + j;
              const leg = model.legs[idx];
              const hip = {
                x: model.body.x + f.x * (7 - j * 5) + n.x * side * 6,
                y: model.body.y + f.y * (7 - j * 5) + n.y * side * 6,
              };
              const reach = 34 + j * 11;
              const sway = Math.sin(t * 2.1 + idx * 0.9) * 6;
              const home = {
                x: model.body.x + f.x * (24 - j * 20) + n.x * side * (reach + sway),
                y: model.body.y + f.y * (24 - j * 20) + n.y * side * (reach + sway),
              };
              if (!leg.foot) leg.foot = { x: home.x, y: home.y };
              const d = Math.hypot(home.x - leg.foot.x, home.y - leg.foot.y);
              if (d > 10 + j * 2) {
                leg.foot.x += (home.x - leg.foot.x) * Math.min(1, dt * 9);
                leg.foot.y += (home.y - leg.foot.y) * Math.min(1, dt * 9);
                leg.lift = Math.min(1, leg.lift + dt * 8);
              } else {
                leg.lift = Math.max(0, leg.lift - dt * 6);
              }
              leg.hip = hip;
            }
          }
        };

        const draw = (now) => {
          const anchors = view.anchors;
          const w = view.w;
          const h = view.h;
          ctx.setTransform(view.dpr, 0, 0, view.dpr, 0, 0);
          ctx.clearRect(0, 0, w, h);
          if (!anchors.length) return;

          const data = dataRef.current || {};
          const gate = data.state && data.state.gate ? data.state.gate : {};
          const gateTone = gate.state || 'none';
          const gateAnchor = anchors.find((a) => a.id === 'gate') || anchors[0];
          const tSec = now / 1000;

          const vg = ctx.createRadialGradient(
            w * 0.7,
            h * 0.42,
            80,
            w * 0.7,
            h * 0.42,
            Math.max(w, h) * 0.72,
          );
          vg.addColorStop(0, 'rgba(3,9,20,0)');
          vg.addColorStop(1, 'rgba(3,9,20,0.10)');
          ctx.fillStyle = vg;
          ctx.fillRect(0, 0, w, h);

          ctx.save();
          ctx.lineWidth = 1;
          for (let i = 0; i < anchors.length - 1; i += 1) {
            const a = anchors[i];
            const b = anchors[i + 1];
            ctx.strokeStyle = 'rgba(120,170,230,0.10)';
            ctx.beginPath();
            ctx.moveTo(a.x, a.y);
            ctx.quadraticCurveTo(
              (a.x + b.x) / 2 - 30,
              (a.y + b.y) / 2 + 20,
              b.x,
              b.y,
            );
            ctx.stroke();
          }
          anchors.forEach((a) => {
            ctx.strokeStyle = 'rgba(120,170,230,0.08)';
            ctx.beginPath();
            ctx.moveTo(a.x, a.y);
            ctx.lineTo(gateAnchor.x, gateAnchor.y);
            ctx.stroke();
          });
          ctx.restore();

          const body = model.body;
          anchors.forEach((a, i) => {
            const dx = a.x - body.x;
            const dy = a.y - body.y;
            const len = Math.hypot(dx, dy) || 1;
            const off = Math.sin(i * 1.7 + tSec * 0.3) * Math.min(96, len * 0.3);
            const c1 = {
              x: body.x + dx * 0.3 - (dy / len) * off,
              y: body.y + dy * 0.3 + (dx / len) * off,
            };
            const c2 = {
              x: body.x + dx * 0.7 - (dy / len) * off * 0.55,
              y: body.y + dy * 0.7 + (dx / len) * off * 0.55,
            };
            const color = crawlerToneColor(a.tone);
            const active =
              a.id !== 'gate' ||
              ['fail', 'open', 'waiting', 'working', 'active'].includes(gateTone);
            ctx.save();
            ctx.strokeStyle = color;
            ctx.globalAlpha = active ? 0.38 : 0.16;
            ctx.lineWidth = a.id === 'gate' ? 1.6 : 1.05;
            if (['fail', 'open', 'waiting'].includes(a.tone)) {
              ctx.setLineDash([5, 6]);
              ctx.lineDashOffset = -tSec * 26;
            }
            ctx.shadowColor = color;
            ctx.shadowBlur = active ? 8 : 3;
            ctx.beginPath();
            ctx.moveTo(body.x, body.y);
            ctx.bezierCurveTo(c1.x, c1.y, c2.x, c2.y, a.x, a.y);
            ctx.stroke();
            ctx.restore();

            for (let p = 0; p < 2; p += 1) {
              const t = (tSec * 0.28 + i * 0.17 + p * 0.5) % 1;
              const pt = crawlerBezier(body, c1, c2, a, t);
              ctx.save();
              ctx.fillStyle = color;
              ctx.globalAlpha = 0.85 - Math.abs(t - 0.5) * 0.5;
              ctx.shadowColor = color;
              ctx.shadowBlur = 9;
              ctx.beginPath();
              ctx.arc(pt.x, pt.y, 2.1, 0, Math.PI * 2);
              ctx.fill();
              ctx.restore();
            }
          });

          anchors.forEach((a) => {
            const color = crawlerToneColor(a.tone);
            const pulse =
              a.tone === 'fail'
                ? 1 + Math.sin(tSec * 9) * 0.35
                : a.tone === 'open' || a.tone === 'waiting'
                  ? 1 + Math.sin(tSec * 5) * 0.22
                  : 1;
            ctx.save();
            ctx.strokeStyle = color;
            ctx.fillStyle = 'rgba(5,14,26,0.85)';
            ctx.shadowColor = color;
            ctx.shadowBlur = 10;
            ctx.globalAlpha = 0.9;
            ctx.beginPath();
            ctx.arc(a.x, a.y, 5.2 * pulse, 0, Math.PI * 2);
            ctx.fill();
            ctx.stroke();
            ctx.globalAlpha = 0.35;
            ctx.beginPath();
            ctx.arc(a.x, a.y, 10 * pulse, 0, Math.PI * 2);
            ctx.stroke();
            if (a.id === 'gate' && ['fail', 'open', 'waiting'].includes(gateTone)) {
              ctx.globalAlpha = 0.5;
              ctx.setLineDash([3, 4]);
              ctx.beginPath();
              ctx.arc(a.x, a.y, 16 + Math.sin(tSec * 4) * 2, 0, Math.PI * 2);
              ctx.stroke();
            }
            ctx.restore();
          });

          const bodyColor = crawlerToneColor(gateTone === 'none' ? 'working' : gateTone);
          model.legs.forEach((leg, idx) => {
            if (!leg.hip || !leg.foot) return;
            const side = idx < 4 ? -1 : 1;
            const foot = { x: leg.foot.x, y: leg.foot.y - leg.lift * 5 };
            const solved = crawlerIK(leg.hip, foot, 24, 26, side);
            ctx.save();
            ctx.strokeStyle =
              idx % 2 === 0 ? 'rgba(160,238,255,0.85)' : 'rgba(255,111,222,0.75)';
            ctx.lineWidth = 1.35;
            ctx.lineCap = 'round';
            ctx.shadowColor = '#38e1ff';
            ctx.shadowBlur = 5;
            ctx.beginPath();
            ctx.moveTo(leg.hip.x, leg.hip.y);
            ctx.lineTo(solved.knee.x, solved.knee.y);
            ctx.lineTo(solved.foot.x, solved.foot.y);
            ctx.stroke();
            ctx.fillStyle = '#ff8ee6';
            ctx.beginPath();
            ctx.arc(solved.foot.x, solved.foot.y, 1.5, 0, Math.PI * 2);
            ctx.fill();
            ctx.restore();
          });

          ctx.save();
          ctx.translate(body.x, body.y + Math.sin(tSec * 3.2) * 1.4);
          ctx.rotate(model.angle);
          ctx.shadowColor = bodyColor;
          ctx.shadowBlur = 16;
          const grad = ctx.createRadialGradient(-4, 0, 2, -4, 0, 16);
          grad.addColorStop(0, '#eafcff');
          grad.addColorStop(0.45, bodyColor);
          grad.addColorStop(1, 'rgba(10,22,38,0.9)');
          ctx.fillStyle = grad;
          ctx.beginPath();
          ctx.ellipse(-8, 0, 14, 10, 0, 0, Math.PI * 2);
          ctx.fill();
          ctx.fillStyle = '#0b1a2c';
          ctx.strokeStyle = bodyColor;
          ctx.lineWidth = 1.2;
          ctx.beginPath();
          ctx.ellipse(7, 0, 7.5, 6, 0, 0, Math.PI * 2);
          ctx.fill();
          ctx.stroke();
          ctx.fillStyle = '#d8fbff';
          ctx.shadowColor = '#38e1ff';
          ctx.shadowBlur = 8;
          ctx.beginPath();
          ctx.arc(12.5, -2.6, 1.25, 0, Math.PI * 2);
          ctx.arc(12.5, 2.6, 1.25, 0, Math.PI * 2);
          ctx.fill();
          ctx.fillStyle = 'rgba(255,79,216,0.9)';
          ctx.shadowColor = '#ff4fd8';
          ctx.shadowBlur = 6;
          ctx.beginPath();
          ctx.arc(-11, -3.2, 1.5, 0, Math.PI * 2);
          ctx.arc(-11, 3.2, 1.5, 0, Math.PI * 2);
          ctx.arc(-4, 0, 1.8, 0, Math.PI * 2);
          ctx.fill();
          ctx.restore();

          model.bursts = model.bursts.filter((b) => now - b.t0 < b.life);
          model.bursts.forEach((b) => {
            if (now < (b.startAt || 0)) return;
            const age = (now - b.t0) / b.life;
            const r = 8 + age * (90 + (b.power || 1) * 60);
            ctx.save();
            ctx.strokeStyle = b.color;
            ctx.globalAlpha = Math.max(0, 1 - age) * 0.9;
            ctx.lineWidth = 2.2 * (1 - age) + 0.4;
            ctx.shadowColor = b.color;
            ctx.shadowBlur = 14;
            ctx.beginPath();
            ctx.arc(b.x, b.y, r, 0, Math.PI * 2);
            ctx.stroke();
            ctx.restore();
            if (age < 0.55) {
              for (let s = 0; s < 6; s += 1) {
                const ang = (Math.PI * 2 * s) / 6 + age * 2;
                const rr = r * 0.8;
                ctx.save();
                ctx.fillStyle = b.color;
                ctx.globalAlpha = (1 - age) * 0.8;
                ctx.beginPath();
                ctx.arc(b.x + Math.cos(ang) * rr, b.y + Math.sin(ang) * rr, 1.8, 0, Math.PI * 2);
                ctx.fill();
                ctx.restore();
              }
            }
          });
        };

        const frame = () => {
          if (!alive) return;
          const now = Date.now();
          const dt = Math.min(0.05, (now - (last || now)) / 1000);
          last = now;
          anchorsNow();
          sync();
          step(dt, now);
          draw(now);
          raf = window.requestAnimationFrame(frame);
        };

        const start = () => {
          if (!alive || reduced) return;
          window.cancelAnimationFrame(raf);
          last = 0;
          raf = window.requestAnimationFrame(frame);
        };

        const onVisibility = () => {
          if (document.hidden) window.cancelAnimationFrame(raf);
          else start();
        };

        resize();
        anchorsNow();
        sync();
        step(0.016, Date.now());
        draw(Date.now());
        start();

        window.addEventListener('resize', resize);
        window.addEventListener('resize', start);
        document.addEventListener('visibilitychange', onVisibility);

        return () => {
          alive = false;
          window.cancelAnimationFrame(raf);
          window.removeEventListener('resize', resize);
          window.removeEventListener('resize', start);
          document.removeEventListener('visibilitychange', onVisibility);
        };
      }, [enabled, reduced]);

      if (!enabled) return null;
      return h('canvas', {
        ref: canvasRef,
        className: 'dwh-crawler',
        'aria-hidden': 'true',
      });
    }

    function createHud(ctx, t) {
      return function WorkflowHud() {
        const [state, setState] = useState(null);
        const [verdicts, setVerdicts] = useState([]);
        const [connected, setConnected] = useState(false);
        const [collapsed, setCollapsed] = useState(false);
        const [web, setWeb] = useState(() => {
          try {
            if (typeof window !== 'undefined' && window.localStorage) {
              return window.localStorage.getItem('dwh.web') !== '0';
            }
          } catch {
            // Private mode or a blocked storage API: keep the effect on.
          }
          return true;
        });
        const [, setLocaleRevision] = useState(0);
        const stateRef = useRef(null);
        const reduced = useReducedMotion();

        useEffect(() => {
          try {
            if (typeof window !== 'undefined' && window.localStorage) {
              window.localStorage.setItem('dwh.web', web ? '1' : '0');
            }
          } catch {
            // Storage is best-effort; the toggle still works for this page.
          }
        }, [web]);

        useEffect(() => {
          stateRef.current = state;
        }, [state]);

        useEffect(() => {
          const off = ctx.locale.subscribe(() => setLocaleRevision((value) => value + 1));
          return off;
        }, []);

        useEffect(() => {
          if (typeof EventSource === 'undefined') return undefined;
          const source = new EventSource('/plugins/workflow-hud/events');
          source.onopen = () => setConnected(true);
          source.onerror = () => setConnected(false);
          source.onmessage = (event) => {
            let frame;
            try {
              frame = JSON.parse(event.data);
            } catch {
              return;
            }
            if (!frame || typeof frame !== 'object') return;
            if (frame.type === 'state') {
              // The gate log is session-owned; the Host pushes the new
              // session's array separately, which can precede the state frame.
              stateRef.current = frame;
              setState(frame);
              setConnected(true);
              return;
            }
            if (frame.type === 'verdicts') {
              const items = Array.isArray(frame.items) ? frame.items : [];
              setVerdicts(items.filter((item) => item && typeof item === 'object').slice(-8));
            }
          };
          return () => source.close();
        }, []);

        const status = typeof state?.status === 'string' ? state.status : '';
        const statusKey = STATUS_KEYS.has(status) ? status : 'idle';
        const stages = Array.isArray(state?.stages) ? state.stages : [];
        const gates = Array.isArray(state?.stageGates) ? state.stageGates : [];
        const catalog = Array.isArray(state?.catalog) ? state.catalog : [];
        // Passed stages survive a reload as evidence rows; live verdicts only
        // exist for this Host process. One log combines both.
        const history = Array.isArray(state?.gate?.history) ? state.gate.history : [];
        const gateLog = [...history, ...verdicts].slice(-8);
        const runState = typeof state?.runState === 'string' ? state.runState : 'none';
        const association = typeof state?.association === 'string' ? state.association : 'none';
        const feature = state?.feature || '';
        const finished = runState === 'finished' || status === 'done';
        const abandoned = runState === 'abandoned' || status === 'abandoned';
        const stageIndex =
          stages.length && state
            ? stages.findIndex((stage) => Number(stage.id) === Number(state.stage))
            : -1;
        const current = finished
          ? stages.length - 1
          : stageIndex;
        const stepNumber = finished
          ? stages.length
          : stageIndex >= 0
            ? Math.min(stageIndex + 1, stages.length)
            : 0;
        const progress =
          stages.length === 0
            ? 0
            : finished
              ? 100
              : stageIndex >= 0
                ? Math.min(100, ((stageIndex + 1) / stages.length) * 100)
                : 0;
        const showCard =
          Boolean(feature) ||
          catalog.length > 0 ||
          verdicts.length > 0 ||
          runState !== 'none';
        const statusLabel = finished
          ? t('status.done')
          : abandoned
            ? t('status.abandoned')
            : status === 'waiting'
              ? t('status.waiting')
              : status === 'working'
                ? t('status.working')
                : t('status.idle');
        const assocKey =
          association === 'session'
            ? 'assocSession'
            : association === 'repo'
              ? 'assocRepo'
              : association === 'missing'
                ? 'assocMissing'
                : 'assocNone';
        const assocClass =
          association === 'session'
            ? 'is-session'
            : association === 'repo'
              ? 'is-repo'
              : association === 'missing'
                ? 'is-missing'
                : '';
        const runKey =
          runState === 'active'
            ? 'runActive'
            : runState === 'finished'
              ? 'runFinished'
              : runState === 'abandoned'
                ? 'runAbandoned'
                : 'runNone';
        const featureLabel = feature || t('noRun');

        const logo = h(
          'svg',
          { viewBox: '0 0 24 24', 'aria-hidden': true, focusable: 'false' },
          h('path', {
            d: 'M2.5 12h4.2l2.1-6 4.2 12 2.1-6h6.4',
            fill: 'none',
            stroke: 'currentColor',
            strokeWidth: 2,
            strokeLinecap: 'round',
            strokeLinejoin: 'round',
          }),
        );

        const link = h('span', {
          className: `dwh-link ${connected ? 'is-on' : 'is-off'}`,
          role: 'img',
          'aria-label': connected ? t('live') : t('offline'),
          title: connected ? t('live') : t('offline'),
        });

        if (!showCard) {
          if (!connected) return null;
          return h(
            'div',
            { className: 'dwh-root' },
            h('style', null, CSS),
            h(
              'div',
              { className: 'dwh-pill is-static' },
              h('span', { className: 'dwh-logo' }, logo),
              h('span', { className: 'dwh-wordmark' }, 'FLOW'),
              h('span', { className: 'dwh-pill-status' }, t('standby')),
              link,
            ),
          );
        }

        if (collapsed) {
          return h(
            'div',
            { className: 'dwh-root' },
            h('style', null, CSS),
            h(
              'button',
              {
                type: 'button',
                className: 'dwh-pill',
                onClick: () => setCollapsed(false),
                'aria-label': t('expand'),
              },
              h('span', { className: 'dwh-logo' }, logo),
              h('span', { className: 'dwh-pill-name' }, featureLabel),
              h('span', { className: 'dwh-pill-status' }, statusLabel),
              link,
            ),
          );
        }

        return h(
          'div',
          { className: 'dwh-root' },
          h('style', null, CSS),
          h(Crawler, {
            state,
            verdicts: gateLog,
            // `reduced` renders one static frame instead of stopping the layer.
            enabled: web,
            reduced,
          }),
          h(
            'section',
            {
              className: 'dwh-card',
              'aria-label': t('title'),
            },
            h('div', { className: 'dwh-sweep', 'aria-hidden': true }),
            h(
              'header',
              { className: 'dwh-head' },
              h('span', { className: 'dwh-logo' }, logo),
              h('span', { className: 'dwh-wordmark' }, 'FLOW'),
              h(
                'span',
                { className: 'dwh-wf' },
                state?.workflow || 'data-workflow',
              ),
              h('span', { className: 'dwh-spacer' }),
              link,
              h(
                'span',
                { className: `dwh-status dwh-status-${statusKey}` },
                h('i'),
                statusLabel,
              ),
              h(
                'button',
                {
                  type: 'button',
                  className: `dwh-icon-btn${web ? ' is-on' : ''}`,
                  onClick: () => setWeb((value) => !value),
                  'aria-label': web ? t('webOff') : t('webOn'),
                  title: web ? t('webOff') : t('webOn'),
                },
                '⌁',
              ),
              h(
                'button',
                {
                  type: 'button',
                  className: 'dwh-icon-btn',
                  onClick: () => setCollapsed(true),
                  'aria-label': t('collapse'),
                },
                '−',
              ),
            ),
            h(
              'div',
              { className: 'dwh-body' },
              h(
                'div',
                { className: 'dwh-feature' },
                h('span', { className: 'dwh-feature-name' }, featureLabel),
                h(
                  'span',
                  { className: `dwh-assoc ${assocClass}` },
                  `${t(assocKey)} · ${t(runKey)}`,
                ),
                state?.repoName
                  ? h('span', { className: 'dwh-repo', title: state.repo || '' }, state.repoName)
                  : null,
              ),
              feature && stages.length
                ? h(StageRail, {
                    stages,
                    gates,
                    current,
                    finished,
                    reduced,
                    t,
                  })
                : null,
              feature && stages.length
                ? h(
                    'div',
                    { className: 'dwh-caption' },
                    h(
                      'span',
                      { className: 'dwh-caption-label' },
                      finished
                        ? t('complete')
                        : abandoned
                          ? t('status.abandoned')
                          : state?.gate?.stageName || statusLabel,
                    ),
                    h(
                      'span',
                      { className: 'dwh-caption-meta' },
                      `${t('step')} ${stepNumber}/${stages.length}`,
                    ),
                  )
                : null,
              feature && stages.length
                ? h(
                    'div',
                    { className: 'dwh-progress' },
                    h('i', { style: { width: `${progress}%` } }),
                  )
                : null,
              feature ? h(GateBanner, { gate: state?.gate, t }) : null,
              feature ? h(SummaryBar, { gate: state?.gate, t }) : null,
              feature
                ? h(StageList, { gates, currentId: state?.stage, t })
                : h(
                    React.Fragment,
                    null,
                    h(
                      'div',
                      {
                        className: 'dwh-gate dwh-tone-none',
                        'data-dwh-anchor': 'gate',
                        'data-dwh-tone': 'none',
                      },
                      h(
                        'div',
                        { className: 'dwh-gate-head' },
                        h('span', { className: 'dwh-gate-dot', 'aria-hidden': true }),
                        h('span', { className: 'dwh-gate-label' }, t('noRun')),
                      ),
                      h('div', { className: 'dwh-gate-reason' }, t('noRunHint')),
                    ),
                    h(Catalog, { catalog, t }),
                  ),
              state?.status === 'waiting' && state.question
                ? h(
                    'div',
                    { className: 'dwh-question' },
                    h('b', null, t('question')),
                    h('span', null, state.question),
                  )
                : null,
              h(
                'div',
                { className: 'dwh-section' },
                h(
                  'div',
                  { className: 'dwh-section-head' },
                  h('span', null, t('verdicts')),
                  h('span', { className: 'dwh-section-count' }, String(gateLog.length)),
                ),
                h(Verdicts, { items: gateLog, t }),
              ),
            ),
          ),
        );
      };
    }

    return {
      name: 'workflow-hud',
      inject: ['slots', 'locale'],
      apply(ctx) {
        const t = ctx.locale.bind(NS);
        const Hud = createHud(ctx, t);

        ctx.effect(
          () => {
            const disposers = [
              ctx.locale.register(NS, 'en', EN),
              ctx.locale.register(NS, 'zh', ZH),
            ];
            return () => {
              for (const dispose of disposers) {
                try {
                  dispose();
                } catch {
                  // Already disposed with the fiber.
                }
              }
            };
          },
          'workflow-hud: locale dictionaries',
        );

        ctx.effect(
          () =>
            ctx.slots.inject('shell.overlay', () =>
              ctx.slots.register(
                {
                  name: 'shell.overlay',
                  id: 'workflow-hud',
                  order: 35,
                },
                Hud,
              ),
            ),
          'workflow-hud: overlay entry',
        );
      },
    };
  },
});
