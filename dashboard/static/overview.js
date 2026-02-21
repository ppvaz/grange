// Grange Overview — Multi-project dashboard

(function () {
  'use strict';

  const POLL_MS = 5000;

  const $ = (id) => document.getElementById(id);
  const scanDirEl    = $('scanDir');
  const projectCount = $('projectCount');
  const statusDot    = $('statusDot');
  const statusLabel  = $('statusLabel');
  const statsBar     = $('statsBar');
  const attentionSection = $('attentionSection');
  const attentionList = $('attentionList');
  const projectsGrid = $('projectsGrid');

  function escapeHtml(s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function setConnected(ok) {
    statusDot.className = 'status-dot ' + (ok ? 'live' : 'error');
    statusLabel.textContent = ok ? 'Live' : 'Error';
  }

  function statusClass(status) {
    return 'status-' + status;
  }

  function statusLabel_(status) {
    return status.charAt(0).toUpperCase() + status.slice(1);
  }

  function progressPercent(done, total) {
    if (total === 0) return 0;
    return Math.round((done / total) * 100);
  }

  function renderStats(stats) {
    const pct = stats.totalTasks > 0
      ? Math.round((stats.totalDone / stats.totalTasks) * 100)
      : 0;

    const byStatus = stats.byStatus || {};
    statsBar.innerHTML = `
      <div class="stat">
        <span class="stat-value">${(byStatus.active || 0) + (byStatus.ike || 0) + (byStatus.stalled || 0) + (byStatus['new'] || 0)}</span>
        <span class="stat-label">In Progress</span>
      </div>
      <div class="stat">
        <span class="stat-value">${byStatus.done || 0}</span>
        <span class="stat-label">Completed</span>
      </div>
      <div class="stat">
        <span class="stat-value">${stats.totalDone}/${stats.totalTasks}</span>
        <span class="stat-label">Tasks (${pct}%)</span>
      </div>
      <div class="stat">
        <span class="stat-value">${stats.totalCommits}</span>
        <span class="stat-label">Total Commits</span>
      </div>
    `;
  }

  function renderAttention(attention) {
    if (!attention || attention.length === 0) {
      attentionSection.style.display = 'none';
      return;
    }
    attentionSection.style.display = '';
    attentionList.innerHTML = attention.map(function (item) {
      return `<div class="attention-item">
        <a class="attention-project" href="/project?project=${encodeURIComponent(item.path)}">${escapeHtml(item.project)}</a>
        <span class="attention-reason">${escapeHtml(item.reason)}</span>
      </div>`;
    }).join('');
  }

  function renderProjects(projects) {
    if (!projects || projects.length === 0) {
      projectsGrid.innerHTML = '<div class="empty-state">No grange projects found</div>';
      return;
    }

    projectsGrid.innerHTML = projects.map(function (p) {
      const pct = progressPercent(p.done, p.total);
      let progressHtml;
      if (p.pipeline && p.pipeline.active) {
        const stage = p.pipeline.stages.find(function (s) { return s.status === 'active'; });
        const label = stage ? stage.name : 'Starting';
        const stageId = stage ? stage.id : '';
        const ikePct = Math.round(((p.pipeline.currentStage + 1) / p.pipeline.stages.length) * 100);
        progressHtml = `<div class="card-progress">
             <div class="progress-bar"><div class="progress-fill ike-fill" style="width:${ikePct}%"></div></div>
             <span class="progress-text">IKE: ${escapeHtml(label)}${stageId ? ' (' + escapeHtml(stageId) + ')' : ''}</span>
           </div>`;
      } else if (p.total > 0) {
        progressHtml = `<div class="card-progress">
             <div class="progress-bar"><div class="progress-fill" style="width:${pct}%"></div></div>
             <span class="progress-text">${p.done}/${p.total} tasks</span>
           </div>`;
      } else {
        progressHtml = '<div class="card-progress"><span class="progress-text no-tasks">No tasks yet</span></div>';
      }

      const signalsHtml = p.signals > 0
        ? `<span class="card-signals" title="${p.signals} signal file(s)">${p.signals} signals</span>`
        : '';

      return `<a class="project-card" href="/project?project=${encodeURIComponent(p.path)}">
        <div class="card-header">
          <span class="card-name">${escapeHtml(p.name)}</span>
          <span class="card-badge ${statusClass(p.status)}">${statusLabel_(p.status)}</span>
        </div>
        ${p.goal ? `<div class="card-goal">${escapeHtml(p.goal)}</div>` : ''}
        ${progressHtml}
        <div class="card-meta">
          <span>${p.commits} commits</span>
          <span>${escapeHtml(p.lastCommit)}</span>
          ${signalsHtml}
        </div>
      </a>`;
    }).join('');
  }

  async function poll() {
    try {
      const resp = await fetch('/api/projects');
      if (!resp.ok) throw new Error('HTTP ' + resp.status);
      const data = await resp.json();
      setConnected(true);

      scanDirEl.textContent = data.scanDir;
      projectCount.textContent = data.projects.length + ' projects';

      renderStats(data.stats);
      renderAttention(data.attention);
      renderProjects(data.projects);
    } catch {
      setConnected(false);
    }
  }

  poll();
  setInterval(poll, POLL_MS);
})();
