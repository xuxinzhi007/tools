/**
 * Data Service API Test & SQL Log Platform - Frontend Application
 */

const STATE = {
  spec: null,
  endpoints: [],
  groups: [],
  selectedEndpoint: null,
  currentView: 'api', // 'api' | 'logs'
  env: 'local',
  baseURL: 'http://127.0.0.1:8005',
  routePrefix: '',
  dataScope: 'data', // 'data', 'full'
  customBaseURL: '',
  environments: [],
  auth: {
    token: '',
    tenantId: '',
    userId: '',
    enableXrAuth: false,
    xrAuthType: '',
    xrAuthUid: '',
    xrAuthSub: '',
    xrAuthTid: '',
    xrAuthAut: '',
    slowThreshold: '200ms',
    // 由 /api/config 的 profile_headers 填充，代码里不写死任何网关头
    profileHeaders: [],
  },
  customHeaders: [
    { key: 'Content-Type', value: 'application/json', enabled: true }
  ],
  pathParams: {},
  queryParams: {},
  logs: [],
  sqlStats: {
    total_queries: 0,
    slow_queries: 0,
    fast_queries: 0,
    avg_latency_ms: 0,
    max_latency_ms: 0,
    slowest_sqls: []
  },
  logFilters: {
    preset: 'all', // 'all', 'sql', 'slow', 'error'
    level: '',
    search: '',
  },
  autoScroll: true,
  favorites: [],
  sourceStates: [],
  wsRetry: 0,
  history: [],
  ws: null,
  activeTraceID: '',
  methodFilter: 'ALL',
  lastResponseBody: '',
  lastResponseParsed: null,
  respFormat: 'pretty',
  respSearchKeyword: '',
  remoteSearching: false,
  remoteLog: {
    workflow_url: '',
    auth_token: '',
    service: '',
    source: 'default',
    query_mode: 'keyword',
    time_range: 'realtime',
    page_size: 100,
    poll_interval_ms: 2000,
    log_source: 'auto',
    environments: {}
  },
  remoteLogStatus: null,
};

// =========================================================================
// Initialization & Theme
// =========================================================================
document.addEventListener('DOMContentLoaded', () => {
  initTheme();
  loadConfig();
  loadOpenAPI();
  initWebSocket();
  loadRecentLogs();
  loadLocalHistory();
  loadFavorites();

  // Poll remote log status periodically
  setInterval(fetchRemoteLogStatus, 4000);

  // K8s 日志源是可选的：只在后端已配置时才点亮选项，不主动启动 kubectl
  fetchK8sStatus();
  setInterval(fetchK8sStatus, 8000);

  // Keyboard shortcuts
  document.addEventListener('keydown', (e) => {
    const mod = e.ctrlKey || e.metaKey;

    // Ctrl/Cmd+Enter 发送请求
    if (mod && e.key === 'Enter') {
      if (STATE.currentView === 'api') {
        e.preventDefault();
        sendAPIRequest();
      }
      return;
    }

    // Ctrl/Cmd+K 聚焦接口搜索
    if (mod && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      const box = document.getElementById('api-search-input');
      if (box) { box.focus(); box.select(); }
      return;
    }

    // Ctrl/Cmd+J 聚焦日志搜索
    if (mod && e.key.toLowerCase() === 'j') {
      e.preventDefault();
      const box = document.getElementById('log-search-input');
      if (box) { box.focus(); box.select(); }
      return;
    }

    // Ctrl/Cmd+S 把当前接口加入常用
    if (mod && e.key.toLowerCase() === 's') {
      e.preventDefault();
      toggleFavoriteCurrent();
      return;
    }

    // Esc 关闭弹窗
    if (e.key === 'Escape') {
      document.querySelectorAll('.modal-overlay')
        .forEach(m => { if (m.style.display === 'flex') m.style.display = 'none'; });
    }
  });

  // 接口搜索防抖（替代原地的 oninput 直接重绘）
  const searchBox = document.getElementById('api-search-input');
  if (searchBox) {
    searchBox.addEventListener('input', scheduleEndpointFilter);
  }

  // Track log container scrolling to pause auto-scroll if user scrolls up
  const logContainer = document.getElementById('log-stream-container');
  if (logContainer) {
    logContainer.addEventListener('scroll', () => {
      const atBottom = logContainer.scrollHeight - logContainer.scrollTop - logContainer.clientHeight < 30;
      if (!atBottom && STATE.autoScroll) {
        // user scrolled up
      }
    });
  }
});

// =========================================================================
// Theme Management (Dark / Light)
// =========================================================================
function initTheme() {
  const savedTheme = localStorage.getItem('dtp:theme') || 'dark';
  applyTheme(savedTheme);
}

function applyTheme(theme) {
  const btn = document.getElementById('btn-theme-toggle');
  if (theme === 'light') {
    document.documentElement.setAttribute('data-theme', 'light');
    if (btn) btn.innerHTML = '切换暗色';
  } else {
    document.documentElement.removeAttribute('data-theme');
    if (btn) btn.innerHTML = '切换亮色';
  }
}

function toggleTheme() {
  const currentTheme = document.documentElement.getAttribute('data-theme') === 'light' ? 'light' : 'dark';
  const newTheme = currentTheme === 'light' ? 'dark' : 'light';
  localStorage.setItem('dtp:theme', newTheme);
  applyTheme(newTheme);
  showToast(newTheme === 'light' ? '已切换至亮色模式' : '已切换至暗色模式');
}

// =========================================================================
// Configuration & Environments
// =========================================================================
async function loadConfig() {
  try {
    const res = await fetch('/api/config');
    const data = await res.json();
    if (data.current_base_url) {
      STATE.baseURL = data.current_base_url;
      document.getElementById('target-base-url').value = data.current_base_url;
      document.getElementById('req-url-base').textContent = data.current_base_url;
    }
    if (data.current_route_prefix !== undefined) {
      STATE.routePrefix = data.current_route_prefix;
      const prefixInput = document.getElementById('req-prefix-input');
      if (prefixInput) prefixInput.value = data.current_route_prefix;
    }
    if (data.current_env_id) {
      STATE.env = data.current_env_id;
      document.getElementById('env-select').value = data.current_env_id;
    }
    if (data.slow_sql_threshold) {
      STATE.auth.slowThreshold = data.slow_sql_threshold;
      document.getElementById('slow-threshold-input').value = data.slow_sql_threshold;
      document.getElementById('stat-threshold-val').textContent = data.slow_sql_threshold;
    }
    if (data.default_tenant_id) {
      STATE.auth.tenantId = data.default_tenant_id;
      const tenantInput = document.getElementById('auth-tenant-id-input');
      if (tenantInput) tenantInput.value = data.default_tenant_id;
    }
    if (data.default_user_id) {
      STATE.auth.userId = data.default_user_id;
    }
    if (data.default_token) {
      STATE.auth.token = data.default_token;
      document.getElementById('auth-token-input').value = data.default_token;
    }
    if (data.enable_xr_auth !== undefined) {
      STATE.auth.enableXrAuth = data.enable_xr_auth;
      document.getElementById('auth-enable-xr').checked = data.enable_xr_auth;
    }
    if (data.xr_auth_type) {
      STATE.auth.xrAuthType = data.xr_auth_type;
      document.getElementById('auth-xr-type-input').value = data.xr_auth_type;
    }
    if (data.xr_auth_uid) {
      STATE.auth.xrAuthUid = data.xr_auth_uid;
      document.getElementById('auth-xr-uid-input').value = data.xr_auth_uid;
    }
    if (data.xr_auth_sub) {
      STATE.auth.xrAuthSub = data.xr_auth_sub;
      document.getElementById('auth-xr-sub-input').value = data.xr_auth_sub;
    }
    if (data.xr_auth_tid) {
      STATE.auth.xrAuthTid = data.xr_auth_tid;
      document.getElementById('auth-xr-tid-input').value = data.xr_auth_tid;
    }
    if (data.xr_auth_aut) {
      STATE.auth.xrAuthAut = data.xr_auth_aut;
      document.getElementById('auth-xr-aut-input').value = data.xr_auth_aut;
    }
    if (data.environments) {
      STATE.environments = data.environments;
      populateEnvSelect(data.environments);
    }

    // 项目 profile：标题与默认请求头全部由后端下发
    if (data.project_name) {
      STATE.projectName = data.project_name;
    }
    if (data.project_title) {
      document.title = (data.project_name ? data.project_name + ' · ' : '') + data.project_title;
      const sub = document.querySelector('.app-subtitle');
      if (sub) sub.textContent = (data.project_name ? data.project_name + ' · ' : '') + '接口测试 / 日志 SQL 监控';
    }
    if (Array.isArray(data.profile_headers)) {
      STATE.auth.profileHeaders = data.profile_headers;
    }

    // 日志源可用性：选中源不可用/全部不可用时给出明确提示
    if (Array.isArray(data.sources)) {
      STATE.sourceStates = data.sources;
      updateSourceHint(data.sources, data.remote_log && data.remote_log.log_source);
    }

    if (data.remote_log) {
      STATE.remoteLog = Object.assign(STATE.remoteLog, data.remote_log);
      const sel = document.getElementById('log-source-select');
      if (sel && data.remote_log.log_source) {
        sel.value = data.remote_log.log_source;
      }
      populateRemoteLogModalInputs();
    }
    if (data.remote_log_status) {
      STATE.remoteLogStatus = data.remote_log_status;
      updateRemoteStatusBadge();
    }
  } catch (err) {
    console.error('Failed to load config:', err);
  }
}

function changeEnvironment() {
  const select = document.getElementById('env-select');
  const env = select.value;
  STATE.env = env;

  let url = '';
  let prefix = '';

  if (STATE.environments && STATE.environments.length > 0) {
    const found = STATE.environments.find(e => e.id === env) || STATE.environments[0];
    if (found) {
      url = found.base_url || '';
      prefix = found.route_prefix || '';
    }
  }
  if (env === 'custom' && STATE.customBaseURL) {
    url = STATE.customBaseURL;
  }

  STATE.baseURL = url;
  STATE.routePrefix = prefix;

  document.getElementById('target-base-url').value = url;
  document.getElementById('req-url-base').textContent = url;
  const prefixInput = document.getElementById('req-prefix-input');
  if (prefixInput) prefixInput.value = prefix;

  fetch('/api/config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ current_env_id: env, custom_base_url: url, current_route_prefix: prefix })
  }).then(() => {
    // auto 模式下日志来源跟随接口环境,环境变化后清空日志流避免新旧来源混杂
    if ((STATE.remoteLog.log_source || 'auto') === 'auto') {
      resetLogStream();
    }
  });
}

function onPrefixChange(val) {
  STATE.routePrefix = (val || '').trim();
  fetch('/api/config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ current_route_prefix: STATE.routePrefix })
  });
}

function updateBaseURL() {
  const input = document.getElementById('target-base-url');
  STATE.baseURL = input.value.trim();
  STATE.customBaseURL = input.value.trim();
  document.getElementById('req-url-base').textContent = STATE.baseURL;

  fetch('/api/config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ current_env_id: 'custom', custom_base_url: STATE.baseURL })
  });
}

// =========================================================================
// OpenAPI Loading & Navigation
// =========================================================================
async function loadOpenAPI() {
  try {
    const res = await fetch('/api/openapi');
    const data = await res.json();
    STATE.spec = data;
    STATE.endpoints = data.endpoints || [];
    STATE.groups = data.groups || [];

    document.getElementById('api-count-badge').textContent = data.total_count || 0;
    renderEndpointTree();

    // Auto-select first endpoint if available
    if (STATE.endpoints.length > 0 && !STATE.selectedEndpoint) {
      selectEndpoint(STATE.endpoints[0]);
    }
  } catch (err) {
    console.error('Failed to load OpenAPI:', err);
  }
}

async function reloadOpenAPI() {
  try {
    const res = await fetch('/api/openapi/reload', { method: 'POST' });
    const data = await res.json();
    if (data.error) {
      alert('重新加载失败: ' + data.error);
      return;
    }
    await loadOpenAPI();
    showToast('OpenAPI 刷新成功 (' + (data.total_count || 0) + ' 个接口)');
  } catch (err) {
    alert('请求失败: ' + err.message);
  }
}

function renderEndpointTree() {
  const container = document.getElementById('endpoint-tree-container');
  if (!container) return;

  const searchKeyword = (document.getElementById('api-search-input')?.value || '').toLowerCase().trim();
  const methodFilter = STATE.methodFilter;

  container.innerHTML = '';

  if (!STATE.groups || STATE.groups.length === 0) {
    container.innerHTML = '<div style="padding:20px; text-align:center; color:var(--text-muted);">暂无接口数据</div>';
    return;
  }

  const matchEndpoint = (ep) => {
    if (methodFilter !== 'ALL' && ep.method !== methodFilter) return false;
    if (!searchKeyword) return true;
    return (
      ep.path.toLowerCase().includes(searchKeyword) ||
      (ep.summary && ep.summary.toLowerCase().includes(searchKeyword)) ||
      (ep.description && ep.description.toLowerCase().includes(searchKeyword)) ||
      (ep.operationId && ep.operationId.toLowerCase().includes(searchKeyword))
    );
  };

  // 常用接口置顶：每天就那几个，不该埋在 140+ 条里找
  const favIds = new Set(STATE.favorites || []);
  const favEndpoints = [];
  STATE.groups.forEach(g => (g.endpoints || []).forEach(ep => {
    if (favIds.has(endpointId(ep)) && matchEndpoint(ep)) favEndpoints.push(ep);
  }));
  if (favEndpoints.length > 0) {
    renderGroup(container, { name: '★ 常用', endpoints: favEndpoints }, favEndpoints.length, true);
  }

  STATE.groups.forEach((group) => {
    const filteredEndpoints = (group.endpoints || []).filter(matchEndpoint);
    if (filteredEndpoints.length === 0) return;
    renderGroup(container, group, filteredEndpoints.length, false);
  });
}

function endpointId(ep) {
  return ep.method + ' ' + ep.path;
}

function renderGroup(container, group, count, pinned) {
  const groupEl = document.createElement('div');
  groupEl.className = 'tag-group';

  const headerEl = document.createElement('div');
  headerEl.className = 'tag-header';
  headerEl.innerHTML = `
    <span>${escapeHTML(group.name)}</span>
    <span class="badge" style="background:var(--bg-tertiary); color:var(--text-muted);">${count}</span>
  `;

  const listEl = document.createElement('div');
  listEl.className = 'tag-endpoints';

  headerEl.onclick = () => {
    listEl.style.display = listEl.style.display === 'none' ? 'flex' : 'none';
  };

  group.endpoints.forEach((ep) => {
    listEl.appendChild(buildEndpointItem(ep));
  });

  groupEl.appendChild(headerEl);
  groupEl.appendChild(listEl);
  container.appendChild(groupEl);
}

function buildEndpointItem(ep) {
  const itemEl = document.createElement('div');
  const isActive = STATE.selectedEndpoint
    && STATE.selectedEndpoint.path === ep.path
    && STATE.selectedEndpoint.method === ep.method;
  itemEl.className = 'endpoint-item' + (isActive ? ' active' : '');

  const starred = (STATE.favorites || []).includes(endpointId(ep));
  const methodBadgeClass = 'badge-' + ep.method.toLowerCase();

  itemEl.innerHTML = `
    <button class="fav-btn${starred ? ' on' : ''}" title="${starred ? '取消常用 (Ctrl+S)' : '加入常用 (Ctrl+S)'}">${starred ? '★' : '☆'}</button>
    <span class="badge ${methodBadgeClass}">${escapeHTML(ep.method)}</span>
    <div style="flex:1; overflow:hidden;">
      <div class="endpoint-path" title="${escapeHTML(ep.path)}">${escapeHTML(ep.path)}</div>
      <div class="endpoint-summary" title="${escapeHTML(ep.summary || ep.description || ep.operationId || '')}">${escapeHTML(ep.summary || ep.description || ep.operationId || '')}</div>
    </div>
  `;

  itemEl.querySelector('.fav-btn').onclick = (e) => {
    e.stopPropagation();
    toggleFavorite(ep);
  };

  itemEl.onclick = () => {
    document.querySelectorAll('.endpoint-item').forEach(el => el.classList.remove('active'));
    itemEl.classList.add('active');
    selectEndpoint(ep);
  };

  return itemEl;
}

// ── 常用接口 ──────────────────────────────────────────────
function loadFavorites() {
  try {
    const raw = localStorage.getItem(storageKey('favorites'));
    STATE.favorites = raw ? JSON.parse(raw) : [];
  } catch (e) { STATE.favorites = []; }
}

function saveFavorites() {
  try { localStorage.setItem(storageKey('favorites'), JSON.stringify(STATE.favorites)); } catch (e) {}
}

function toggleFavorite(ep) {
  const id = endpointId(ep);
  const i = (STATE.favorites || []).indexOf(id);
  if (i >= 0) {
    STATE.favorites.splice(i, 1);
    showToast('已取消常用');
  } else {
    STATE.favorites.push(id);
    showToast('已加入常用');
  }
  saveFavorites();
  renderEndpointTree();
}

function toggleFavoriteCurrent() {
  const ep = STATE.selectedEndpoint;
  if (!ep) { showToast('先选一个接口'); return; }
  toggleFavorite(ep);
}

function filterEndpoints() {
  renderEndpointTree();
}

// 搜索输入防抖：接口多时每敲一个字就全量重绘会卡
let endpointFilterTimer = null;
function scheduleEndpointFilter() {
  clearTimeout(endpointFilterTimer);
  endpointFilterTimer = setTimeout(renderEndpointTree, 120);
}

function filterMethod(method) {
  STATE.methodFilter = method;
  document.querySelectorAll('.method-filter-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.method === method);
  });
  renderEndpointTree();
}

// =========================================================================
// Select & Build Endpoint Request
// =========================================================================
function selectEndpoint(ep) {
  STATE.selectedEndpoint = ep;

  // Set method & path
  document.getElementById('req-method-select').value = ep.method;
  document.getElementById('req-path-input').value = ep.path;
  document.getElementById('active-op-id').textContent = ep.operationId || (ep.method + ' ' + ep.path);
  document.getElementById('active-op-desc').textContent = ep.summary || ep.description || '';

  // Reset params
  STATE.pathParams = {};
  STATE.queryParams = {};

  // Render Path Params
  const pathParamsArea = document.getElementById('path-params-area');
  const pathParamsBody = document.getElementById('path-params-body');
  pathParamsBody.innerHTML = '';

  if (ep.path_params && ep.path_params.length > 0) {
    pathParamsArea.style.display = 'block';
    ep.path_params.forEach(paramName => {
      STATE.pathParams[paramName] = '';
      const tr = document.createElement('tr');
      tr.innerHTML = `
        <td style="font-weight:600; color:var(--info);">${paramName}</td>
        <td style="color:var(--text-muted);">string (path)</td>
        <td>
          <input type="text" placeholder="输入 ${paramName}" oninput="onPathParamChange('${paramName}', this.value)">
        </td>
      `;
      pathParamsBody.appendChild(tr);
    });
  } else {
    pathParamsArea.style.display = 'none';
  }

  // Render Query Params
  const queryParamsBody = document.getElementById('query-params-body');
  queryParamsBody.innerHTML = '';

  if (ep.query_params && ep.query_params.length > 0) {
    ep.query_params.forEach(param => {
      STATE.queryParams[param.name] = param.schema && param.schema.default !== undefined ? String(param.schema.default) : '';
      const tr = document.createElement('tr');
      const requiredHtml = param.required ? '<span style="color:var(--danger); font-weight:700;">* 是</span>' : '<span style="color:var(--text-muted);">否</span>';
      const typeStr = param.schema ? (param.schema.type || 'string') : 'string';
      tr.innerHTML = `
        <td style="font-weight:600;">${param.name}</td>
        <td style="color:var(--text-muted);">${typeStr}</td>
        <td>${requiredHtml}</td>
        <td style="color:var(--text-muted); font-size:11px;">${param.description || '-'}</td>
        <td>
          <input type="text" value="${STATE.queryParams[param.name]}" placeholder="值" oninput="onQueryParamChange('${param.name}', this.value)">
        </td>
      `;
      queryParamsBody.appendChild(tr);
    });
  } else {
    queryParamsBody.innerHTML = '<tr><td colspan="5" style="color:var(--text-muted); text-align:center;">此接口无 Query 参数</td></tr>';
  }

  // Render Mock Body
  const bodyEditor = document.getElementById('req-body-editor');
  if (ep.mock_body_json) {
    bodyEditor.value = ep.mock_body_json;
  } else if (ep.method === 'GET' || ep.method === 'DELETE') {
    bodyEditor.value = '';
  } else {
    bodyEditor.value = '{\n\n}';
  }

  renderCustomHeadersTable();
  resetResponsePane();
}

function onPathParamChange(paramName, val) {
  STATE.pathParams[paramName] = val;
  updatePathInputWithParams();
}

function onQueryParamChange(paramName, val) {
  STATE.queryParams[paramName] = val;
}

function updatePathInputWithParams() {
  if (!STATE.selectedEndpoint) return;
  let p = STATE.selectedEndpoint.path;
  for (const [k, v] of Object.entries(STATE.pathParams)) {
    if (v) {
      p = p.replace(`{${k}}`, encodeURIComponent(v));
    }
  }
  document.getElementById('req-path-input').value = p;
}

function applyMockBody() {
  if (STATE.selectedEndpoint && STATE.selectedEndpoint.mock_body_json) {
    document.getElementById('req-body-editor').value = STATE.selectedEndpoint.mock_body_json;
    showToast('已自动填充 Schema Mock 数据');
  } else {
    showToast('该接口无请求体 Schema');
  }
}

function formatReqBody() {
  const editor = document.getElementById('req-body-editor');
  try {
    const parsed = JSON.parse(editor.value);
    editor.value = JSON.stringify(parsed, null, 2);
  } catch (err) {
    alert('JSON 格式无效: ' + err.message);
  }
}

function clearReqBody() {
  document.getElementById('req-body-editor').value = '';
}

// Request Tabs
function switchReqTab(tab) {
  document.querySelectorAll('.request-pane .tab-btn').forEach(btn => btn.classList.remove('active'));
  document.getElementById(`tab-btn-${tab}`).classList.add('active');

  document.getElementById('req-tab-body').style.display = tab === 'body' ? 'block' : 'none';
  document.getElementById('req-tab-params').style.display = tab === 'params' ? 'block' : 'none';
  document.getElementById('req-tab-headers').style.display = tab === 'headers' ? 'block' : 'none';
}

// Response Tabs
function switchRespTab(tab) {
  document.querySelectorAll('.response-pane .tab-btn').forEach(btn => btn.classList.remove('active'));
  document.getElementById(`tab-btn-resp-${tab}`).classList.add('active');

  document.getElementById('resp-tab-body').style.display = tab === 'body' ? 'block' : 'none';
  document.getElementById('resp-tab-headers').style.display = tab === 'headers' ? 'block' : 'none';
  document.getElementById('resp-tab-sql').style.display = tab === 'sql' ? 'block' : 'none';
  document.getElementById('resp-tab-biz').style.display = tab === 'biz' ? 'block' : 'none';
}

function resetResponsePane() {
  document.getElementById('response-status-bar').style.display = 'none';
  const bizBadge = document.getElementById('resp-badge-biz-code');
  if (bizBadge) bizBadge.style.display = 'none';
  const bizTabBtn = document.getElementById('tab-btn-resp-biz');
  if (bizTabBtn) bizTabBtn.style.display = 'none';
  const bizDetail = document.getElementById('resp-biz-detail');
  if (bizDetail) bizDetail.innerHTML = '';
  const scopePills = document.getElementById('resp-data-scope-pills');
  if (scopePills) scopePills.style.display = 'none';
  const copyDataBtn = document.getElementById('btn-copy-data');
  if (copyDataBtn) copyDataBtn.style.display = 'none';

  const prettyContainer = document.getElementById('response-pretty-container');
  if (prettyContainer) {
    prettyContainer.innerHTML = '<div class="empty-state">点击上方「发送请求」按钮测试接口</div>';
  }
  const bodyPre = document.getElementById('response-body-content');
  if (bodyPre) bodyPre.textContent = '';
  document.getElementById('resp-sql-count').textContent = '0';
  document.getElementById('resp-related-logs').innerHTML = '<div style="color:var(--text-muted); padding:10px; text-align:center;">发送请求后将通过 x-trace-id 自动关联并展示该请求触发的 SQL 与服务端日志</div>';
  STATE.lastResponseBody = '';
  STATE.lastResponseParsed = null;
  const searchInput = document.getElementById('resp-search-input');
  if (searchInput) searchInput.value = '';
  STATE.respSearchKeyword = '';
  const copyLogsBtn = document.getElementById('btn-copy-req-logs');
  if (copyLogsBtn) copyLogsBtn.style.display = 'none';
}

// Custom Headers
function renderCustomHeadersTable() {
  const tbody = document.getElementById('custom-headers-body');
  if (!tbody) return;
  tbody.innerHTML = '';

  // Tenant ID Header (required by dev/test gateway)
  if (STATE.auth.tenantId) {
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td style="color:var(--ok); font-weight:600;">tenant-id</td>
      <td style="font-family:monospace; font-weight:600;">${STATE.auth.tenantId}</td>
      <td><span class="badge" style="background:rgba(62,207,142,0.12); color:var(--ok);">网关租户</span></td>
    `;
    tbody.appendChild(tr);
  }

  // Profile 默认请求头（域名/租户/账号均由 --project 提供，不再写死）
  const profileHeaders = STATE.auth.profileHeaders || [];
  profileHeaders.forEach(h => {
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td style="color:var(--info); font-weight:600;">${escapeHTML(h.key)}</td>
      <td style="font-family:monospace;">${escapeHTML(h.value)}</td>
      <td><span class="badge" style="background:rgba(88,166,255,0.12); color:var(--info);">${escapeHTML(h.desc || '默认头')}</span></td>
    `;
    tbody.appendChild(tr);
  });

  // Auth Header row
  if (STATE.auth.token) {
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td style="color:var(--violet); font-weight:600;">Authorization</td>
      <td style="font-family:monospace; font-size:11px;">${STATE.auth.token.substring(0, 30)}...</td>
      <td><span class="badge" style="background:rgba(167,139,250,0.12); color:var(--violet);">全局认证</span></td>
    `;
    tbody.appendChild(tr);
  }

  STATE.customHeaders.forEach((h, idx) => {
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td><input type="text" value="${h.key}" placeholder="Header Name" oninput="STATE.customHeaders[${idx}].key = this.value"></td>
      <td><input type="text" value="${h.value}" placeholder="Header Value" oninput="STATE.customHeaders[${idx}].value = this.value"></td>
      <td><button class="btn btn-danger btn-sm" onclick="removeHeaderRow(${idx})">删除</button></td>
    `;
    tbody.appendChild(tr);
  });
}

function addHeaderRow() {
  STATE.customHeaders.push({ key: '', value: '', enabled: true });
  renderCustomHeadersTable();
}

function removeHeaderRow(idx) {
  STATE.customHeaders.splice(idx, 1);
  renderCustomHeadersTable();
}

// =========================================================================
// Execute API Test Request via Proxy
// =========================================================================
async function sendAPIRequest() {
  const method = document.getElementById('req-method-select').value;
  let path = document.getElementById('req-path-input').value.trim();
  const body = document.getElementById('req-body-editor').value;

  if (!path) {
    alert('请输入请求路径');
    return;
  }

  // Append Query parameters if any
  const queryPairs = [];
  for (const [k, v] of Object.entries(STATE.queryParams)) {
    if (v !== '' && v !== null && v !== undefined) {
      queryPairs.push(`${encodeURIComponent(k)}=${encodeURIComponent(v)}`);
    }
  }
  if (queryPairs.length > 0) {
    const sep = path.includes('?') ? '&' : '?';
    path += sep + queryPairs.join('&');
  }

  // Build target URL (combining BaseURL + RoutePrefix + Path)
  const prefix = (document.getElementById('req-prefix-input')?.value || STATE.routePrefix || '').trim();
  let targetURL = '';
  if (path.startsWith('http://') || path.startsWith('https://')) {
    targetURL = path;
  } else {
    let base = STATE.baseURL.replace(/\/+$/, '');
    let cleanPath = path.startsWith('/') ? path : '/' + path;

    if (prefix) {
      let normPrefix = prefix.startsWith('/') ? prefix : '/' + prefix;
      normPrefix = normPrefix.replace(/\/+$/, '');
      if (!base.endsWith(normPrefix) && !cleanPath.startsWith(normPrefix + '/') && cleanPath !== normPrefix) {
        base = base + normPrefix;
      }
    }
    targetURL = base + cleanPath;
  }

  // Build headers
  const headers = {};
  // 1. tenant-id header
  if (STATE.auth.tenantId) {
    headers['tenant-id'] = STATE.auth.tenantId;
  }
  // 2. Gateway x-xr-auth-*
  if (STATE.auth.enableXrAuth) {
    headers['x-xr-auth-type'] = STATE.auth.xrAuthType || 'jwt';
    const ph = STATE.auth.profileHeaders || [];
    ph.forEach(h => { if (h.key) headers[h.key] = h.value; });
    headers['x-xr-auth-aut'] = STATE.auth.xrAuthAut || 'SYS_ADMIN';
  }
  if (STATE.auth.token) {
    headers['Authorization'] = STATE.auth.token.startsWith('Bearer ') ? STATE.auth.token : 'Bearer ' + STATE.auth.token;
  }
  STATE.customHeaders.forEach(h => {
    if (h.key && h.value) {
      headers[h.key] = h.value;
    }
  });

  // Generate trace ID
  const traceID = generateTraceID();
  STATE.activeTraceID = traceID;
  headers['x-trace-id'] = traceID;

  // UI loading
  const sendBtn = document.getElementById('btn-send-request');
  const originalBtnText = sendBtn.innerHTML;
  sendBtn.innerHTML = '发送中…';
  sendBtn.disabled = true;

  const copyLogsBtn = document.getElementById('btn-copy-req-logs');
  if (copyLogsBtn) copyLogsBtn.style.display = 'none';

  try {
    const payload = {
      method,
      url: targetURL,
      headers,
      body,
      trace_id: traceID
    };

    const res = await fetch('/api/proxy', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const result = await res.json();

    displayResponse(result, method, path);

    // Save to history（存完整快照，这样「重发」能真的重现当时的请求）
    saveHistory({
      method,
      path,
      url: targetURL,
      baseURL: STATE.customBaseURL || STATE.baseURL,
      prefix: STATE.routePrefix,
      headers: (STATE.customHeaders || []).filter(h => h.enabled !== false),
      status: result.status,
      statusText: result.status_text,
      durationMs: result.duration_ms,
      time: new Date().toLocaleTimeString(),
      traceID: result.trace_id,
      body: body
    });

  } catch (err) {
    displayResponse({
      status: 0,
      status_text: 'Client Error',
      duration_ms: 0,
      size_bytes: 0,
      headers: {},
      body: err.message,
      trace_id: traceID
    }, method, path);
  } finally {
    sendBtn.innerHTML = originalBtnText;
    sendBtn.disabled = false;
  }
}

function displayResponse(res, method, path) {
  const statusBar = document.getElementById('response-status-bar');
  const statusBadge = document.getElementById('resp-badge-status');
  const timeBadge = document.getElementById('resp-badge-time');
  const sizeBadge = document.getElementById('resp-badge-size');
  const bodyPre = document.getElementById('response-body-content');

  statusBar.style.display = 'flex';
  statusBadge.textContent = `${res.status} ${res.status_text || ''}`;

  statusBadge.className = 'status-badge';
  if (res.status >= 200 && res.status < 300) {
    statusBadge.classList.add('status-2xx');
  } else if (res.status >= 400 && res.status < 500) {
    statusBadge.classList.add('status-4xx');
  } else {
    statusBadge.classList.add('status-5xx');
  }

  timeBadge.textContent = `${res.duration_ms ? res.duration_ms.toFixed(1) : 0} ms`;
  sizeBadge.textContent = formatBytes(res.size_bytes || 0);

  // Store response body
  STATE.lastResponseBody = res.body || '';
  let jsonParsed = null;
  try {
    jsonParsed = JSON.parse(res.body);
    STATE.lastResponseParsed = jsonParsed;
  } catch (e) {
    STATE.lastResponseParsed = null;
  }

  // Check business status code (dev / test standard: { "code": 0, "data": ... })
  const bizBadge = document.getElementById('resp-badge-biz-code');
  const bizTabBtn = document.getElementById('tab-btn-resp-biz');

  if (jsonParsed && typeof jsonParsed === 'object' && jsonParsed.code !== undefined) {
    // 状态栏仅显示短码，完整信息（含 message 等长文本）放入独立的业务状态 Tab
    if (bizBadge) {
      bizBadge.style.display = 'inline-flex';
      bizBadge.textContent = `业务状态: ${jsonParsed.code}`;
      if (jsonParsed.code === 0) {
        bizBadge.style.background = 'rgba(62, 207, 142, 0.14)';
        bizBadge.style.color = 'var(--ok)';
        bizBadge.style.borderColor = 'transparent';
      } else {
        bizBadge.style.background = 'rgba(240, 106, 106, 0.14)';
        bizBadge.style.color = 'var(--danger)';
        bizBadge.style.borderColor = 'transparent';
      }
    }
    if (bizTabBtn) {
      bizTabBtn.style.display = 'flex';
      if (jsonParsed.code !== 0) bizTabBtn.classList.add('biz-error-hint');
      else bizTabBtn.classList.remove('biz-error-hint');
    }
    renderBizStatusTab(jsonParsed);
    // 业务错误时自动切换到业务状态 Tab，方便第一时间看到完整错误信息
    if (jsonParsed.code !== 0) switchRespTab('biz');
  } else {
    if (bizBadge) bizBadge.style.display = 'none';
    if (bizTabBtn) {
      bizTabBtn.style.display = 'none';
      bizTabBtn.classList.remove('biz-error-hint');
    }
  }

  // Check if response contains 'data' field (dev/test standard format)
  const scopePills = document.getElementById('resp-data-scope-pills');
  const copyDataBtn = document.getElementById('btn-copy-data');
  const hasDataField = jsonParsed && typeof jsonParsed === 'object' && ('data' in jsonParsed);
  if (hasDataField) {
    if (scopePills) scopePills.style.display = 'inline-flex';
    if (copyDataBtn) copyDataBtn.style.display = 'inline-flex';
    STATE.dataScope = 'data';
    updateDataScopePillsUI();
  } else {
    if (scopePills) scopePills.style.display = 'none';
    if (copyDataBtn) copyDataBtn.style.display = 'none';
    STATE.dataScope = 'full';
  }

  // Render pretty JSON or raw
  renderCurrentResponseView();

  // Headers table
  const headersTable = document.getElementById('response-headers-table');
  headersTable.innerHTML = '';
  if (res.headers && Object.keys(res.headers).length > 0) {
    for (const [k, v] of Object.entries(res.headers)) {
      const tr = document.createElement('tr');
      tr.innerHTML = `<td style="font-weight:600; color:var(--text-secondary);">${k}</td><td style="font-family:monospace;">${v}</td>`;
      headersTable.appendChild(tr);
    }
  } else {
    headersTable.innerHTML = '<tr><td colspan="2" style="color:var(--text-muted); text-align:center;">无返回响应头</td></tr>';
  }

  // 请求完成后始终显示复制日志按钮（包括 4xx/5xx 错误响应，便于复制日志排查问题）
  const copyLogsBtn = document.getElementById('btn-copy-req-logs');
  if (copyLogsBtn) {
    copyLogsBtn.style.display = 'inline-flex';
  }

  // Correlate Related Logs & SQL queries
  updateRelatedLogsForTrace(res.trace_id);
}

// 渲染业务状态 Tab 的完整详情：业务码高亮 + 所有非 data 字段（message/reason 等长文本完整展示）
function renderBizStatusTab(jsonParsed) {
  const container = document.getElementById('resp-biz-detail');
  if (!container) return;

  const code = jsonParsed.code;
  const isSuccess = code === 0;
  const codeColor = isSuccess ? 'var(--ok)' : 'var(--danger)';
  const codeBg = isSuccess ? 'rgba(62, 207, 142, 0.14)' : 'rgba(240, 106, 106, 0.14)';
  const codeBorder = 'transparent';

  // 收集除 data 外的所有顶层字段，完整展示业务错误上下文
  const fields = [];
  for (const [k, v] of Object.entries(jsonParsed)) {
    if (k === 'data') continue;
    let display = v;
    if (v !== null && typeof v === 'object') {
      try { display = JSON.stringify(v, null, 2); } catch (e) { display = String(v); }
    } else if (v === null || v === undefined) {
      display = '';
    }
    fields.push({ key: k, value: String(display) });
  }

  const rowsHtml = fields.map(f => `
    <tr>
      <td style="font-weight:600; white-space:nowrap; vertical-align:top; ${f.key === 'code' ? 'color:' + codeColor + ';' : ''}">${escapeHTML(f.key)}</td>
      <td style="font-family:monospace; word-break:break-all; white-space:pre-wrap; vertical-align:top;">${escapeHTML(f.value)}</td>
    </tr>`).join('');

  container.innerHTML = `
    <div style="display:flex; align-items:center; gap:10px; margin-bottom:12px; flex-wrap:wrap;">
      <span class="status-badge" style="background:${codeBg}; color:${codeColor}; border:1px solid ${codeBorder}; font-size:14px; padding:4px 12px;">业务状态码: ${escapeHTML(String(code))} ${isSuccess ? '成功' : '失败'}</span>
      <button class="btn btn-secondary btn-sm" onclick="copyBizStatus()">复制业务状态详情</button>
    </div>
    <table class="param-table">
      <thead><tr><th style="width:120px;">字段</th><th>值</th></tr></thead>
      <tbody>${rowsHtml}</tbody>
    </table>
  `;
}

function copyBizStatus() {
  const parsed = STATE.lastResponseParsed;
  if (!parsed || typeof parsed !== 'object') return;
  const detail = {};
  for (const [k, v] of Object.entries(parsed)) {
    if (k !== 'data') detail[k] = v;
  }
  const text = JSON.stringify(detail, null, 2);
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(() => {
      showToast('业务状态详情已复制到剪贴板');
    }).catch(() => fallbackCopyText(text));
  } else {
    fallbackCopyText(text);
  }
}

function renderCurrentResponseView() {
  const jsonParsed = STATE.lastResponseParsed;
  const rawBody = STATE.lastResponseBody;

  if (STATE.dataScope === 'data' && jsonParsed && typeof jsonParsed === 'object' && ('data' in jsonParsed)) {
    renderJsonViewer(jsonParsed.data, JSON.stringify(jsonParsed.data, null, 2));
  } else {
    renderJsonViewer(jsonParsed, rawBody);
  }
}

function toggleDataScope(scope) {
  STATE.dataScope = scope;
  updateDataScopePillsUI();
  renderCurrentResponseView();
}

function updateDataScopePillsUI() {
  const pillData = document.getElementById('pill-scope-data');
  const pillFull = document.getElementById('pill-scope-full');
  if (pillData) pillData.classList.toggle('active', STATE.dataScope === 'data');
  if (pillFull) pillFull.classList.toggle('active', STATE.dataScope === 'full');
}

function copyDataPayload() {
  if (STATE.lastResponseParsed && typeof STATE.lastResponseParsed === 'object' && ('data' in STATE.lastResponseParsed)) {
    const formatted = JSON.stringify(STATE.lastResponseParsed.data, null, 2);
    navigator.clipboard.writeText(formatted).then(() => {
      showToast('data 核心业务数据已复制到剪贴板');
    });
  } else {
    copyResponseBody();
  }
}

function updateRelatedLogsForTrace(traceID) {
  if (!traceID) return;
  const relatedLogs = STATE.logs.filter(l => l.trace_id === traceID || (l.raw && l.raw.includes(traceID)));
  const countBadge = document.getElementById('resp-sql-count');
  countBadge.textContent = relatedLogs.length;

  const container = document.getElementById('resp-related-logs');
  if (relatedLogs.length === 0) {
    container.innerHTML = `
      <div style="color:var(--text-muted); padding:16px; text-align:center;">
        当前请求 trace_id: <code style="color:var(--violet);">${traceID}</code><br>
        暂未在应用日志中匹配到此 trace 的日志输出（若服务未开启 traceparent 传播或为本地模拟模式，可切换至全部日志流查看）
      </div>
    `;
    updateCopyLogsBtnCount();
    return;
  }

  container.innerHTML = '';
  relatedLogs.forEach(entry => {
    container.appendChild(createLogDOMNode(entry));
  });
  updateCopyLogsBtnCount();
}

function updateCopyLogsBtnCount() {
  const btn = document.getElementById('btn-copy-req-logs');
  if (!btn) return;
  const traceID = STATE.activeTraceID;
  if (!traceID) {
    btn.textContent = '复制本次请求日志';
    return;
  }
  const related = STATE.logs.filter(l => l.trace_id === traceID || (l.raw && l.raw.includes(traceID)));
  btn.textContent = related.length > 0 ? `复制本次请求日志 (${related.length})` : '复制本次请求日志';
}

function copyRequestLogs() {
  const traceID = STATE.activeTraceID;
  const logEntries = [];

  // 1. Gather all related logs for this request trace from STATE.logs
  if (traceID && Array.isArray(STATE.logs)) {
    STATE.logs.forEach(entry => {
      const isMatch = entry.trace_id === traceID || (entry.raw && entry.raw.includes(traceID));
      if (isMatch) {
        logEntries.push(entry);
      }
    });
  }

  // 2. Format each log entry (including SQL statements)
  let textLines = [];
  if (logEntries.length > 0) {
    textLines = logEntries.map(entry => {
      const timeStr = entry.time_str || (entry.timestamp ? new Date(entry.timestamp).toLocaleTimeString() : '');
      const timePrefix = timeStr ? `[${timeStr}] ` : '';
      const podPrefix = entry.pod_name ? `[${entry.pod_name}] ` : '';

      if (entry.is_sql) {
        const dur = entry.elapsed || (entry.duration_ms ? entry.duration_ms.toFixed(2) + 'ms' : '');
        const rows = entry.rows !== undefined ? `${entry.rows} rows` : '';
        const source = entry.source ? entry.source : '';
        const envTag = entry.log_source ? `[${entry.log_source.toUpperCase()}] ` : '';
        const metaParts = [dur, rows, source].filter(Boolean).join(' | ');
        const metaStr = metaParts ? ` [${metaParts}]` : '';
        const sqlText = (entry.sql || entry.message || entry.raw || '').trim();
        return `${timePrefix}${envTag}${podPrefix}[SQL]${metaStr}\n${sqlText}${sqlText.endsWith(';') ? '' : ';'}`;
      } else {
        const level = entry.level || 'INFO';
        const source = entry.source ? `[${entry.source}] ` : '';
        const envTag = entry.log_source ? `[${entry.log_source.toUpperCase()}] ` : '';
        const msg = entry.message || entry.raw || '';
        return `${timePrefix}${envTag}${podPrefix}[${level}] ${source}${msg}`;
      }
    });
  } else {
    // Fallback: extract text from DOM if STATE.logs did not capture entries
    const container = document.getElementById('resp-related-logs');
    if (container) {
      const domEntries = container.querySelectorAll('.log-entry');
      domEntries.forEach(el => {
        const text = el.innerText.trim();
        if (text) textLines.push(text);
      });
    }
  }

  if (textLines.length === 0) {
    showToast('本次请求暂未捕获到关联的日志与 SQL');
    return;
  }

  const fullText = textLines.join('\n\n');
  const onCopySuccess = () => {
    showToast(`已复制本次请求的 ${textLines.length} 条日志(含SQL)到剪贴板`);
  };

  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(fullText).then(onCopySuccess).catch(() => {
      fallbackCopyText(fullText, onCopySuccess);
    });
  } else {
    fallbackCopyText(fullText, onCopySuccess);
  }
}

function fallbackCopyText(text, callback) {
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.style.position = 'fixed';
  ta.style.left = '-9999px';
  ta.style.top = '-9999px';
  ta.style.opacity = '0';
  document.body.appendChild(ta);
  ta.select();
  try {
    document.execCommand('copy');
    if (callback) callback();
    else showToast('已复制到剪贴板');
  } catch (err) {
    showToast('复制失败，请手动复制');
  }
  document.body.removeChild(ta);
}

function copyCurlCommand() {
  const method = document.getElementById('req-method-select').value;
  let path = document.getElementById('req-path-input').value.trim();
  const body = document.getElementById('req-body-editor').value;
  const prefix = (document.getElementById('req-prefix-input')?.value || STATE.routePrefix || '').trim();

  let targetURL = '';
  if (path.startsWith('http://') || path.startsWith('https://')) {
    targetURL = path;
  } else {
    let base = STATE.baseURL.replace(/\/+$/, '');
    let cleanPath = path.startsWith('/') ? path : '/' + path;
    if (prefix) {
      let normPrefix = prefix.startsWith('/') ? prefix : '/' + prefix;
      normPrefix = normPrefix.replace(/\/+$/, '');
      if (!base.endsWith(normPrefix) && !cleanPath.startsWith(normPrefix + '/') && cleanPath !== normPrefix) {
        base = base + normPrefix;
      }
    }
    targetURL = base + cleanPath;
  }

  let curl = `curl -X ${method} "${targetURL}" \\\n`;
  if (STATE.auth.tenantId) {
    curl += `  -H "tenant-id: ${STATE.auth.tenantId}" \\\n`;
  }
  if (STATE.auth.enableXrAuth) {
    curl += `  -H "x-xr-auth-type: ${STATE.auth.xrAuthType || 'jwt'}" \\\n`;
    (STATE.auth.profileHeaders || []).forEach(h => {
      if (h.key) curl += `  -H "${h.key}: ${h.value}" \\\n`;
    });
    curl += `  -H "x-xr-auth-aut: ${STATE.auth.xrAuthAut || 'SYS_ADMIN'}" \\\n`;
  }
  if (STATE.auth.token) {
    curl += `  -H "Authorization: Bearer ${STATE.auth.token.replace(/^Bearer\s+/, '')}" \\\n`;
  }
  STATE.customHeaders.forEach(h => {
    if (h.key && h.value) {
      curl += `  -H "${h.key}: ${h.value}" \\\n`;
    }
  });

  if (body && method !== 'GET' && method !== 'HEAD') {
    curl += `  -d '${body.replace(/'/g, "'\\''")}'`;
  } else {
    curl = curl.replace(/ \\\n$/, '');
  }

  navigator.clipboard.writeText(curl).then(() => {
    showToast('cURL 命令已复制到剪贴板');
  });
}

function renderJsonViewer(parsed, raw) {
  const prettyContainer = document.getElementById('response-pretty-container');
  const rawPre = document.getElementById('response-body-content');

  let rawFormatted = raw || '';
  if (parsed !== null && parsed !== undefined) {
    try {
      rawFormatted = JSON.stringify(parsed, null, 2);
    } catch (e) {}
  }
  if (rawPre) rawPre.textContent = rawFormatted || '(Empty response body)';

  if (!prettyContainer) return;

  if (parsed === null || parsed === undefined) {
    if (!raw) {
      prettyContainer.innerHTML = '<div style="color:var(--text-muted); text-align:center; padding:30px;">(Empty response body)</div>';
    } else {
      prettyContainer.innerHTML = `<pre style="margin:0; padding:6px; font-family:monospace; color:var(--text-primary); white-space:pre-wrap;">${escapeHTML(raw)}</pre>`;
    }
    return;
  }

  prettyContainer.innerHTML = '';
  const rootNode = buildJsonNode(parsed, '', true, 0);
  prettyContainer.appendChild(rootNode);
}

function buildJsonNode(val, key, isLast, depth) {
  const container = document.createElement('div');
  container.className = 'json-node';

  const row = document.createElement('div');
  row.className = 'json-row';

  // Indentation
  if (depth > 0) {
    const indent = document.createElement('span');
    indent.className = 'json-indent';
    indent.style.width = `${depth * 16}px`;
    row.appendChild(indent);
  }

  const isObject = val !== null && typeof val === 'object';
  const isArray = Array.isArray(val);

  if (isObject) {
    const keys = Object.keys(val);
    const count = keys.length;
    const openChar = isArray ? '[' : '{';
    const closeChar = isArray ? ']' : '}';

    if (count === 0) {
      const caret = document.createElement('span');
      caret.className = 'json-caret hidden';
      caret.innerHTML = '▶';
      row.appendChild(caret);

      if (key !== '') {
        const keySpan = document.createElement('span');
        keySpan.className = 'json-key';
        keySpan.innerHTML = `"${highlightSearchText(escapeHTML(key))}"`;
        row.appendChild(keySpan);

        const colon = document.createElement('span');
        colon.className = 'json-colon';
        colon.textContent = ': ';
        row.appendChild(colon);
      }

      const bracketSpan = document.createElement('span');
      bracketSpan.className = 'json-bracket';
      bracketSpan.textContent = openChar + closeChar;
      row.appendChild(bracketSpan);

      if (!isLast) {
        const comma = document.createElement('span');
        comma.className = 'json-comma';
        comma.textContent = ',';
        row.appendChild(comma);
      }

      container.appendChild(row);
      return container;
    }

    // Collapsible object/array
    const caret = document.createElement('span');
    caret.className = 'json-caret';
    caret.innerHTML = '▼';
    row.appendChild(caret);

    if (key !== '') {
      const keySpan = document.createElement('span');
      keySpan.className = 'json-key';
      keySpan.innerHTML = `"${highlightSearchText(escapeHTML(key))}"`;
      row.appendChild(keySpan);

      const colon = document.createElement('span');
      colon.className = 'json-colon';
      colon.textContent = ': ';
      row.appendChild(colon);
    }

    const openSpan = document.createElement('span');
    openSpan.className = 'json-bracket';
    openSpan.textContent = openChar;
    row.appendChild(openSpan);

    const preview = document.createElement('span');
    preview.className = 'json-collapsed-preview';
    preview.style.display = 'none';
    preview.textContent = isArray ? `... Array(${count})` : `... {${count} keys}`;
    row.appendChild(preview);

    container.appendChild(row);

    // Children wrapper
    const childrenWrapper = document.createElement('div');
    childrenWrapper.className = 'json-collapsible';

    keys.forEach((childKey, idx) => {
      const childVal = val[childKey];
      const childIsLast = idx === count - 1;
      const childNode = buildJsonNode(childVal, isArray ? '' : childKey, childIsLast, depth + 1);
      childrenWrapper.appendChild(childNode);
    });

    // Closing row
    const closingRow = document.createElement('div');
    closingRow.className = 'json-row';
    if (depth > 0) {
      const closeIndent = document.createElement('span');
      closeIndent.className = 'json-indent';
      closeIndent.style.width = `${depth * 16}px`;
      closingRow.appendChild(closeIndent);
    }
    const hiddenCaret = document.createElement('span');
    hiddenCaret.className = 'json-caret hidden';
    closingRow.appendChild(hiddenCaret);

    const closeSpan = document.createElement('span');
    closeSpan.className = 'json-bracket';
    closeSpan.textContent = closeChar;
    closingRow.appendChild(closeSpan);

    if (!isLast) {
      const comma = document.createElement('span');
      comma.className = 'json-comma';
      comma.textContent = ',';
      closingRow.appendChild(comma);
    }
    childrenWrapper.appendChild(closingRow);
    container.appendChild(childrenWrapper);

    const toggleCollapse = () => {
      const isCollapsed = caret.classList.contains('collapsed');
      if (isCollapsed) {
        caret.classList.remove('collapsed');
        childrenWrapper.style.display = 'block';
        preview.style.display = 'none';
      } else {
        caret.classList.add('collapsed');
        childrenWrapper.style.display = 'none';
        preview.style.display = 'inline-block';
      }
    };

    caret.onclick = toggleCollapse;
    preview.onclick = toggleCollapse;

    return container;
  }

  // Primitive value
  const hiddenCaret = document.createElement('span');
  hiddenCaret.className = 'json-caret hidden';
  row.appendChild(hiddenCaret);

  if (key !== '') {
    const keySpan = document.createElement('span');
    keySpan.className = 'json-key';
    keySpan.innerHTML = `"${highlightSearchText(escapeHTML(key))}"`;
    row.appendChild(keySpan);

    const colon = document.createElement('span');
    colon.className = 'json-colon';
    colon.textContent = ': ';
    row.appendChild(colon);
  }

  const valSpan = document.createElement('span');
  if (typeof val === 'string') {
    valSpan.className = 'json-string';
    valSpan.innerHTML = `"${highlightSearchText(escapeHTML(val))}"`;
  } else if (typeof val === 'number') {
    valSpan.className = 'json-number';
    valSpan.innerHTML = highlightSearchText(String(val));
  } else if (typeof val === 'boolean') {
    valSpan.className = 'json-boolean';
    valSpan.innerHTML = highlightSearchText(String(val));
  } else if (val === null) {
    valSpan.className = 'json-null';
    valSpan.innerHTML = highlightSearchText('null');
  } else {
    valSpan.textContent = String(val);
  }
  row.appendChild(valSpan);

  if (!isLast) {
    const comma = document.createElement('span');
    comma.className = 'json-comma';
    comma.textContent = ',';
    row.appendChild(comma);
  }

  container.appendChild(row);
  return container;
}

function highlightSearchText(text) {
  if (!STATE.respSearchKeyword) return text;
  const kw = STATE.respSearchKeyword.replace(/[-/\\^$*+?.()|[\]{}]/g, '\\$&');
  const regex = new RegExp(`(${kw})`, 'gi');
  return text.replace(regex, '<span class="json-search-highlight">$1</span>');
}

function toggleRespFormat(fmt) {
  STATE.respFormat = fmt;
  const prettyBtn = document.getElementById('pill-resp-pretty');
  const rawBtn = document.getElementById('pill-resp-raw');
  if (prettyBtn) prettyBtn.classList.toggle('active', fmt === 'pretty');
  if (rawBtn) rawBtn.classList.toggle('active', fmt === 'raw');

  const prettyContainer = document.getElementById('response-pretty-container');
  const rawPre = document.getElementById('response-body-content');
  const collapseBtn = document.getElementById('btn-collapse-all');
  const expandBtn = document.getElementById('btn-expand-all');

  if (fmt === 'pretty') {
    if (prettyContainer) prettyContainer.style.display = 'block';
    if (rawPre) rawPre.style.display = 'none';
    if (collapseBtn) collapseBtn.style.display = 'inline-flex';
    if (expandBtn) expandBtn.style.display = 'inline-flex';
  } else {
    if (prettyContainer) prettyContainer.style.display = 'none';
    if (rawPre) rawPre.style.display = 'block';
    if (collapseBtn) collapseBtn.style.display = 'none';
    if (expandBtn) expandBtn.style.display = 'none';
  }
}

function collapseAllJson() {
  document.querySelectorAll('#response-pretty-container .json-collapsible').forEach(el => {
    el.style.display = 'none';
  });
  document.querySelectorAll('#response-pretty-container .json-collapsed-preview').forEach(el => {
    el.style.display = 'inline-block';
  });
  document.querySelectorAll('#response-pretty-container .json-caret:not(.hidden)').forEach(el => {
    el.classList.add('collapsed');
  });
}

function expandAllJson() {
  document.querySelectorAll('#response-pretty-container .json-collapsible').forEach(el => {
    el.style.display = 'block';
  });
  document.querySelectorAll('#response-pretty-container .json-collapsed-preview').forEach(el => {
    el.style.display = 'none';
  });
  document.querySelectorAll('#response-pretty-container .json-caret:not(.hidden)').forEach(el => {
    el.classList.remove('collapsed');
  });
}

function onRespSearchChange(kw) {
  STATE.respSearchKeyword = (kw || '').trim();
  if (STATE.lastResponseParsed !== null && STATE.lastResponseParsed !== undefined) {
    renderJsonViewer(STATE.lastResponseParsed, STATE.lastResponseBody);
  }
}

function copyResponseBody() {
  let text = STATE.lastResponseBody;
  if (STATE.lastResponseParsed !== null && STATE.lastResponseParsed !== undefined) {
    try {
      text = JSON.stringify(STATE.lastResponseParsed, null, 2);
    } catch (e) {}
  }
  navigator.clipboard.writeText(text).then(() => {
    showToast('响应内容已复制');
  });
}

// localStorage key 按项目隔离：同一个工具接不同项目时历史不会混在一起
function storageKey(name) {
  const p = (STATE.projectName || 'default').replace(/[^A-Za-z0-9_.-]/g, '_');
  return `dtp:${p}:${name}`;
}

// =========================================================================
// Real-Time Log & SQL Monitoring (WebSocket)
// =========================================================================
function initWebSocket() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsURL = `${protocol}//${window.location.host}/ws/logs`;

  const indicator = document.getElementById('ws-indicator');
  let retry = STATE.wsRetry || 0;

  try {
    STATE.ws = new WebSocket(wsURL);

    STATE.ws.onopen = () => {
      STATE.wsRetry = 0;
      indicator.className = 'ws-badge ok';
      indicator.textContent = '日志流已连接';
    };

    STATE.ws.onmessage = (event) => {
      try {
        const entry = JSON.parse(event.data);
        handleIncomingLog(entry);
      } catch (e) {
        console.error('Failed to parse WS log:', e);
      }
    };

    // 指数退避：后端重启时 3s 固定重连会刷屏，且没意义
    STATE.ws.onclose = () => {
      retry = Math.min(retry + 1, 6);
      STATE.wsRetry = retry;
      const delay = Math.min(1000 * Math.pow(1.6, retry - 1), 15000);
      indicator.className = 'ws-badge error';
      indicator.textContent = `日志流断开，${Math.round(delay / 1000)}s 后重连`;
      clearTimeout(STATE.wsRetryTimer);
      STATE.wsRetryTimer = setTimeout(initWebSocket, delay);
    };

    STATE.ws.onerror = () => {
      STATE.ws.close();
    };
  } catch (err) {
    console.error('WebSocket init failed:', err);
  }
}

// 日志缓存上限（与后端 Hub 缓存同步）与 DOM 渲染保护上限
const MAX_LOG_CACHE = 10000;
const MAX_RENDER_LOGS = 2000;

async function loadRecentLogs() {
  try {
    const res = await fetch(`/api/logs/recent?limit=${MAX_LOG_CACHE}`);
    const data = await res.json();
    if (data.logs && Array.isArray(data.logs)) {
      data.logs.forEach(entry => {
        STATE.logs.push(entry);
      });
      renderLogStream();
    }
    if (data.stats) {
      updateSQLStatsUI(data.stats);
    }
  } catch (err) {
    console.error('Failed to load recent logs:', err);
  }
}

function handleIncomingLog(entry) {
  STATE.logs.push(entry);
  if (STATE.logs.length > MAX_LOG_CACHE) {
    STATE.logs.splice(0, STATE.logs.length - MAX_LOG_CACHE);
  }

  // Update stats if SQL
  if (entry.is_sql) {
    STATE.sqlStats.total_queries++;
    if (entry.is_slow_sql) {
      STATE.sqlStats.slow_queries++;
    }
    if (entry.duration_ms > STATE.sqlStats.max_latency_ms) {
      STATE.sqlStats.max_latency_ms = entry.duration_ms;
    }
    updateSQLStatsUI(STATE.sqlStats);
  }

  // If current active trace matches, update related logs
  if (STATE.activeTraceID && (entry.trace_id === STATE.activeTraceID || (entry.raw && entry.raw.includes(STATE.activeTraceID)))) {
    const container = document.getElementById('resp-related-logs');
    if (container) {
      container.appendChild(createLogDOMNode(entry));
      const badge = document.getElementById('resp-sql-count');
      if (badge) badge.textContent = parseInt(badge.textContent || 0) + 1;
      updateCopyLogsBtnCount();
    }
  }

  // Check filter
  if (!matchesLogFilter(entry)) {
    return;
  }

  const container = document.getElementById('log-stream-container');
  if (container) {
    const node = createLogDOMNode(entry);
    container.appendChild(node);

    // 渲染保护：DOM 节点超过上限时移除最旧的日志节点
    while (container.childElementCount > MAX_RENDER_LOGS) {
      const first = container.firstElementChild;
      if (!first || !first.classList.contains('log-entry')) break;
      container.removeChild(first);
    }

    if (STATE.autoScroll) {
      container.scrollTop = container.scrollHeight;
    }
  }

  document.getElementById('stat-log-count').textContent = STATE.logs.length;
}

function matchesLogFilter(entry) {
  const p = STATE.logFilters.preset;
  if (p === 'sql' && !entry.is_sql) return false;
  if (p === 'slow' && !entry.is_slow_sql) return false;
  if (p === 'error' && entry.level !== 'ERROR' && entry.level !== 'FATAL' && !entry.error) return false;

  if (STATE.logFilters.level && entry.level !== STATE.logFilters.level) {
    return false;
  }

  if (STATE.logFilters.search) {
    const q = STATE.logFilters.search.toLowerCase();
    const match = (entry.message && entry.message.toLowerCase().includes(q)) ||
                  (entry.sql && entry.sql.toLowerCase().includes(q)) ||
                  (entry.source && entry.source.toLowerCase().includes(q)) ||
                  (entry.trace_id && entry.trace_id.toLowerCase().includes(q)) ||
                  (entry.pod_name && entry.pod_name.toLowerCase().includes(q)) ||
                  (entry.namespace && entry.namespace.toLowerCase().includes(q)) ||
                  (entry.container_name && entry.container_name.toLowerCase().includes(q)) ||
                  (entry.log_source && entry.log_source.toLowerCase().includes(q)) ||
                  (entry.raw && entry.raw.toLowerCase().includes(q));
    if (!match) return false;
  }

  return true;
}

function createLogDOMNode(entry) {
  const div = document.createElement('div');
  div.className = 'log-entry' + 
    (entry.level === 'ERROR' ? ' level-error' : '') +
    (entry.is_slow_sql ? ' is-slow-sql' : '');

  const timeStr = entry.time_str || (entry.timestamp ? new Date(entry.timestamp).toLocaleTimeString() : '');
  const envBadge = entry.log_source ? `<span class="log-env-badge log-env-${entry.log_source}">${entry.log_source.toUpperCase()}</span>` : '';
  const podBadge = entry.pod_name ? `<span class="log-pod-badge" onclick="filterByPod('${escapeQuote(entry.pod_name)}')" title="Pod: ${entry.pod_name}\nNamespace: ${entry.namespace || ''}\nContainer: ${entry.container_name || ''}">${escapeHTML(entry.pod_name)}</span>` : '';

  if (entry.is_sql) {
    // Render rich SQL card
    const isSlow = entry.is_slow_sql;
    const durClass = isSlow ? 'sql-duration slow' : 'sql-duration';
    const slowPill = isSlow ? `<span class="slow-sql-pill">慢 SQL ${entry.elapsed || entry.duration_ms + 'ms'}</span>` : '';
    const traceHtml = entry.trace_id ? `<span class="log-trace" onclick="filterByTrace('${entry.trace_id}')" title="点击过滤此 trace 关联日志">trace:${entry.trace_id.substring(0, 12)}...</span>` : '';

    div.innerHTML = `
      <div class="log-time">${timeStr}</div>
      <div class="sql-card">
        <div class="sql-header">
          <span class="sql-badge-tag">SQL</span>
          ${envBadge}
          ${podBadge}
          ${slowPill}
          <span class="${durClass}">${entry.elapsed || (entry.duration_ms ? entry.duration_ms.toFixed(2) + 'ms' : '')}</span>
          <span class="sql-rows">${entry.rows !== undefined ? entry.rows + ' 行' : ''}</span>
          <span class="log-source" title="${entry.source}">${entry.source || 'caller'}</span>
          ${traceHtml}
          <button class="btn btn-secondary btn-sm" style="margin-left:auto; padding:1px 6px; font-size:10px;" onclick="copyText('${escapeQuote(entry.sql)}')">复制 SQL</button>
        </div>
        <div class="sql-text">${highlightSQLKeywords(entry.sql || entry.message)}</div>
      </div>
    `;
  } else {
    // Render standard log line
    const traceHtml = entry.trace_id ? `<span class="log-trace" onclick="filterByTrace('${entry.trace_id}')">trace:${entry.trace_id.substring(0, 10)}...</span>` : '';
    const sourceHtml = entry.source ? `<span class="log-source">${entry.source}</span>` : '';

    div.innerHTML = `
      <div class="log-time">${timeStr}</div>
      <div class="log-level-badge lvl-${entry.level}">${entry.level}</div>
      ${envBadge}
      ${podBadge}
      ${sourceHtml}
      ${traceHtml}
      <div class="log-content">${escapeHTML(entry.message || entry.raw)}</div>
    `;
  }

  return div;
}

function renderLogStream() {
  const container = document.getElementById('log-stream-container');
  if (!container) return;

  container.innerHTML = '';
  const filtered = STATE.logs.filter(matchesLogFilter);

  // 渲染保护：最多渲染最近 MAX_RENDER_LOGS 条，超出部分顶部提示
  let renderList = filtered;
  if (filtered.length > MAX_RENDER_LOGS) {
    const omitted = filtered.length - MAX_RENDER_LOGS;
    renderList = filtered.slice(-MAX_RENDER_LOGS);
    const hint = document.createElement('div');
    hint.className = 'log-render-hint';
    hint.textContent = `结果过多：已隐藏较早的 ${omitted} 条，仅渲染最近 ${MAX_RENDER_LOGS} 条，请用筛选或搜索缩小范围`;
    container.appendChild(hint);
  }

  const frag = document.createDocumentFragment();
  renderList.forEach(entry => {
    frag.appendChild(createLogDOMNode(entry));
  });
  container.appendChild(frag);

  if (STATE.autoScroll) {
    container.scrollTop = container.scrollHeight;
  }

  document.getElementById('stat-log-count').textContent = STATE.logs.length;
}

function setLogPreset(preset) {
  STATE.logFilters.preset = preset;
  document.querySelectorAll('.log-filters .filter-chip').forEach(el => {
    if (el.id.startsWith('chip-') && !el.id.startsWith('chip-lvl-')) {
      el.classList.toggle('active', el.id === `chip-${preset}`);
    }
  });
  renderLogStream();
}

function toggleLevel(level) {
  if (STATE.logFilters.level === level) {
    STATE.logFilters.level = '';
    document.getElementById(`chip-lvl-${level.toLowerCase()}`).classList.remove('active');
  } else {
    STATE.logFilters.level = level;
    ['debug', 'info', 'warn', 'error'].forEach(l => {
      document.getElementById(`chip-lvl-${l}`).classList.toggle('active', l === level.toLowerCase());
    });
  }
  renderLogStream();
}

function onLogSearchChange() {
  STATE.logFilters.search = document.getElementById('log-search-input').value.trim();
  renderLogStream();
}

function filterByTrace(traceID) {
  document.getElementById('log-search-input').value = traceID;
  STATE.logFilters.search = traceID;
  renderLogStream();
  showToast('已过滤 Trace: ' + traceID);
}

function toggleAutoScroll() {
  STATE.autoScroll = !STATE.autoScroll;
  const btn = document.getElementById('btn-toggle-scroll');
  btn.innerHTML = STATE.autoScroll ? '自动滚动: 开启' : '自动滚动: 暂停';
  if (STATE.autoScroll) {
    const c = document.getElementById('log-stream-container');
    if (c) c.scrollTop = c.scrollHeight;
  }
}

function clearLogs() {
  STATE.logs = [];
  renderLogStream();
  fetch('/api/logs/clear', { method: 'POST' });
}

async function generateMockLogs() {
  showToast('正在生成模拟日志与慢 SQL...');
  try {
    await fetch('/api/logs/mock?count=15', { method: 'POST' });
  } catch (err) {
    console.error('Mock logs request failed:', err);
  }
}

function updateSQLStatsUI(stats) {
  if (!stats) return;
  document.getElementById('stat-total-sql').textContent = stats.total_queries || 0;
  document.getElementById('stat-slow-sql').textContent = stats.slow_queries || 0;
  document.getElementById('stat-avg-latency').textContent = (stats.avg_latency_ms ? stats.avg_latency_ms.toFixed(2) : '0.00') + ' ms';
  document.getElementById('stat-max-latency').textContent = (stats.max_latency_ms ? stats.max_latency_ms.toFixed(2) : '0.00') + ' ms';
}

// =========================================================================
// Views Switcher: API Tester, Live Logs, Split View
// =========================================================================
function switchView(view) {
  if (view !== 'api' && view !== 'logs') view = 'api';
  STATE.currentView = view;

  document.querySelectorAll('.mode-pill').forEach(pill => pill.classList.remove('active'));
  const pill = document.getElementById(`pill-${view}`);
  if (pill) pill.classList.add('active');

  document.getElementById('api-view').style.display = view === 'api' ? 'flex' : 'none';
  document.getElementById('log-view').style.display = view === 'logs' ? 'flex' : 'none';
}

// =========================================================================
// Modals & Helpers
// =========================================================================
function openAuthModal() {
  document.getElementById('auth-token-input').value = STATE.auth.token || '';
  document.getElementById('slow-threshold-input').value = STATE.auth.slowThreshold || '200ms';
  document.getElementById('auth-enable-xr').checked = STATE.auth.enableXrAuth !== false;
  document.getElementById('auth-xr-type-input').value = STATE.auth.xrAuthType || 'jwt';
  document.getElementById('auth-xr-uid-input').value = STATE.auth.xrAuthUid || '';
  document.getElementById('auth-xr-sub-input').value = STATE.auth.xrAuthSub || '';
  document.getElementById('auth-xr-tid-input').value = STATE.auth.xrAuthTid || '';
  document.getElementById('auth-xr-aut-input').value = STATE.auth.xrAuthAut || 'SYS_ADMIN';
  const tenantInput = document.getElementById('auth-tenant-id-input');
  if (tenantInput) tenantInput.value = STATE.auth.tenantId || '';
  document.getElementById('modal-auth').style.display = 'flex';
}
function closeAuthModal() {
  document.getElementById('modal-auth').style.display = 'none';
}
function saveAuthModal() {
  STATE.auth.token = document.getElementById('auth-token-input').value.trim();
  STATE.auth.slowThreshold = document.getElementById('slow-threshold-input').value.trim();
  STATE.auth.enableXrAuth = document.getElementById('auth-enable-xr').checked;
  STATE.auth.xrAuthType = document.getElementById('auth-xr-type-input').value.trim();
  STATE.auth.xrAuthUid = document.getElementById('auth-xr-uid-input').value.trim();
  STATE.auth.xrAuthSub = document.getElementById('auth-xr-sub-input').value.trim();
  STATE.auth.xrAuthTid = document.getElementById('auth-xr-tid-input').value.trim();
  STATE.auth.xrAuthAut = document.getElementById('auth-xr-aut-input').value.trim();
  const tenantInput = document.getElementById('auth-tenant-id-input');
  if (tenantInput) STATE.auth.tenantId = tenantInput.value.trim() || '';

  document.getElementById('stat-threshold-val').textContent = STATE.auth.slowThreshold;

  fetch('/api/config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      default_token: STATE.auth.token,
      default_tenant_id: STATE.auth.tenantId,
      slow_sql_threshold: STATE.auth.slowThreshold,
      enable_xr_auth: STATE.auth.enableXrAuth,
      xr_auth_type: STATE.auth.xrAuthType,
      xr_auth_uid: STATE.auth.xrAuthUid,
      xr_auth_sub: STATE.auth.xrAuthSub,
      xr_auth_tid: STATE.auth.xrAuthTid,
      xr_auth_aut: STATE.auth.xrAuthAut
    })
  });

  renderCustomHeadersTable();
  closeAuthModal();
  showToast('全局认证与网关鉴权请求头已更新');
}

async function openSlowSQLModal() {
  const container = document.getElementById('slow-sql-list-container');
  container.innerHTML = '<div style="text-align:center; padding:20px; color:var(--text-muted);">加载中...</div>';
  document.getElementById('modal-slow-sql').style.display = 'flex';
  switchSlowTab('window');
}

// 保留窗口内的慢 SQL（stats.slowest_sqls）与全生命周期累计聚合（/api/logs/aggregates）
// 是两个口径：前者会被 2000 条保留淘汰掉，后者不会。
let slowTab = 'window';
let aggrCache = [];

async function switchSlowTab(tab) {
  slowTab = tab;
  document.getElementById('slow-tab-window').className = 'btn btn-sm' + (tab === 'window' ? '' : ' btn-secondary');
  document.getElementById('slow-tab-aggr').className = 'btn btn-sm' + (tab === 'aggr' ? '' : ' btn-secondary');
  document.getElementById('aggr-filter').style.display = tab === 'aggr' ? '' : 'none';

  const container = document.getElementById('slow-sql-list-container');
  if (tab === 'window') {
    await renderWindowSlowSQL(container);
  } else {
    await loadAggregates(container);
  }
}

async function loadAggregates(container) {
  container.innerHTML = '<div style="text-align:center; padding:20px; color:var(--text-muted);">加载中...</div>';
  try {
    const res = await fetch('/api/logs/aggregates?sort=hotspot&limit=200');
    const data = await res.json();
    aggrCache = data.groups || [];
    document.getElementById('aggr-count').textContent = data.total || 0;
    renderAggregates();
  } catch (err) {
    container.innerHTML = `<div style="color:var(--danger); padding:20px;">加载失败: ${escapeHTML(err.message)}</div>`;
  }
}

function renderAggregates() {
  const container = document.getElementById('slow-sql-list-container');
  const q = (document.getElementById('aggr-filter').value || '').trim().toLowerCase();
  const list = q
    ? aggrCache.filter(g => (g.fingerprint || '').toLowerCase().includes(q)
        || (g.source || '').toLowerCase().includes(q)
        || (g.sql || '').toLowerCase().includes(q)
        || (g.reason || '').toLowerCase().includes(q))
    : aggrCache;

  if (list.length === 0) {
    container.innerHTML = `<div class="empty-state">${q ? '没有匹配的模板' : '暂无累计数据'}</div>`;
    return;
  }

  container.innerHTML = '';
  list.forEach((g, idx) => {
    const card = document.createElement('div');
    card.style.cssText = 'background:var(--bg-primary); border:1px solid var(--border-color); border-radius:6px; padding:10px 14px; margin-bottom:8px;';

    const slow = g.slow || 0;
    const errs = g.errors || 0;
    const badge = (n, color, label) => n > 0
      ? `<span style="font-size:11px; padding:1px 6px; border-radius:10px; background:${color}22; color:${color}; font-weight:600;">${label} ${n}</span>`
      : '';
    const n1 = g.n1 ? `<span style="font-size:11px; padding:1px 6px; border-radius:10px; background:#8b5cf622; color:#a78bfa; font-weight:600;">N+1 ×${g.n1_max || 0}</span>` : '';

    card.innerHTML = `
      <div style="display:flex; justify-content:space-between; align-items:center; gap:8px; margin-bottom:6px; flex-wrap:wrap;">
        <span style="font-weight:700; color:var(--text-primary);">#${idx + 1} × ${g.count || 0} 次</span>
        ${badge(slow, 'var(--warning, #e0a63c)', '慢')}
        ${badge(errs, 'var(--danger)', '错误')}
        ${n1}
        <span style="font-size:11px; color:var(--text-muted); margin-left:auto;">最大 ${g.max_ms != null ? g.max_ms.toFixed(1) + 'ms' : '未知'} · 均 ${g.avg_ms != null ? g.avg_ms.toFixed(1) + 'ms' : '未知'}${g.reason ? ' · ' + escapeHTML(g.reason) : ''}</span>
      </div>
      <div style="font-size:11px; color:var(--violet); margin-bottom:5px; font-family:var(--font-mono); word-break:break-all;">${escapeHTML(g.source || '（无代码位置）')}</div>
      <div style="background:var(--bg-code); padding:8px; border-radius:var(--radius-s); font-family:var(--font-mono); font-size:11px; color:var(--cyan); word-break:break-all;">${escapeHTML(g.sql || g.fingerprint || '')}</div>
    `;
    container.appendChild(card);
  });
}

async function renderWindowSlowSQL(container) {
  try {
    const res = await fetch('/api/logs/stats');
    const stats = await res.json();
    const list = stats.slowest_sqls || [];

    if (list.length === 0) {
      container.innerHTML = '<div class="empty-state">暂无慢 SQL 记录</div>';
      return;
    }

    container.innerHTML = '';
    list.forEach((item, idx) => {
      const card = document.createElement('div');
      card.style.background = 'var(--bg-primary)';
      card.style.border = '1px solid var(--border-color)';
      card.style.borderRadius = '6px';
      card.style.padding = '10px 14px';
      card.style.marginBottom = '8px';

      card.innerHTML = `
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px;">
          <span style="font-weight:700; color:var(--danger);">#${idx + 1} 慢 SQL (${item.elapsed || item.duration_ms + 'ms'})</span>
          <span class="log-env-badge log-env-${item.log_source || 'local'}">${(item.log_source || 'local').toUpperCase()}</span>
          ${item.pod_name ? `<span class="log-pod-badge">${escapeHTML(item.pod_name)}</span>` : ''}
          <span style="font-size:11px; color:var(--text-muted);">${item.time_str || ''}</span>
        </div>
        <div style="font-size:11px; color:var(--violet); margin-bottom:6px;">${item.source || ''}</div>
        <div style="background:var(--bg-code); padding:8px; border-radius:var(--radius-s); font-family:var(--font-mono); font-size:11px; color:var(--cyan); word-break:break-all;">
          ${item.sql}
        </div>
      `;
      container.appendChild(card);
    });
  } catch (err) {
    container.innerHTML = `<div style="color:var(--danger); padding:20px;">加载失败: ${err.message}</div>`;
  }
}
function closeSlowSQLModal() {
  document.getElementById('modal-slow-sql').style.display = 'none';
}

function openHistoryModal() {
  renderHistoryList();
  document.getElementById('modal-history').style.display = 'flex';
}
function closeHistoryModal() {
  document.getElementById('modal-history').style.display = 'none';
}

function saveHistory(item) {
  STATE.history.unshift(item);
  if (STATE.history.length > 50) STATE.history.pop();
  try {
    localStorage.setItem(storageKey('history'), JSON.stringify(STATE.history));
  } catch (e) {}
}

function loadLocalHistory() {
  try {
    const raw = localStorage.getItem(storageKey('history'));
    if (raw) STATE.history = JSON.parse(raw);
  } catch (e) {}
}

function clearHistory() {
  STATE.history = [];
  try { localStorage.removeItem(storageKey('history')); } catch(e) {}
  renderHistoryList();
}

function renderHistoryList() {
  const container = document.getElementById('history-list-container');
  if (!STATE.history || STATE.history.length === 0) {
    container.innerHTML = '<div style="text-align:center; padding:30px; color:var(--text-muted);">暂无测试历史记录</div>';
    return;
  }

  container.innerHTML = '';
  STATE.history.forEach((h, idx) => {
    const div = document.createElement('div');
    div.style.background = 'var(--bg-primary)';
    div.style.border = '1px solid var(--border-color)';
    div.style.borderRadius = '6px';
    div.style.padding = '8px 12px';
    div.style.marginBottom = '6px';
    div.style.display = 'flex';
    div.style.alignItems = 'center';
    div.style.justifyContent = 'space-between';
    div.style.gap = '10px';

    div.innerHTML = `
      <div style="display:flex; align-items:center; gap:8px; flex:1; overflow:hidden; cursor:pointer;">
        <span class="badge badge-${escapeHTML(String(h.method).toLowerCase())}">${escapeHTML(h.method)}</span>
        <span style="font-family:monospace; font-size:12px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${escapeHTML(h.path)}</span>
        <span class="status-badge status-${String(h.status)[0]}xx" style="font-size:10px;">${escapeHTML(String(h.status))}</span>
        <span style="font-size:11px; color:var(--text-muted);">${h.durationMs ? h.durationMs.toFixed(1) + 'ms' : ''}</span>
      </div>
      <div style="display:flex; align-items:center; gap:8px; flex-shrink:0;">
        <button class="btn btn-secondary btn-sm" data-act="replay" title="填充到请求面板">重发</button>
        <span style="font-size:11px; color:var(--text-muted);">${escapeHTML(h.time || '')}</span>
      </div>
    `;

    // 点左侧填充（不发送）
    div.querySelector('div').onclick = () => {
      applyHistory(h);
      closeHistoryModal();
    };

    // 点「重发」填充并直接发送
    div.querySelector('[data-act=replay]').onclick = (e) => {
      e.stopPropagation();
      applyHistory(h);
      closeHistoryModal();
      sendAPIRequest();
    };

    container.appendChild(div);
  });
}

// 把一条历史恢复成完整请求；旧格式只有 method/path/body 也能用
function applyHistory(h) {
  document.getElementById('req-method-select').value = h.method;
  document.getElementById('req-path-input').value = h.path;
  if (h.body !== undefined) {
    document.getElementById('req-body-editor').value = h.body || '';
  }
  if (h.baseURL) {
    const el = document.getElementById('target-base-url');
    if (el) el.value = h.baseURL;
    STATE.customBaseURL = h.baseURL;
  }
  if (h.prefix !== undefined) {
    const el = document.getElementById('req-prefix-input');
    if (el) el.value = h.prefix;
    STATE.routePrefix = h.prefix;
  }
  if (Array.isArray(h.headers) && h.headers.length) {
    STATE.customHeaders = h.headers.map(x => ({ ...x }));
    renderCustomHeadersTable();
  }
}

// SQL Syntax Highlighting
function highlightSQLKeywords(sql) {
  if (!sql) return '';
  const keywords = [
    'SELECT', 'FROM', 'WHERE', 'JOIN', 'LEFT JOIN', 'RIGHT JOIN', 'INNER JOIN',
    'GROUP BY', 'ORDER BY', 'LIMIT', 'OFFSET', 'INSERT INTO', 'VALUES',
    'UPDATE', 'SET', 'DELETE', 'AND', 'OR', 'IN', 'ON', 'AS', 'COUNT', 'AVG', 'SUM'
  ];
  let escaped = escapeHTML(sql);
  keywords.forEach(kw => {
    const regex = new RegExp(`\\b${kw}\\b`, 'gi');
    escaped = escaped.replace(regex, `<span class="sql-kw">${kw}</span>`);
  });
  return escaped;
}

function generateTraceID() {
  const chars = '0123456789abcdef';
  let result = '';
  for (let i = 0; i < 32; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
}

function formatBytes(bytes) {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

function escapeHTML(str) {
  if (!str) return '';
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function escapeQuote(str) {
  if (!str) return '';
  return str.replace(/'/g, "\\'").replace(/\n/g, ' ');
}

function copyText(str) {
  navigator.clipboard.writeText(str).then(() => {
    showToast('已复制到剪贴板');
  });
}

function showToast(msg) {
  let toast = document.getElementById('app-toast');
  if (!toast) {
    toast = document.createElement('div');
    toast.id = 'app-toast';
    toast.className = 'app-toast';
    document.body.appendChild(toast);
  }
  toast.textContent = msg;
  toast.style.opacity = '1';
  clearTimeout(toast._timer);
  toast._timer = setTimeout(() => {
    toast.style.opacity = '0';
  }, 2500);
}

// =========================================================================
// Remote Cloud Log Management (Workflow API)
// =========================================================================
function populateRemoteLogModalInputs() {
  const rl = STATE.remoteLog || {};
  const tokenInput = document.getElementById('remote-token-input');
  if (tokenInput && rl.auth_token !== undefined) tokenInput.value = rl.auth_token;

  const urlInput = document.getElementById('remote-workflow-url-input');
  if (urlInput) urlInput.value = rl.workflow_url || '';

  const serviceInput = document.getElementById('remote-service-input');
  if (serviceInput) serviceInput.value = rl.service || '';

  const sourceInput = document.getElementById('remote-source-input');
  if (sourceInput) sourceInput.value = rl.source || 'default';

  const queryModeInput = document.getElementById('remote-query-mode-input');
  if (queryModeInput) queryModeInput.value = rl.query_mode || 'keyword';

  const timeRangeInput = document.getElementById('remote-time-range-input');
  if (timeRangeInput) timeRangeInput.value = rl.time_range || 'realtime';

  const intervalInput = document.getElementById('remote-poll-interval-input');
  if (intervalInput) intervalInput.value = rl.poll_interval_ms || 2000;

  const pageSizeInput = document.getElementById('remote-page-size-input');
  if (pageSizeInput) pageSizeInput.value = rl.page_size || 100;

  const devEnv = (rl.environments && rl.environments.dev) || {};
  const devEnvInput = document.getElementById('remote-dev-environment-input');
  if (devEnvInput) devEnvInput.value = devEnv.environment || devEnv.namespace || '';

  const testEnv = (rl.environments && rl.environments.test) || {};
  const testEnvInput = document.getElementById('remote-test-environment-input');
  if (testEnvInput) testEnvInput.value = testEnv.environment || testEnv.namespace || '';

  const prodEnv = (rl.environments && rl.environments.prod) || {};
  const prodEnvInput = document.getElementById('remote-prod-environment-input');
  if (prodEnvInput) prodEnvInput.value = prodEnv.environment || prodEnv.namespace || '';
}

function openRemoteLogModal() {
  populateRemoteLogModalInputs();
  const modal = document.getElementById('modal-remote-logs');
  if (modal) modal.style.display = 'flex';
  const resultBox = document.getElementById('remote-test-result-box');
  if (resultBox) resultBox.style.display = 'none';
}

function closeRemoteLogModal() {
  const modal = document.getElementById('modal-remote-logs');
  if (modal) modal.style.display = 'none';
}

function toggleRemoteTokenVisibility() {
  const input = document.getElementById('remote-token-input');
  if (input) {
    input.type = input.type === 'password' ? 'text' : 'password';
  }
}

function updateRemoteStatusBadge() {
  const badge = document.getElementById('remote-log-status-badge');
  if (!badge) return;

  const status = STATE.remoteLogStatus;
  if (!status) {
    badge.className = 'remote-status-badge remote-status-active';
    badge.textContent = '本地文件';
    badge.title = '本地日志文件监控';
    return;
  }

  const active = status.active_source || 'local';
  if (active === 'local') {
    badge.className = 'remote-status-badge remote-status-active';
    badge.textContent = '本地文件';
    badge.title = '本地日志监控: ' + (STATE.auth.logFilePath || 'app.log');
  } else {
    const envUpper = active.toUpperCase();
    if (!status.has_token) {
      badge.className = 'remote-status-badge remote-status-warn';
      badge.textContent = `远端 ${envUpper}: 未配Token`;
      badge.title = '点击配置远端云日志 Authorization Token';
    } else if (status.last_error) {
      badge.className = 'remote-status-badge remote-status-error';
      badge.textContent = `远端 ${envUpper}: 异常`;
      badge.title = status.last_error;
    } else {
      badge.className = 'remote-status-badge remote-status-active';
      badge.textContent = `远端 ${envUpper} 监听中 (${status.logs_fetched || 0})`;
      badge.title = `服务: ${status.service || ''}\n环境: ${status.environment || status.namespace}\n已拉取: ${status.logs_fetched || 0} 条日志`;
    }
  }

  updateRemoteSearchBarVisibility();
}

// 历史搜索栏仅在日志来源为远端云日志 (dev/test/prod) 时显示
function updateRemoteSearchBarVisibility() {
  const bar = document.getElementById('remote-search-bar');
  if (!bar) return;

  const status = STATE.remoteLogStatus;
  const active = status ? (status.active_source || 'local') : (STATE.remoteLog.log_source || 'local');
  const isRemote = ['dev', 'test', 'prod'].includes(active);
  bar.style.display = isRemote ? 'flex' : 'none';
}

// 切换自定义时间范围输入框显隐
function onRemoteSearchRangeChange() {
  const val = document.getElementById('remote-search-time-range')?.value;
  const isCustom = val === 'custom';
  const start = document.getElementById('remote-search-start');
  const end = document.getElementById('remote-search-end');
  const sep = document.getElementById('remote-search-range-sep');
  if (start) start.style.display = isCustom ? '' : 'none';
  if (sep) sep.style.display = isCustom ? '' : 'none';
  if (end) end.style.display = isCustom ? '' : 'none';
  if (isCustom) {
    // 默认填充最近 1 小时，避免用户忘填搜出全量
    if (start && !start.value) {
      const d = new Date(Date.now() - 3600 * 1000);
      start.value = toLocalDatetimeValue(d);
    }
    if (end && !end.value) {
      end.value = toLocalDatetimeValue(new Date());
    }
  }
}

function toLocalDatetimeValue(d) {
  const pad = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// 自定义范围校验：开始必须早于结束
function validateRemoteSearchRange() {
  const start = document.getElementById('remote-search-start');
  const end = document.getElementById('remote-search-end');
  if (!start || !end) return true;
  if (start.value && end.value) {
    const ok = new Date(start.value).getTime() < new Date(end.value).getTime();
    start.style.borderColor = ok ? '' : 'var(--danger)';
    end.style.borderColor = ok ? '' : 'var(--danger)';
    return ok;
  }
  return true;
}

// 一次性拉取指定时间窗口的历史日志：后端清空缓存并分页拉满，
// 结果经 WebSocket 推送追加到已清空的本地日志流；实时轮询随后继续 tail。
async function executeRemoteSearch() {
  if (STATE.remoteSearching) return;

  const rangeVal = document.getElementById('remote-search-time-range')?.value || '15m';
  const queryMode = document.getElementById('remote-search-mode')?.value || 'keyword';
  const keyword = document.getElementById('remote-search-input')?.value.trim() || '';
  const btn = document.getElementById('btn-remote-search');

  const reqBody = {
    time_range: rangeVal,
    query_mode: queryMode,
    keyword: keyword,
    max_results: MAX_LOG_CACHE
  };

  if (rangeVal === 'custom') {
    const startVal = document.getElementById('remote-search-start')?.value;
    const endVal = document.getElementById('remote-search-end')?.value;
    if (!startVal || !endVal) {
      showToast('请填写完整的自定义时间范围 (开始与结束时间)');
      return;
    }
    const startSec = Math.floor(new Date(startVal).getTime() / 1000);
    const endSec = Math.floor(new Date(endVal).getTime() / 1000);
    if (startSec >= endSec) {
      showToast('自定义时间范围无效: 开始时间必须早于结束时间');
      return;
    }
    reqBody.start_time = startSec;
    reqBody.end_time = endSec;
  }

  // 先清空本地日志流与统计，后端推送的历史日志经 WS 逐条追加
  STATE.logs = [];
  STATE.sqlStats = {
    total_queries: 0,
    slow_queries: 0,
    fast_queries: 0,
    avg_latency_ms: 0,
    max_latency_ms: 0,
    slowest_sqls: []
  };
  updateSQLStatsUI(STATE.sqlStats);
  renderLogStream();

  STATE.remoteSearching = true;
  if (btn) {
    btn.disabled = true;
    btn.textContent = '搜索中...';
  }

  try {
    const res = await fetch('/api/logs/remote/search', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(reqBody)
    });
    const data = await res.json();

    // 等待后端 WS 推送队列排空后统一渲染
    await new Promise(resolve => setTimeout(resolve, 300));

    // 按时间正序兜底排序，确保历史结果有序展示
    STATE.logs.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));
    renderLogStream();

    if (data.success) {
      const extra = data.truncated ? '（已达拉取上限，结果被截断）' : '';
      showToast(`${data.message}${extra}，耗时 ${data.duration_ms}ms`);
    } else {
      showToast('历史搜索失败: ' + (data.message || '未知错误'));
    }

    // SQL 统计面板拉取后端最新统计（已覆盖整个搜索窗口）
    try {
      const statsRes = await fetch('/api/logs/stats');
      if (statsRes.ok) {
        updateSQLStatsUI(await statsRes.json());
      }
    } catch (e) {
      // ignore stats refresh error
    }
  } catch (err) {
    showToast('历史搜索请求失败: ' + err.message);
  } finally {
    STATE.remoteSearching = false;
    if (btn) {
      btn.disabled = false;
      btn.textContent = '搜索';
    }
  }
}

async function fetchRemoteLogStatus() {
  try {
    const res = await fetch('/api/logs/remote/status');
    if (res.ok) {
      const data = await res.json();
      STATE.remoteLogStatus = data;
      updateRemoteStatusBadge();
    }
  } catch (e) {
    // ignore background status fetch error
  }
}

async function changeLogSource() {
  const sel = document.getElementById('log-source-select');
  if (!sel) return;
  const source = sel.value;

  if (source === 'k8s') {
    await switchToK8s();
    return;
  }

  // 离开 K8s 源时先把 kubectl 收掉
  if (STATE.remoteLog.log_source === 'k8s') {
    await fetch('/api/logs/k8s/stop', { method: 'POST' });
  }

  STATE.remoteLog.log_source = source;

  try {
    await fetch('/api/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        remote_log: { log_source: source }
      })
    });
    showToast(`日志来源已切换: ${sel.options[sel.selectedIndex].text}`);
    await fetchRemoteLogStatus();
    resetLogStream();
  } catch (err) {
    showToast('切换日志来源失败: ' + err.message);
  }
}

// ── K8s 日志源（可选加载，未配置时不会碰 kubectl）──────────────
let k8sStatus = { enabled: false, running: false, state: 'idle', environments: [] };

async function fetchK8sStatus() {
  try {
    const res = await fetch('/api/logs/k8s/status');
    k8sStatus = await res.json();
    const opt = document.getElementById('log-source-k8s-option');
    if (opt) {
      if (!k8sStatus.enabled) {
        opt.disabled = true;
        opt.textContent = 'K8s Pod (未配置)';
      } else {
        opt.disabled = false;
        const n = (k8sStatus.environments || []).length;
        opt.textContent = n ? `K8s Pod (${n} 个环境)` : 'K8s Pod (全部)';
      }
    }
    if (k8sStatus.running) {
      const badge = document.getElementById('remote-log-status-badge');
      if (badge) {
        badge.textContent = 'K8s ' + (k8sStatus.describe || '');
        badge.className = 'remote-status-badge remote-status-active';
      }
    }
    if (k8sStatus.state === 'error' && k8sStatus.error) {
      showToast('K8s 日志源: ' + k8sStatus.error, true);
    }
  } catch (e) {
    /* 后端未就绪时忽略 */
  }
}

async function switchToK8s() {
  const sel = document.getElementById('log-source-select');
  if (!k8sStatus.enabled) {
    showToast('K8s 日志源未启用，请在配置里设 k8s.enabled=true', true);
    sel.value = STATE.remoteLog.log_source || 'local';
    return;
  }

  const envs = k8sStatus.environments || [];
  let environment = '';
  if (envs.length > 1) {
    const names = envs.map(e => e.name).join(' / ');
    const pick = prompt(`选择 K8s 环境（${names}）\n留空 = 全部 pod`, envs[0].name);
    if (pick === null) { sel.value = STATE.remoteLog.log_source || 'local'; return; }
    environment = pick.trim();
  } else if (envs.length === 1) {
    environment = envs[0].name;
  }

  try {
    const res = await fetch('/api/logs/k8s/start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ environment })
    });
    const j = await res.json();
    if (!res.ok || !j.ok) {
      showToast(j.error || '启动失败', true);
      sel.value = STATE.remoteLog.log_source || 'local';
      return;
    }
    STATE.remoteLog.log_source = 'k8s';
    showToast('已连接到 K8s' + (environment ? ' · ' + environment : ''));
    resetLogStream();
    setTimeout(fetchK8sStatus, 1200);
  } catch (err) {
    showToast('启动 K8s 日志源失败: ' + err.message, true);
    sel.value = STATE.remoteLog.log_source || 'local';
  }
}

// 日志来源切换后清空本地日志流与 SQL 统计,并重新拉取当前来源的日志
function resetLogStream() {
  STATE.logs = [];
  STATE.sqlStats = {
    total_queries: 0,
    slow_queries: 0,
    fast_queries: 0,
    avg_latency_ms: 0,
    max_latency_ms: 0,
    slowest_sqls: []
  };
  updateSQLStatsUI(STATE.sqlStats);
  loadRecentLogs();
}

function filterByPod(podName) {
  if (!podName) return;
  const input = document.getElementById('log-search-input');
  if (input) {
    input.value = podName;
    STATE.logFilters.search = podName;
    renderLogStream();
    showToast(`已按 Pod 过滤: ${podName}`);
  }
}

async function testRemoteLogConnection() {
  const btn = document.getElementById('btn-test-remote-log');
  const resultBox = document.getElementById('remote-test-result-box');
  if (!resultBox) return;

  const token = document.getElementById('remote-token-input').value.trim();
  const workflowUrl = document.getElementById('remote-workflow-url-input').value.trim();
  const service = (document.getElementById('remote-service-input')?.value || STATE.remoteLog.service || '').trim();
  const source = (document.getElementById('remote-source-input')?.value || 'default').trim();
  const queryMode = (document.getElementById('remote-query-mode-input')?.value || 'keyword').trim();
  const timeRange = (document.getElementById('remote-time-range-input')?.value || 'realtime').trim();

  // Test the environment target selected in log source (fallback: dev)
  const activeSource = ['dev', 'test', 'prod'].includes(STATE.remoteLog && STATE.remoteLog.log_source)
    ? STATE.remoteLog.log_source : 'dev';
  let environment = '';

  if (activeSource === 'test') {
    environment = (document.getElementById('remote-test-environment-input')?.value || '').trim();
  } else if (activeSource === 'prod') {
    environment = (document.getElementById('remote-prod-environment-input')?.value || '').trim();
  } else {
    environment = (document.getElementById('remote-dev-environment-input')?.value || '').trim();
  }

  if (!token) {
    resultBox.className = 'test-result-box error';
    resultBox.style.display = 'flex';
    resultBox.innerHTML = '<strong>测试失败:</strong> 请先输入 Authorization Token';
    return;
  }

  btn.disabled = true;
  btn.textContent = '测试连接中…';
  resultBox.style.display = 'none';

  try {
    const res = await fetch('/api/logs/remote/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        workflow_url: workflowUrl,
        auth_token: token,
        service: service,
        source: source,
        environment: environment,
        query_mode: queryMode,
        time_range: timeRange
      })
    });
    const data = await res.json();

    resultBox.style.display = 'flex';
    if (data.success) {
      resultBox.className = 'test-result-box success';
      let sampleHtml = '';
      if (data.sample_entry) {
        const pod = data.sample_entry.instance || data.sample_entry.pod_name || '';
        const msg = data.sample_entry.message || '';
        sampleHtml = `<div style="font-family:var(--font-mono); font-size:11px; background:var(--bg-code); padding:6px 8px; border-radius:var(--radius-s); margin-top:4px;">
          <div>Pod/Instance: ${escapeHTML(pod)}</div>
          <div style="margin-top:2px; color:var(--text-muted);">${escapeHTML(msg.substring(0, 150))}${msg.length > 150 ? '...' : ''}</div>
        </div>`;
      }
      resultBox.innerHTML = `<div><strong>${escapeHTML(data.message)}</strong> (耗时: ${data.duration_ms}ms)</div>${sampleHtml}`;
    } else {
      resultBox.className = 'test-result-box error';
      resultBox.innerHTML = `<strong>测试失败:</strong> ${escapeHTML(data.message || data.error || '未知错误')}`;
    }
  } catch (err) {
    resultBox.style.display = 'flex';
    resultBox.className = 'test-result-box error';
    resultBox.innerHTML = `<strong>网络错误:</strong> ${escapeHTML(err.message)}`;
  } finally {
    btn.disabled = false;
    btn.textContent = '立即测试连接与拉取';
  }
}

async function saveRemoteLogConfig() {
  const token = document.getElementById('remote-token-input').value.trim();
  const workflowUrl = document.getElementById('remote-workflow-url-input').value.trim();
  const service = (document.getElementById('remote-service-input')?.value || STATE.remoteLog.service || '').trim();
  const source = (document.getElementById('remote-source-input')?.value || 'default').trim();
  const queryMode = (document.getElementById('remote-query-mode-input')?.value || 'keyword').trim();
  const timeRange = (document.getElementById('remote-time-range-input')?.value || 'realtime').trim();
  const pollInterval = parseInt(document.getElementById('remote-poll-interval-input').value) || 2000;
  const pageSize = parseInt(document.getElementById('remote-page-size-input').value) || 100;

  const devEnv = (document.getElementById('remote-dev-environment-input')?.value || '').trim();
  const testEnv = (document.getElementById('remote-test-environment-input')?.value || '').trim();
  const prodEnv = (document.getElementById('remote-prod-environment-input')?.value || '').trim();

  const sel = document.getElementById('log-source-select');
  const logSource = sel ? sel.value : (STATE.remoteLog.log_source || 'auto');

  const payload = {
    remote_log: {
      workflow_url: workflowUrl,
      auth_token: token,
      service: service,
      source: source,
      query_mode: queryMode,
      time_range: timeRange,
      poll_interval_ms: pollInterval,
      page_size: pageSize,
      realtime: true,
      log_source: logSource,
      environments: {
        dev: {
          environment: devEnv,
          namespace: devEnv,
          service: service,
          source: source
        },
        test: {
          environment: testEnv,
          namespace: testEnv,
          service: service,
          source: source
        },
        prod: {
          environment: prodEnv,
          namespace: prodEnv,
          service: service,
          source: source
        }
      }
    }
  };

  try {
    const res = await fetch('/api/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    if (res.ok) {
      STATE.remoteLog = Object.assign(STATE.remoteLog, payload.remote_log);
      closeRemoteLogModal();
      showToast('远端云日志配置已保存并生效');
      await fetchRemoteLogStatus();
    } else {
      showToast('保存配置失败');
    }
  } catch (err) {
    showToast('保存配置失败: ' + err.message);
  }
}

// 环境下拉由 profile 驱动，不再写死在 HTML 里
function populateEnvSelect(envs) {
  const sel = document.getElementById('env-select');
  if (!sel || !Array.isArray(envs)) return;
  const current = STATE.env || sel.value;
  sel.innerHTML = envs.map(e =>
    `<option value="${escapeHTML(e.id)}">${escapeHTML(e.name || e.id)}</option>`
  ).join('');
  if (envs.some(e => e.id === current)) sel.value = current;
}

// 日志源提示：选中的源不可用时说清原因，全部不可用时给出操作指引。
// 之前本地文件不存在只会静默重试，页面一片空白，很容易误以为工具坏了。
function updateSourceHint(states, current) {
  const badge = document.getElementById('remote-log-status-badge');
  if (!badge) return;

  const map = {};
  states.forEach(s => { map[s.id] = s; });
  const cur = map[current];

  if (!states.some(s => s.available)) {
    badge.textContent = '⚠ 无可用日志源';
    badge.className = 'remote-status-badge remote-status-error';
    badge.title = states.map(s => `${s.id}: ${s.reason}`).join('\n')
      + '\n\n用 --log 指定日志文件，或配置云端 token / --k8s-config';
    return;
  }

  if (cur && !cur.available) {
    badge.textContent = `${current} 不可用`;
    badge.className = 'remote-status-badge remote-status-error';
    badge.title = cur.reason;
    return;
  }

  if (cur && cur.available && cur.reason) {
    badge.title = cur.reason;
  }
}
