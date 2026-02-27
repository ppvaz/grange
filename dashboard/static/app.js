// Grange Dashboard — Client

(function () {
  'use strict';

  const POLL_MS = 2000;
  let prevState = null;
  let pollTimer = null;

  // Multi-project support: read ?project= param for drill-down
  const urlParams = new URLSearchParams(window.location.search);
  const projectParam = urlParams.get('project');
  const apiSuffix = projectParam ? '?project=' + encodeURIComponent(projectParam) : '';

  // --- DOM refs ---

  const $ = (id) => document.getElementById(id);
  const header     = $('header');
  const workDirEl  = $('workDir');
  const statusDot  = $('statusDot');
  const statusLabel= $('statusLabel');
  const healthBar  = $('healthBar');
  const agentsEl   = $('agents');
  const visionBody = $('visionBody');
  const planBody   = $('planBody');
  const signalsBody= $('signalsBody');
  const ciBody     = $('ciBody');
  const gateBody   = $('gateBody');
  const reviewGateBody=$('reviewGateBody');
  const modeBody   = $('modeBody');
  const pipelineBody=$('pipelineBody');
  const specsBody  = $('specsBody');
  const activityBody=$('activityBody');
  const modalBackdrop=$('modalBackdrop');
  const modalTitle = $('modalTitle');
  const modalContent=$('modalContent');
  const modalClose = $('modalClose');
  const toastContainer=$('toastContainer');
  const btnStart   = $('btnStart');
  const btnStop    = $('btnStop');

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

  // --- Toast notifications ---

  function showToast(message, type) {
    var el = document.createElement('div');
    el.className = 'toast toast-' + (type || 'success');
    el.textContent = message;
    toastContainer.appendChild(el);
    setTimeout(function () {
      if (el.parentNode) el.parentNode.removeChild(el);
    }, 3000);
  }

  // --- Action helper ---

  async function postAction(endpoint, body) {
    try {
      var opts = {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' }
      };
      if (body) opts.body = JSON.stringify(body);
      var resp = await fetch(endpoint + apiSuffix, opts);
      var data = await resp.json();
      if (data.ok) {
        showToast(data.message || 'Done', 'success');
      } else {
        showToast(data.error || 'Action failed', 'error');
      }
      // Trigger immediate re-poll to reflect changes
      poll();
      return data;
    } catch (err) {
      showToast('Network error', 'error');
      return { ok: false, error: err.message };
    }
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
      const resp = await fetch('/api/file/' + key + apiSuffix);
      if (!resp.ok) throw new Error('Not found');
      const text = await resp.text();
      openModal(title, renderMarkdown(text));
    } catch {
      openModal(title, '<p class="empty-state">File not available</p>');
    }
  }

  // --- Event delegation ---
  // Single listener on document.body for all interactive buttons.
  // This avoids losing listeners when innerHTML is replaced.

  document.body.addEventListener('click', function (e) {
    var target = e.target;

    // Signal file viewer buttons
    if (target.classList.contains('signal-btn') && target.dataset.key) {
      e.stopPropagation();
      viewFile(target.dataset.key, target.dataset.title);
      return;
    }

    // CI log button
    if (target.classList.contains('ci-log-btn')) {
      viewFile('ci', 'CI Log');
      return;
    }

    // Agent card click — view log
    var agentEl = target.closest('.agent[data-agent]');
    if (agentEl && !target.classList.contains('btn-agent-run')) {
      viewFile('agent-' + agentEl.dataset.agent.toLowerCase(), agentEl.dataset.agent + ' Log');
      return;
    }

    // Run single agent button
    if (target.classList.contains('btn-agent-run')) {
      e.stopPropagation();
      var agent = target.dataset.agent;
      postAction('/api/action/run-agent?agent=' + encodeURIComponent(agent));
      return;
    }

    // Task checkbox toggle
    if (target.classList.contains('task-check-btn')) {
      var idx = parseInt(target.dataset.index, 10);
      var isDone = target.dataset.done === 'true';
      if (isDone) {
        if (!confirm('Uncheck this task? This may cause agents to redo work.')) return;
      }
      postAction('/api/action/plan/toggle', { index: idx });
      return;
    }

    // Task cut button
    if (target.classList.contains('task-cut-btn')) {
      var cutIdx = parseInt(target.dataset.index, 10);
      var reason = prompt('Reason for cutting this task:');
      if (reason === null) return; // cancelled
      postAction('/api/action/plan/remove', { index: cutIdx, reason: reason });
      return;
    }

    // Plan add button
    if (target.id === 'btnPlanAdd') {
      addPlanTask();
      return;
    }

    // CI run button
    if (target.id === 'btnCIRun') {
      postAction('/api/action/ci/run');
      return;
    }

    // Approve gate button
    if (target.id === 'btnApprove') {
      postAction('/api/action/approve');
      return;
    }

    // Review acknowledge button
    if (target.id === 'btnReviewAck') {
      postAction('/api/action/review-ack');
      return;
    }
  });

  // Plan add via Enter key
  document.body.addEventListener('keydown', function (e) {
    if (e.target.id === 'planTaskInput' && e.key === 'Enter') {
      e.preventDefault();
      addPlanTask();
    }
  });

  function addPlanTask() {
    var input = $('planTaskInput');
    if (!input) return;
    var task = input.value.trim();
    if (!task) return;
    input.value = '';
    postAction('/api/action/plan/add', { task: task });
  }

  // Header Start/Stop buttons
  btnStart.addEventListener('click', function () {
    postAction('/api/action/start');
  });
  btnStop.addEventListener('click', function () {
    if (!confirm('Stop all agents?')) return;
    postAction('/api/action/stop');
  });

  // --- Renderers ---

  const agentColors = {
    Executor: 'executor', Planner: 'planner',
    Gap: 'gap', Oracle: 'oracle', Visionary: 'visionary'
  };

  function renderHealth(health) {
    if (!health) return;
    var timeoutCls = health.timeoutCount > 0 ? ' amber' : '';
    healthBar.innerHTML =
      '<span class="health-item">agents <span class="health-value">' + health.agentCount + '/' + health.maxAgents + '</span></span>' +
      '<span class="health-item">timeouts <span class="health-value' + timeoutCls + '">' + health.timeoutCount + '</span></span>' +
      '<span class="health-item">commits <span class="health-value">' + health.commitCount + '</span></span>';
  }

  function renderAgents(agents, running) {
    agentsEl.innerHTML = agents.map(function (a) {
      var dotClass = a.status === 'running' ? 'running' : a.status === 'locked' ? 'locked' : '';
      var timeStr = a.lastRun ? a.lastRun : '';
      var elapsed = a.status === 'running' && a.elapsed > 0 ? formatElapsed(a.elapsed) : '';
      var runBtn = (a.status === 'idle') ?
        '<button class="btn btn-agent-run" data-agent="' + a.name.toLowerCase() + '">Run</button>' : '';
      return '<div class="agent" data-agent="' + a.name + '" title="Click to view log">' +
        '<span class="agent-dot ' + dotClass + '"></span>' +
        '<span class="agent-name">' + a.name + '</span>' +
        (elapsed ? '<span class="agent-elapsed">' + elapsed + '</span>' : '') +
        (timeStr ? '<span class="agent-time">' + timeStr + '</span>' : '') +
        runBtn +
        '</div>';
    }).join('');
  }

  function formatElapsed(seconds) {
    if (seconds < 60) return seconds + 's';
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return m + 'm' + (s > 0 ? s + 's' : '');
  }

  function renderPlan(plan) {
    if (!plan.exists) {
      planBody.innerHTML = '<div class="plan-add">' +
        '<input type="text" id="planTaskInput" placeholder="Add a task..." maxlength="500">' +
        '<button class="btn" id="btnPlanAdd">+</button>' +
        '</div>' +
        '<div class="empty-state">No PLAN.md found</div>';
      return;
    }

    var html = '<div class="plan-add">' +
      '<input type="text" id="planTaskInput" placeholder="Add a task..." maxlength="500">' +
      '<button class="btn" id="btnPlanAdd">+</button>' +
      '</div>';

    html += '<div class="plan-summary">' +
      '<span class="done-count">' + plan.completed + ' done</span> &middot; ' +
      '<span class="pending-count">' + plan.pending + ' remaining</span>' +
      '</div>';

    if (plan.tasks && plan.tasks.length > 0) {
      html += '<ul class="task-list">';

      // Build index map: tasks appear in original PLAN.md order with their original indices
      // We need original index (position among checkbox lines in PLAN.md) for API calls
      plan.tasks.forEach(function (task, originalIdx) {
        var isDone = /^- \[x\]/i.test(task);
        var text = task.replace(/^- \[[ xX]\]\s*/, '');
        html += '<li class="task-item ' + (isDone ? 'completed' : '') + '">' +
          '<button class="task-check-btn" data-index="' + originalIdx + '" data-done="' + isDone + '">' +
          (isDone ? '\u2611' : '\u2610') + '</button>' +
          '<span>' + escapeHtml(text) + '</span>' +
          '<button class="task-cut-btn" data-index="' + originalIdx + '" title="Cut task">\u2715</button>' +
          '</li>';
      });
      html += '</ul>';
    }

    planBody.innerHTML = html;
  }

  var lastVisionModified = null;

  function renderVision(files) {
    const f = files['vision'];
    if (!f || !f.exists) {
      visionBody.innerHTML = '<div class="empty-state">No VISION.md found</div>';
      lastVisionModified = null;
      return;
    }
    // Skip fetch if vision hasn't changed
    if (prevState && prevState.files && prevState.files.vision &&
        prevState.files.vision.modified === f.modified &&
        lastVisionModified === f.modified) {
      return;
    }
    lastVisionModified = f.modified;
    // Fetch and render inline
    fetch('/api/file/vision' + apiSuffix)
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
        return '<div class="signal-row">' +
          '<span class="signal-name">' + s.label + '</span>' +
          '<span class="signal-missing">\u2014</span>' +
          '</div>';
      }
      return '<div class="signal-row">' +
        '<span class="signal-name">' + s.label + '</span>' +
        '<span class="signal-right">' +
        '<span class="signal-lines">' + f.lines + ' lines</span>' +
        '<button class="signal-btn" data-key="' + s.key + '" data-title="' + s.label + '">\u25B6</button>' +
        '</span>' +
        '</div>';
    }).join('');
  }

  function renderCI(ci) {
    if (!ci || !ci.enabled) {
      ciBody.innerHTML = '<div class="ci-status ci-disabled">No ci.sh found</div>';
      return;
    }

    var icon, cls, label;
    if (ci.lastResult === 'pass') {
      icon = '&#10003;'; cls = 'ci-pass'; label = 'Passed';
    } else if (ci.lastResult === 'fail') {
      icon = '&#10007;'; cls = 'ci-fail'; label = 'Failed';
    } else {
      icon = '&#8226;'; cls = 'ci-unknown'; label = 'No runs yet';
    }

    var html = '<div class="ci-status ' + cls + '">';
    html += '<span class="ci-icon">' + icon + '</span> ';
    html += '<span class="ci-label">' + label + '</span>';
    if (ci.lastRun) {
      html += '<span class="ci-time">' + escapeHtml(ci.lastRun) + '</span>';
    }
    html += '</div>';
    html += '<div style="display:flex;gap:8px;margin-top:8px">';
    html += '<button class="signal-btn ci-log-btn">View CI Log</button>';
    html += '<button class="btn btn-ci-run" id="btnCIRun">Run CI</button>';
    html += '</div>';

    ciBody.innerHTML = html;
  }

  function renderGate(gate) {
    if (!gate || !gate.enabled) {
      gateBody.innerHTML = '<div class="gate-status gate-disabled">Gate disabled</div>';
      return;
    }

    var html;
    if (gate.status === 'waiting') {
      html = '<div class="gate-status gate-waiting">';
      html += '<span class="gate-icon">&#9679;</span> ';
      html += 'Waiting for approval';
      html += '</div>';
      html += '<button class="btn btn-approve" id="btnApprove">Approve Plan</button>';
    } else if (gate.status === 'approved') {
      html = '<div class="gate-status gate-approved">';
      html += '<span class="gate-icon">&#10003;</span> Approved';
      html += '</div>';
    } else {
      html = '<div class="gate-status gate-disabled">Gate disabled</div>';
    }

    gateBody.innerHTML = html;
  }

  function renderReviewGate(reviewGate) {
    if (!reviewGate || !reviewGate.enabled) {
      reviewGateBody.innerHTML = '<div class="gate-status gate-disabled">Review gate disabled</div>';
      return;
    }

    var html;
    if (reviewGate.status === 'pending') {
      html = '<div class="gate-status gate-waiting">';
      html += '<span class="gate-icon">&#9679;</span> ';
      html += 'Vision review pending';
      html += '</div>';
      html += '<button class="btn btn-review-ack" id="btnReviewAck">Acknowledge Review</button>';
    } else if (reviewGate.status === 'acknowledged') {
      html = '<div class="gate-status gate-approved">';
      html += '<span class="gate-icon">&#10003;</span> Acknowledged';
      html += '</div>';
    } else {
      html = '<div class="gate-status gate-approved">';
      html += '<span class="gate-icon">&#10003;</span> Clear';
      html += '</div>';
    }

    reviewGateBody.innerHTML = html;
  }

  function renderMode(mode, config) {
    var label, desc;
    if (mode === 'pair') {
      label = 'Pair';
      desc = 'Interactive \u2014 gates ON, Visionary always validates';
    } else if (mode === 'sleep') {
      label = 'Sleep';
      desc = 'Autonomous \u2014 gates OFF, safety checks active';
    } else {
      label = 'Auto';
      desc = 'Custom configuration via individual ENABLE_* vars';
    }

    var html = '<div class="mode-label">' +
      '<span class="mode-name">' + escapeHtml(label) + '</span>' +
      '<span class="mode-desc">' + escapeHtml(desc) + '</span>' +
      '</div>';

    // Show config flags
    if (config) {
      html += '<div class="config-list">';
      var keys = Object.keys(config).sort();
      for (var i = 0; i < keys.length; i++) {
        html += '<div class="config-row">' +
          '<span class="config-key">' + escapeHtml(keys[i]) + '</span>' +
          '<span class="config-val">' + escapeHtml(config[keys[i]]) + '</span>' +
          '</div>';
      }
      html += '</div>';
    }

    modeBody.innerHTML = html;
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

  function renderStartStop(running) {
    btnStart.disabled = running;
    btnStop.disabled = !running;
  }

  // --- State polling ---

  function setConnected(ok) {
    statusDot.className = 'status-dot ' + (ok ? 'live' : 'error');
    statusLabel.textContent = ok ? 'Live' : 'Error';
  }

  async function poll() {
    try {
      const resp = await fetch('/api/state' + apiSuffix);
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

    // Health + Start/Stop
    renderHealth(state.health);
    renderStartStop(state.running);

    // Sections
    renderAgents(state.agents, state.running);
    renderVision(state.files);
    renderPlan(state.plan);
    renderSignals(state.files);
    renderCI(state.ci);
    renderGate(state.gate);
    renderReviewGate(state.reviewGate);
    renderMode(state.mode, state.config);
    renderPipeline(state.pipeline);
    renderSpecs(state.specs);
    renderActivity(state.log);
  }

  // --- Init ---

  poll();
  pollTimer = setInterval(poll, POLL_MS);
})();
