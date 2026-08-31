const state = { projects: [], health: null, events: [] };
const el = id => document.getElementById(id);
const number = value => new Intl.NumberFormat('zh-CN').format(Number(value || 0));
const time = value => value ? new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value)) : '—';

function setConnection(kind, text) {
  const node = el('connection');
  node.className = 'status ' + kind;
  node.lastElementChild.textContent = text;
}

function renderMetrics() {
  const projects = state.projects;
  el('project-count').textContent = number(projects.length);
  el('chapter-count').textContent = number(projects.reduce((sum, p) => sum + (p.completed_chapters || 0), 0));
  el('word-count').textContent = number(projects.reduce((sum, p) => sum + (p.total_words || 0), 0));
  el('version').textContent = state.health?.version || 'dev';
  el('uptime').textContent = state.health ? `运行 ${number(state.health.uptime_seconds)} 秒 · API ${state.health.api_version}` : '服务状态未知';
  el('workspace-label').textContent = state.health ? `工作区：${state.health.workspace}` : '工作区不可用';
}

function renderProjects() {
  const container = el('projects');
  container.replaceChildren();
  if (!state.projects.length) {
    const empty = document.createElement('div');
    empty.className = 'empty';
    empty.textContent = '没有发现 ainovel / NovelForge 项目。把项目目录放入 --workspace 指定的目录后刷新。';
    container.append(empty);
    return;
  }
  state.projects.forEach(project => {
    const row = document.createElement('article');
    row.className = 'project';
    const body = document.createElement('div');
    const title = document.createElement('h3');
    title.textContent = project.title;
    const meta = document.createElement('div');
    meta.className = 'project-meta';
    [
      project.phase ? `阶段 ${project.phase}` : '阶段未初始化',
      `当前第 ${number(project.current_chapter)} 章`,
      project.current_volume ? `卷 ${number(project.current_volume)} / 弧 ${number(project.current_arc)}` : null,
      `格式 v${number(project.format_version)}`
    ].filter(Boolean).forEach(text => {
      const badge = document.createElement('span');
      badge.className = 'badge';
      badge.textContent = text;
      meta.append(badge);
    });
    const path = document.createElement('div');
    path.className = 'project-path';
    path.textContent = project.path;
    const progress = document.createElement('progress');
    progress.className = 'progress';
    progress.max = 100;
    const ratio = project.total_chapters > 0 ? Math.min(100, project.completed_chapters / project.total_chapters * 100) : 0;
    progress.value = ratio;
    progress.setAttribute('aria-label', `完成度 ${Math.round(ratio)}%`);
    body.append(title, meta, path, progress);

    const stat = document.createElement('div');
    stat.className = 'project-stat';
    const strong = document.createElement('strong');
    strong.textContent = number(project.total_words);
    const small = document.createElement('small');
    small.textContent = `${number(project.completed_chapters)} / ${number(project.total_chapters)} 章`;
    stat.append(strong, small);
    row.append(body, stat);
    container.append(row);
  });
}

function renderEvents() {
  const container = el('events');
  container.replaceChildren();
  if (!state.events.length) {
    const empty = document.createElement('div');
    empty.className = 'empty';
    empty.textContent = '等待服务器事件…';
    container.append(empty);
    return;
  }
  state.events.slice(0, 30).forEach(event => {
    const row = document.createElement('div');
    row.className = 'event';
    const top = document.createElement('div');
    top.className = 'event-top';
    const type = document.createElement('span');
    type.className = 'event-type';
    type.textContent = event.type || 'message';
    const stamp = document.createElement('span');
    stamp.className = 'event-time';
    stamp.textContent = time(event.time);
    top.append(type, stamp);
    const data = document.createElement('div');
    data.className = 'event-data';
    data.textContent = event.project ? `${event.project} · ${JSON.stringify(event.data || {})}` : JSON.stringify(event.data || {});
    row.append(top, data);
    container.append(row);
  });
}

async function load() {
  el('refresh').disabled = true;
  try {
    const [healthResponse, projectsResponse] = await Promise.all([fetch('/api/health'), fetch('/api/projects')]);
    if (!healthResponse.ok || !projectsResponse.ok) throw new Error('API 返回非成功状态');
    state.health = await healthResponse.json();
    state.projects = (await projectsResponse.json()).projects || [];
    renderMetrics();
    renderProjects();
    el('last-refresh').textContent = `更新 ${time(new Date())}`;
    setConnection('online', '在线');
  } catch (error) {
    setConnection('offline', '离线');
    const container = el('projects');
    container.replaceChildren();
    const message = document.createElement('div');
    message.className = 'error';
    message.textContent = `读取失败：${error.message}`;
    container.append(message);
  } finally {
    el('refresh').disabled = false;
  }
}

function connectEvents() {
  const source = new EventSource('/api/events');
  source.onopen = () => setConnection('online', '在线');
  source.onerror = () => setConnection('offline', '重连中');
  const receive = message => {
    try {
      state.events.unshift(JSON.parse(message.data));
      renderEvents();
    } catch (_) {}
  };
  source.onmessage = receive;
  ['connected', 'server.ready', 'job.started', 'job.progress', 'agent.started', 'agent.output', 'agent.completed', 'chapter.generated', 'chapter.reviewed', 'chapter.rewritten', 'chapter.finalized', 'checkpoint.created', 'automation.paused', 'automation.completed', 'error'].forEach(type => source.addEventListener(type, receive));
}

const savedTheme = localStorage.getItem('novelforge-theme');
if (savedTheme === 'light' || savedTheme === 'dark') document.documentElement.dataset.theme = savedTheme;
el('theme').addEventListener('click', () => {
  const next = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light';
  document.documentElement.dataset.theme = next;
  localStorage.setItem('novelforge-theme', next);
});
el('refresh').addEventListener('click', load);
load();
connectEvents();
