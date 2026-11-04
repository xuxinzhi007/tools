#!/usr/bin/env node

/**
 * Universal Excalidraw Diagram Generator
 * 
 * Generates:
 * 1. <name>.excalidraw (Official Excalidraw JSON format)
 * 2. <name>.svg (High-fidelity vector graphic optimized for GitHub README)
 * 3. <name>.png (High-resolution bitmap via rsvg-convert if available)
 * 
 * Usage:
 *   node diagram_generator.js spec.json [outputDir] [namePrefix]
 *   or require('./diagram_generator.js').generateDiagram(spec, options)
 */

const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

// 经典 Excalidraw / 现代化手绘柔和调色盘
const PALETTES = {
  gray:   { bg: '#f8f9fa', stroke: '#868e96', text: '#212529', lightText: '#868e96', line: '#dee2e6' },
  green:  { bg: '#ebfbee', stroke: '#0ca678', text: '#0ca678', lightText: '#868e96', line: '#b2f2bb' },
  blue:   { bg: '#e7f5ff', stroke: '#1971c2', text: '#1971c2', lightText: '#868e96', line: '#a5d8ff' },
  purple: { bg: '#f3f0ff', stroke: '#7048e8', text: '#7048e8', lightText: '#868e96', line: '#d0bfff' },
  orange: { bg: '#fff4e6', stroke: '#e8590c', text: '#e8590c', lightText: '#868e96', line: '#ffc078' },
  red:    { bg: '#fff5f5', stroke: '#e03131', text: '#c92a2a', lightText: '#868e96', line: '#ffc9c9' },
  dark:   { bg: '#f1f3f5', stroke: '#343a40', text: '#212529', lightText: '#868e96', line: '#ced4da' }
};

class DiagramBuilder {
  constructor(spec = {}) {
    this.spec = spec;
    this.title = spec.title || "系统架构与数据流";
    this.subtitle = spec.subtitle || "";
    this.width = spec.width || 1200;
    this.height = spec.height || 920;
    this.cards = spec.cards || [];
    this.arrows = spec.arrows || [];
    this.configBoxes = spec.configBoxes || [];
  }

  buildExcalidraw() {
    let idCounter = 1;
    const genId = (prefix) => `${prefix}_${idCounter++}_${Math.random().toString(36).substr(2, 6)}`;
    const elements = [];

    const addRect = (cfg) => {
      elements.push({
        id: cfg.id || genId('rect'),
        type: 'rectangle',
        x: cfg.x, y: cfg.y, width: cfg.width, height: cfg.height,
        angle: 0,
        strokeColor: cfg.strokeColor || '#1e1e1e',
        backgroundColor: cfg.backgroundColor || '#ffffff',
        fillStyle: 'solid',
        strokeWidth: cfg.strokeWidth || 2,
        strokeStyle: cfg.strokeStyle || 'solid',
        roughness: 1,
        opacity: 100,
        groupIds: [],
        frameId: null,
        roundness: { type: 3 },
        seed: Math.floor(Math.random() * 1000000),
        version: 1, versionNonce: 1, isDeleted: false,
        boundElements: null, updated: Date.now(), link: null, locked: false
      });
    };

    const addText = (cfg) => {
      elements.push({
        id: cfg.id || genId('text'),
        type: 'text',
        x: cfg.x, y: cfg.y, width: cfg.width, height: cfg.height,
        angle: 0,
        strokeColor: cfg.strokeColor || '#1e1e1e',
        backgroundColor: 'transparent',
        fillStyle: 'solid',
        strokeWidth: 1, strokeStyle: 'solid', roughness: 1, opacity: 100,
        groupIds: [], frameId: null, roundness: null,
        seed: Math.floor(Math.random() * 1000000),
        version: 1, versionNonce: 1, isDeleted: false, boundElements: null,
        updated: Date.now(), link: null, locked: false,
        text: cfg.text,
        fontSize: cfg.fontSize || 16,
        fontFamily: cfg.fontFamily || 1,
        textAlign: cfg.textAlign || 'center',
        verticalAlign: 'middle',
        containerId: null,
        originalText: cfg.text,
        lineHeight: 1.3
      });
    };

    const addArrow = (cfg) => {
      const endPoint = cfg.points[cfg.points.length - 1];
      elements.push({
        id: cfg.id || genId('arrow'),
        type: 'arrow',
        x: cfg.startX, y: cfg.startY,
        width: Math.abs(endPoint[0]),
        height: Math.abs(endPoint[1]),
        angle: 0,
        strokeColor: cfg.strokeColor || '#495057',
        backgroundColor: 'transparent',
        fillStyle: 'solid',
        strokeWidth: cfg.strokeWidth || 2,
        strokeStyle: cfg.strokeStyle || 'solid',
        roughness: 1, opacity: 100,
        groupIds: [], frameId: null, roundness: { type: 2 },
        seed: Math.floor(Math.random() * 1000000),
        version: 1, versionNonce: 1, isDeleted: false,
        points: cfg.points,
        startBinding: null, endBinding: null,
        startArrowhead: null, endArrowhead: 'arrow'
      });
    };

    // 1. 标题
    addText({
      x: (this.width - 800) / 2, y: 35, width: 800, height: 45,
      text: this.title, fontSize: 32, strokeColor: "#1e1e1e", textAlign: "center"
    });
    if (this.subtitle) {
      addText({
        x: (this.width - 700) / 2, y: 85, width: 700, height: 26,
        text: this.subtitle, fontSize: 18, strokeColor: "#495057", textAlign: "center"
      });
    }

    // 2. 卡片
    this.cards.forEach(c => {
      const palette = PALETTES[c.theme] || PALETTES.gray;
      addRect({
        x: c.x, y: c.y, width: c.w, height: c.h,
        backgroundColor: palette.bg, strokeColor: palette.stroke, strokeWidth: 2
      });
      addText({
        x: c.x + 10, y: c.y + 15, width: c.w - 20, height: 28,
        text: c.title, fontSize: 20, strokeColor: palette.text
      });
      if (c.sub) {
        addText({
          x: c.x + 10, y: c.y + 42, width: c.w - 20, height: 20,
          text: c.sub, fontSize: 13, strokeColor: palette.lightText
        });
      }
      let curY = c.y + (c.sub ? 75 : 55);
      if (c.items) {
        c.items.forEach(it => {
          addText({
            x: c.x + 20, y: curY, width: c.w - 40, height: 24,
            text: `• ${it}`, fontSize: 15, strokeColor: "#2b2b2b", textAlign: "left"
          });
          curY += 30;
        });
      }
    });

    // 3. 连线与标签
    this.arrows.forEach(a => {
      addArrow({
        startX: a.startX, startY: a.startY,
        points: a.points,
        strokeColor: a.strokeColor || "#495057",
        strokeWidth: a.strokeWidth || 2,
        strokeStyle: a.strokeStyle || "solid"
      });
      if (a.label && a.labelPos) {
        addText({
          x: a.labelPos.x, y: a.labelPos.y,
          width: a.labelPos.w || 80, height: 22,
          text: a.label, fontSize: a.labelPos.fontSize || 13,
          strokeColor: a.labelColor || "#495057",
          textAlign: a.labelPos.align || "center"
        });
      }
    });

    // 4. 配置中心与横幅
    this.configBoxes.forEach(box => {
      const palette = PALETTES[box.theme || 'red'];
      addRect({
        x: box.x, y: box.y, width: box.w, height: box.h,
        backgroundColor: palette.bg, strokeColor: palette.stroke, strokeWidth: 2
      });
      addText({
        x: box.x + 20, y: box.y + 18, width: box.w - 40, height: 30,
        text: box.title, fontSize: 22, strokeColor: palette.text
      });
      if (box.sub) {
        addText({
          x: box.x + 20, y: box.y + 50, width: box.w - 40, height: 22,
          text: box.sub, fontSize: 14, strokeColor: "#495057"
        });
      }
      let bY = box.y + 85;
      if (box.items) {
        box.items.forEach(it => {
          addText({
            x: box.x + 30, y: bY, width: box.w - 60, height: 24,
            text: `• ${it}`, fontSize: 14.5, strokeColor: "#2b2b2b", textAlign: "left"
          });
          bY += 28;
        });
      }
    });

    return {
      type: "excalidraw",
      version: 2,
      source: "https://excalidraw.com",
      elements,
      appState: { gridSize: null, viewBackgroundColor: "#ffffff" },
      files: {}
    };
  }

  buildSVG() {
    const cardsSvg = this.cards.map(c => {
      const p = PALETTES[c.theme] || PALETTES.gray;
      const itemsSvg = (c.items || []).map((it, idx) => `<text y="${idx * 28}">• ${escapeXml(it)}</text>`).join('\n        ');
      return `
    <g transform="translate(${c.x}, ${c.y})" filter="url(#shadow)">
      <rect width="${c.w}" height="${c.h}" rx="14" fill="${p.bg}" stroke="${p.stroke}" stroke-width="2"/>
      <text class="card-title" x="${c.w / 2}" y="36" text-anchor="middle" fill="${p.text}">${escapeXml(c.title)}</text>
      ${c.sub ? `<text class="card-sub" x="${c.w / 2}" y="56" text-anchor="middle">${escapeXml(c.sub)}</text>` : ''}
      <line x1="25" y1="${c.sub ? 72 : 55}" x2="${c.w - 25}" y2="${c.sub ? 72 : 55}" stroke="${p.line}" stroke-width="1.5" stroke-dasharray="4,4"/>
      <g class="card-item" transform="translate(24, ${c.sub ? 102 : 85})">
        ${itemsSvg}
      </g>
    </g>`;
    }).join('\n');

    const arrowsSvg = this.arrows.map(a => {
      const isRed = a.strokeColor === '#e03131' || (a.marker === 'red');
      const markerAttr = isRed ? 'url(#arrow-red)' : 'url(#arrow-gray)';
      const dashAttr = a.strokeStyle === 'dashed' ? 'stroke-dasharray="5,5"' : '';
      const color = a.strokeColor || '#495057';
      const width = a.strokeWidth || 2.2;

      let pathD = '';
      if (a.path) {
        pathD = a.path;
      } else if (a.points && a.points.length === 2) {
        const x1 = a.startX, y1 = a.startY;
        const x2 = x1 + a.points[1][0], y2 = y1 + a.points[1][1];
        pathD = `M ${x1} ${y1} L ${x2} ${y2}`;
      } else if (a.points) {
        let curX = a.startX, curY = a.startY;
        pathD = `M ${curX} ${curY}`;
        for (let i = 1; i < a.points.length; i++) {
          curX = a.startX + a.points[i][0];
          curY = a.startY + a.points[i][1];
          pathD += ` L ${curX} ${curY}`;
        }
      }

      let labelSvg = '';
      if (a.label && a.labelPos) {
        const lp = a.labelPos;
        const tagClass = isRed ? 'cfg-text' : 'tag-text';
        const tagFill = isRed ? '#ffe3e3' : '#f1f3f5';
        const tagStroke = isRed ? 'stroke="#ffa8a8"' : '';
        labelSvg = `
      <rect x="${lp.x - (lp.w || 60) / 2}" y="${lp.y - 14}" width="${lp.w || 60}" height="22" rx="4" fill="${tagFill}" ${tagStroke}/>
      <text class="${tagClass}" x="${lp.x}" y="${lp.y}" text-anchor="middle">${escapeXml(a.label)}</text>`;
      }

      return `
    <g>
      <path d="${pathD}" fill="none" stroke="${color}" stroke-width="${width}" ${dashAttr} marker-end="${markerAttr}"/>
      ${labelSvg}
    </g>`;
    }).join('\n');

    const configSvg = this.configBoxes.map(b => {
      const p = PALETTES[b.theme || 'red'];
      const itemsSvg = (b.items || []).map((it, idx) => `<text y="${idx * 28}">• ${parseBold(it)}</text>`).join('\n        ');
      return `
    <g transform="translate(${b.x}, ${b.y})" filter="url(#red-shadow)">
      <rect width="${b.w}" height="${b.h}" rx="16" fill="${p.bg}" stroke="${p.stroke}" stroke-width="2.5"/>
      <text class="card-title" x="${b.w / 2}" y="36" text-anchor="middle" fill="${p.text}" font-size="22">${escapeXml(b.title)}</text>
      ${b.sub ? `<text class="card-sub" x="${b.w / 2}" y="58" text-anchor="middle" font-size="14">${escapeXml(b.sub)}</text>` : ''}
      <line x1="30" y1="74" x2="${b.w - 30}" y2="74" stroke="${p.line}" stroke-width="1.5" stroke-dasharray="4,4"/>
      <g class="card-item" transform="translate(35, 104)" font-size="15">
        ${itemsSvg}
      </g>
    </g>`;
    }).join('\n');

    return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${this.width} ${this.height}" width="100%" height="100%">
  <defs>
    <filter id="shadow" x="-5%" y="-5%" width="110%" height="110%" filterUnits="userSpaceOnUse">
      <feDropShadow dx="0" dy="4" stdDeviation="6" flood-color="#000000" flood-opacity="0.07"/>
    </filter>
    <filter id="red-shadow" x="-4%" y="-4%" width="108%" height="108%" filterUnits="userSpaceOnUse">
      <feDropShadow dx="0" dy="5" stdDeviation="8" flood-color="#e03131" flood-opacity="0.10"/>
    </filter>
    <marker id="arrow-gray" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
      <path d="M 0 1.5 L 7.5 4.5 L 0 7.5 Z" fill="#495057" />
    </marker>
    <marker id="arrow-red" markerWidth="9" markerHeight="9" refX="7" refY="4.5" orient="auto">
      <path d="M 0 1.5 L 7.5 4.5 L 0 7.5 Z" fill="#e03131" />
    </marker>
    <style>
      .title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif; font-weight: 800; fill: #1e1e1e; }
      .subtitle { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 500; fill: #495057; }
      .card-title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-weight: 700; font-size: 19px; }
      .card-sub { font-family: "SF Mono", "Consolas", Menlo, monospace; font-size: 13px; fill: #868e96; }
      .card-item { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 14.5px; fill: #2b2b2b; }
      .tag-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 13px; font-weight: 600; fill: #495057; text-anchor: middle; }
      .cfg-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", sans-serif; font-size: 13px; font-weight: 700; fill: #c92a2a; text-anchor: middle; }
    </style>
  </defs>

  <rect width="100%" height="100%" fill="#ffffff" rx="16"/>

  <g transform="translate(${this.width / 2}, 50)" text-anchor="middle">
    <text class="title" font-size="30" y="0">${escapeXml(this.title)}</text>
    ${this.subtitle ? `<text class="subtitle" font-size="16" y="32">${escapeXml(this.subtitle)}</text>` : ''}
  </g>

  <!-- 卡片 -->
  ${cardsSvg}

  <!-- 连线与标签 -->
  ${arrowsSvg}

  <!-- 配置驱动底框 -->
  ${configSvg}
</svg>`;
  }

  generate(outputDir = './docs', namePrefix = 'architecture') {
    if (!fs.existsSync(outputDir)) {
      fs.mkdirSync(outputDir, { recursive: true });
    }

    const excalidrawPath = path.join(outputDir, `${namePrefix}.excalidraw`);
    fs.writeFileSync(excalidrawPath, JSON.stringify(this.buildExcalidraw(), null, 2), 'utf-8');

    const svgPath = path.join(outputDir, `${namePrefix}.svg`);
    fs.writeFileSync(svgPath, this.buildSVG(), 'utf-8');

    const pngPath = path.join(outputDir, `${namePrefix}.png`);
    try {
      execSync(`rsvg-convert -w 2400 "${svgPath}" -o "${pngPath}"`);
    } catch (e) {
      // ignore if rsvg-convert not present
    }

    return { excalidrawPath, svgPath, pngPath };
  }
}

function escapeXml(str = '') {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;');
}

function parseBold(str = '') {
  // 简易将 **text** 转换为 <tspan font-weight="700">text</tspan>
  const escaped = escapeXml(str);
  return escaped.replace(/\*\*(.*?)\*\*/g, '<tspan font-weight="700">$1</tspan>');
}

function generateDiagram(spec, options = {}) {
  const builder = new DiagramBuilder(spec);
  return builder.generate(options.outputDir, options.namePrefix);
}

// CLI 执行入口
if (require.main === module) {
  const args = process.argv.slice(2);
  if (args.length === 0) {
    console.log("Usage: node diagram_generator.js <spec.json> [outputDir] [namePrefix]");
    process.exit(1);
  }
  const specFile = path.resolve(args[0]);
  const outputDir = args[1] || './docs';
  const namePrefix = args[2] || 'architecture';
  const spec = JSON.parse(fs.readFileSync(specFile, 'utf-8'));
  const res = generateDiagram(spec, { outputDir, namePrefix });
  console.log(`✓ Diagram generated:\n  - ${res.excalidrawPath}\n  - ${res.svgPath}\n  - ${res.pngPath}`);
}

module.exports = {
  DiagramBuilder,
  generateDiagram,
  PALETTES
};
