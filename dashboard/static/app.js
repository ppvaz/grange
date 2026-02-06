// Grange Dashboard — Client

(function () {
  'use strict';

  const POLL_MS = 2000;
  let prevState = null;
  let pollTimer = null;

  // --- DOM refs ---

  const $ = (id) => document.getElementById(id);
  const header     = $('header');
  const workDirEl  = $('workDir');
  const statusDot  = $('statusDot');
  const statusLabel= $('statusLabel');
  const agentsEl   = $('agents');
  const visionBody = $('visionBody');
  const planBody   = $('planBody');
  const signalsBody= $('signalsBody');
  const pipelineBody=$('pipelineBody');
  const specsBody  = $('specsBody');
  const activityBody=$('activityBody');
  const modalBackdrop=$('modalBackdrop');
  const modalTitle = $('modalTitle');
  const modalContent=$('modalContent');
  const modalClose = $('modalClose');

  // --- Mini Markdown renderer ---

  function renderMarkdown(text) {
    const lines = text.split('\n');
    let html = '';
    let inCode = false;

    for (let i = 0; i < lines.length; i++) {
      let line = lines[i];

      // Fenced code blocks
      if (line.trimStart().startsWith('```')) {
        if (inCode) {
          html += '</code></pre>';
          inCode = false;
        } else {
          html += '<pre><code>';
          inCode = true;
        }
        continue;
      }
      if (inCode) {
        html += escapeHtml(line) + '\n';
        continue;
      }

      // Headers
      const hMatch = line.match(/^(#{1,3})\s+(.+)/);
      if (hMatch) {
        const level = hMatch[1].length;
        html += `<h${level}>${inline(hMatch[2])}</h${level}>`;
        continue;
      }

      // HR
      if (/^---+\s*$/.test(line)) {
        html += '<hr>';
        continue;
      }

      // Checkboxes
      if (/^\s*- \[x\]/i.test(line)) {
        html += `<p class="task-done">&#9745; ${inline(line.replace(/^\s*- \[x\]\s*/i, ''))}</p>`;
        continue;
      }
      if (/^\s*- \[ \]/.test(line)) {
        html += `<p class="task-pending">&#9744; ${inline(line.replace(/^\s*- \[ \]\s*/, ''))}</p>`;
        continue;
      }

      // Unordered list
      if (/^\s*[-*]\s+/.test(line)) {
        html += `<li>${inline(line.replace(/^\s*[-*]\s+/, ''))}</li>`;
        continue;
      }

      // Ordered list
      if (/^\s*\d+\.\s+/.test(line)) {
        html += `<li>${inline(line.replace(/^\s*\d+\.\s+/, ''))}</li>`;
        continue;
      }

      // Empty line
      if (line.trim() === '') {
        html += '<br>';
        continue;
      }

      // Paragraph
      html += `<p>${inline(line)}</p>`;
    }

    if (inCode) html += '</code></pre>';
    return html;
  }

  function inline(text) {
    // Bold
    text = text.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
    // Inline code
    text = text.replace(/`([^`]+)`/g, '<code>$1</code>');
    return text;
  }

  function escapeHtml(s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // --- Modal ---

  function openModal(title, content) {
    modalTitle.textContent = title;
    modalContent.innerHTML = content;
    modalBackdrop.classList.add('open');
  }

  function closeModal() {
    modalBackdrop.classList.remove('open');
  }

  modalClose.addEventListener('click', closeModal);
  modalBackdrop.addEventListener('click', function (e) {
    if (e.target === modalBackdrop) closeModal();
  });
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') closeModal();
  });

  // --- File viewer ---

  async function viewFile(key, title) {
    try {
      const resp = await fetch('/api/file/' + key);
      if (!resp.ok) throw new Error('Not found');
      const text = await resp.text();
      openModal(title, renderMarkdown(text));
    } catch {
      openModal(title, '<p class="empty-state">File not available</p>');
    }
  }

  // --- Renderers ---

  const agentColors = {
    Executor: 'executor', Planner: 'planner', Critic: 'critic',
    Gap: 'gap', Oracle: 'oracle', Visionary: 'visionary'
  };

  function renderAgents(agents) {
    agentsEl.innerHTML = agents.map(function (a) {
      const dotClass = a.status === 'running' ? 'running' : a.status === 'locked' ? 'locked' : '';
      const timeStr = a.lastRun ? a.lastRun : '';
      const elapsed = a.status === 'running' && a.elapsed > 0 ? formatElapsed(a.elapsed) : '';
      return `<div class="agent" data-agent="${a.name}" title="Click to view log">
        <span class="agent-dot ${dotClass}"></span>
        <span class="agent-name">${a.name}</span>
        ${elapsed ? `<span class="agent-elapsed">${elapsed}</span>` : ''}
        ${timeStr ? `<span class="agent-time">${timeStr}</span>` : ''}
      </div>`;
    }).join('');

    // Attach click handlers
    agentsEl.querySelectorAll('.agent').forEach(function (el) {
      el.addEventListener('click', function () {
        const name = el.dataset.agent;
        viewFile('agent-' + name.toLowerCase(), name + ' Log');
      });
    });
  }

  function formatElapsed(seconds) {
    if (seconds < 60) return seconds + 's';
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return m + 'm' + (s > 0 ? s + 's' : '');
  }

  function renderPlan(plan) {
    if (!plan.exists) {
      planBody.innerHTML = '<div class="empty-state">No PLAN.md found</div>';
      return;
    }

    let html = `<div class="plan-summary">
      <span class="done-count">${plan.completed} done</span> &middot;
      <span class="pending-count">${plan.pending} remaining</span>
    </div>`;

    if (plan.tasks && plan.tasks.length > 0) {
      html += '<ul class="task-list">';
      // Show pending first, then completed (reversed for recent first)
      const pending = plan.tasks.filter(function(t) { return t.match(/^- \[ \]/); });
      const done = plan.tasks.filter(function(t) { return t.match(/^- \[x\]/i); });
      const ordered = pending.concat(done);

      ordered.forEach(function (task) {
        const isDone = /^- \[x\]/i.test(task);
        const text = task.replace(/^- \[[ xX]\]\s*/, '');
        html += `<li class="task-item ${isDone ? 'completed' : ''}">
          <span class="task-check">${isDone ? '☑' : '☐'}</span>
          <span>${escapeHtml(text)}</span>
        </li>`;
      });
      html += '</ul>';
    }

    planBody.innerHTML = html;
  }

  function renderVision(files) {
    const f = files['vision'];
    if (!f || !f.exists) {
      visionBody.innerHTML = '<div class="empty-state">No VISION.md found</div>';
      return;
    }
    // Fetch and render inline
    fetch('/api/file/vision')
      .then(function (resp) { return resp.ok ? resp.text() : Promise.reject(); })
      .then(function (text) { visionBody.innerHTML = renderMarkdown(text); })
      .catch(function () {
        visionBody.innerHTML = '<div class="empty-state">Could not load VISION.md</div>';
      });
  }

  function renderSignals(files) {
    const signals = [
      { key: 'blockers', label: 'BLOCKERS' },
      { key: 'drift',    label: 'DRIFT' },
      { key: 'cuts',     label: 'CUTS' },
      { key: 'review',   label: 'REVIEW' },
      { key: 'done',     label: 'DONE' }
    ];

    signalsBody.innerHTML = signals.map(function (s) {
      const f = files[s.key];
      if (!f || !f.exists) {
        return `<div class="signal-row">
          <span class="signal-name">${s.label}</span>
          <span class="signal-missing">—</span>
        </div>`;
      }
      return `<div class="signal-row">
        <span class="signal-name">${s.label}</span>
        <span class="signal-right">
          <span class="signal-lines">${f.lines} lines</span>
          <button class="signal-btn" data-key="${s.key}" data-title="${s.label}">▶</button>
        </span>
      </div>`;
    }).join('');

    signalsBody.querySelectorAll('.signal-btn').forEach(function (btn) {
      btn.addEventListener('click', function (e) {
        e.stopPropagation();
        viewFile(btn.dataset.key, btn.dataset.title);
      });
    });
  }

  function renderPipeline(pipeline) {
    if (!pipeline.active) {
      pipelineBody.innerHTML = '<div class="pipeline-inactive">No active pipeline</div>';
      return;
    }

    let trackHtml = '<div class="pipeline-track">';
    pipeline.stages.forEach(function (stage, i) {
      if (i > 0) {
        const connDone = pipeline.stages[i - 1].status === 'complete' ? 'done' : '';
        trackHtml += `<div class="pipeline-connector ${connDone}"></div>`;
      }
      trackHtml += `<div class="pipeline-node ${stage.status}">${stage.id}</div>`;
    });
    trackHtml += '</div>';

    let labelsHtml = '<div class="pipeline-labels">';
    pipeline.stages.forEach(function (stage) {
      labelsHtml += `<span class="pipeline-label ${stage.status}">${stage.name}</span>`;
    });
    labelsHtml += '</div>';

    let targetHtml = '';
    if (pipeline.targetPath) {
      targetHtml = `<div class="pipeline-target">${escapeHtml(pipeline.targetPath)}</div>`;
    }

    pipelineBody.innerHTML = trackHtml + labelsHtml + targetHtml;
  }

  function renderSpecs(specs) {
    if (specs.total === 0) {
      specsBody.innerHTML = '<div class="spec-empty">No specs found</div>';
      return;
    }

    let html = '';

    if (specs.patterns > 0) {
      html += `<div class="spec-row">
        <span class="spec-name">patterns/</span>
        <span class="spec-count">${specs.patterns}</span>
      </div>`;
    }

    Object.keys(specs.projects).sort().forEach(function (name) {
      html += `<div class="spec-row">
        <span class="spec-name">${escapeHtml(name)}/</span>
        <span class="spec-count">${specs.projects[name]}</span>
      </div>`;
    });

    html += `<div class="spec-total">total: <strong>${specs.total}</strong></div>`;

    specsBody.innerHTML = html;
  }

  function renderActivity(log) {
    if (!log || log.length === 0) {
      activityBody.innerHTML = '<div class="log-empty">No activity yet</div>';
      return;
    }

    // Reverse to show newest first
    const reversed = log.slice().reverse();

    activityBody.innerHTML = reversed.map(function (entry) {
      let msg = escapeHtml(entry.message);
      // Colorize agent tags [AgentName]
      msg = msg.replace(/\[(\w+)\]/g, function (match, name) {
        const cls = agentColors[name];
        if (cls) return `<span class="agent-tag ${cls}">[${name}]</span>`;
        return match;
      });

      return `<div class="log-entry">
        ${entry.time ? `<span class="log-time">${entry.time}</span>` : ''}
        <span class="log-msg">${msg}</span>
      </div>`;
    }).join('');
  }

  // --- State polling ---

  function setConnected(ok) {
    statusDot.className = 'status-dot ' + (ok ? 'live' : 'error');
    statusLabel.textContent = ok ? 'Live' : 'Error';
  }

  async function poll() {
    try {
      const resp = await fetch('/api/state');
      if (!resp.ok) throw new Error('HTTP ' + resp.status);
      const state = await resp.json();
      setConnected(true);
      render(state);
      prevState = state;
    } catch {
      setConnected(false);
    }
  }

  function render(state) {
    // Header
    workDirEl.textContent = state.workDir;
    if (state.done) {
      header.classList.add('done');
    } else {
      header.classList.remove('done');
    }

    // Sections
    renderAgents(state.agents);
    renderVision(state.files);
    renderPlan(state.plan);
    renderSignals(state.files);
    renderPipeline(state.pipeline);
    renderSpecs(state.specs);
    renderActivity(state.log);
  }

  // --- Init ---

  poll();
  pollTimer = setInterval(poll, POLL_MS);
})();
