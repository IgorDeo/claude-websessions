window.websessions = (function() {
  const terminals = {};
  const splitInstances = [];
  var openTabs = []; // [{id, name, state}]
  var activeTabId = null;

  // ── Notification Sounds (Web Audio API) ──────────────────
  var audioCtx = null;
  function getAudioCtx() {
    if (!audioCtx) {
      audioCtx = new (window.AudioContext || window.webkitAudioContext)();
    }
    return audioCtx;
  }

  function playTone(frequencies, durations, type, volume) {
    try {
      var ctx = getAudioCtx();
      var startTime = ctx.currentTime;
      for (var i = 0; i < frequencies.length; i++) {
        var osc = ctx.createOscillator();
        var gain = ctx.createGain();
        osc.type = type || 'sine';
        osc.frequency.value = frequencies[i];
        gain.gain.setValueAtTime(volume || 0.15, startTime);
        gain.gain.exponentialRampToValueAtTime(0.001, startTime + durations[i]);
        osc.connect(gain);
        gain.connect(ctx.destination);
        osc.start(startTime);
        osc.stop(startTime + durations[i]);
        startTime += durations[i] * 0.6; // overlap slightly
      }
    } catch(e) {}
  }

  var notifSounds = {
    completed: function() {
      // Ascending two-note chime — pleasant, success
      playTone([523, 784], [0.15, 0.25], 'sine', 0.12);
    },
    waiting: function() {
      // Three gentle pings — attention needed
      playTone([660, 660, 880], [0.1, 0.1, 0.2], 'sine', 0.1);
    },
    errored: function() {
      // Descending two-note — low, alert
      playTone([440, 330], [0.18, 0.3], 'triangle', 0.14);
    }
  };

  function playNotifSound(eventType) {
    // Check if sounds are enabled (stored in localStorage)
    try {
      if (localStorage.getItem('ws-notif-sounds') === 'false') return;
    } catch(e) {}
    var fn = notifSounds[eventType];
    if (fn) fn();
  }

  function setNotifSoundsEnabled(enabled) {
    try { localStorage.setItem('ws-notif-sounds', enabled ? 'true' : 'false'); } catch(e) {}
  }

  function getNotifSoundsEnabled() {
    try { return localStorage.getItem('ws-notif-sounds') !== 'false'; } catch(e) { return true; }
  }

  // Test sound for settings page
  function testNotifSound(eventType) {
    var fn = notifSounds[eventType];
    if (fn) fn();
  }
  // ─────────────────────────────────────────────────────────

  function connectSession(sessionID, containerID) {
    const container = document.getElementById(containerID);
    if (!container) return;

    const term = new Terminal({
      cursorBlink: true,
      scrollback: 10000,
      theme: {
        background: '#13141c',
        foreground: '#d0d4f0',
        cursor: '#6c8cff',
        selectionBackground: 'rgba(108, 140, 255, 0.2)',
      },
      fontFamily: "'Maple Mono Normal NF', 'IBM Plex Mono', 'JetBrains Mono', 'Fira Code', monospace",
      fontSize: 14,
    });

    const fitAddon = new FitAddon.FitAddon();
    term.loadAddon(fitAddon);
    term.open(container);
    fitAddon.fit();

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(protocol + '//' + window.location.host + '/ws/' + sessionID);
    ws.binaryType = 'arraybuffer';

    ws.onopen = function() {
      var dims = { type: 'resize', rows: term.rows, cols: term.cols };
      ws.send(JSON.stringify(dims));
    };

    ws.onmessage = function(event) {
      if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data));
      } else {
        try {
          var msg = JSON.parse(event.data);
          if (msg.type === 'notification') { handleNotification(msg); }
        } catch(e) {
          term.write(event.data);
        }
      }
    };

    ws.onclose = function() {
      term.write('\r\n\x1b[33m[Connection closed]\x1b[0m\r\n');
    };

    term.onData(function(data) {
      if (ws.readyState === WebSocket.OPEN) { ws.send(data); }
    });

    var resizeObserver = new ResizeObserver(function() {
      fitAddon.fit();
      if (ws.readyState === WebSocket.OPEN) {
        var dims = { type: 'resize', rows: term.rows, cols: term.cols };
        ws.send(JSON.stringify(dims));
      }
    });
    resizeObserver.observe(container);

    terminals[sessionID] = { term: term, ws: ws, fitAddon: fitAddon, resizeObserver: resizeObserver };
  }

  function disconnectSession(sessionID) {
    var t = terminals[sessionID];
    if (!t) return;
    t.ws.close();
    t.resizeObserver.disconnect();
    t.term.dispose();
    delete terminals[sessionID];
  }

  function splitPane(currentSessionID, direction) {
    var area = document.getElementById('terminal-area');
    if (!area) return;

    // Prompt user to pick a session for the new pane
    // Fetch sidebar sessions and show a quick picker
    fetch('/api/sessions')
      .then(function(r) { return r.json(); })
      .then(function(sessions) {
        showSplitPicker(currentSessionID, direction, sessions);
      })
      .catch(function() {
        // Fallback: just ask for session ID
        var sid = prompt('Session ID to open in new pane:');
        if (sid) doSplit(currentSessionID, sid, direction);
      });
  }

  function showSplitPicker(currentSessionID, direction, sessions) {
    // Create a small picker overlay
    var overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.addEventListener('click', function() { overlay.remove(); });
    var content = document.createElement('div');
    content.className = 'modal-content';
    content.style.width = '300px';
    content.addEventListener('click', function(e) { e.stopPropagation(); });
    var title = document.createElement('h2');
    title.textContent = 'Open in split';
    content.appendChild(title);
    sessions.forEach(function(s) {
      if (s.id === currentSessionID) return;
      var btn = document.createElement('button');
      btn.className = 'recent-item';
      btn.style.width = '100%';
      btn.style.marginBottom = '0.25rem';
      var nameSpan = document.createElement('span');
      nameSpan.className = 'recent-name';
      nameSpan.textContent = s.name;
      var pathSpan = document.createElement('span');
      pathSpan.className = 'recent-path';
      pathSpan.textContent = s.state + ' - ' + s.work_dir;
      btn.appendChild(nameSpan);
      btn.appendChild(pathSpan);
      btn.addEventListener('click', function() {
        overlay.remove();
        doSplit(currentSessionID, s.id, direction);
      });
      content.appendChild(btn);
    });
    if (sessions.length <= 1) {
      var msg = document.createElement('p');
      msg.textContent = 'No other sessions available';
      msg.style.color = '#565f89';
      msg.style.textAlign = 'center';
      msg.style.padding = '1rem';
      content.appendChild(msg);
    }
    overlay.appendChild(content);
    document.body.appendChild(overlay);
  }

  function doSplit(sessionID1, sessionID2, direction) {
    var area = document.getElementById('terminal-area');
    if (!area) return;

    // Disconnect existing terminals in the area
    for (var sid in terminals) {
      var el = document.getElementById('term-' + sid);
      if (el && area.contains(el)) {
        disconnectSession(sid);
      }
    }

    // Create two pane containers (no IDs with session names — use data attributes)
    var pane1 = document.createElement('div');
    pane1.className = 'split-pane';
    pane1.setAttribute('data-split-session', sessionID1);
    var pane2 = document.createElement('div');
    pane2.className = 'split-pane';
    pane2.setAttribute('data-split-session', sessionID2);

    // Clear area and add panes
    while (area.firstChild) area.removeChild(area.firstChild);
    area.appendChild(pane1);
    area.appendChild(pane2);

    // Set flex direction based on split direction
    area.style.flexDirection = direction === 'horizontal' ? 'row' : 'column';

    // Use Split.js with DOM elements directly (not CSS selectors)
    Split([pane1, pane2], {
      direction: direction === 'horizontal' ? 'horizontal' : 'vertical',
      sizes: [50, 50],
      minSize: 100,
      gutterSize: 4,
    });

    // Load terminal content into each pane via htmx
    htmx.ajax('POST', '/sessions/' + encodeURIComponent(sessionID1) + '/open', {
      target: pane1,
      swap: 'innerHTML'
    });
    htmx.ajax('POST', '/sessions/' + encodeURIComponent(sessionID2) + '/open', {
      target: pane2,
      swap: 'innerHTML'
    });
  }

  function showToast(title, body, event) {
    var container = document.getElementById('toast-container');
    if (!container) {
      container = document.createElement('div');
      container.id = 'toast-container';
      document.body.appendChild(container);
    }
    var toast = document.createElement('div');
    toast.className = 'toast toast-' + (event || 'info');
    var titleEl = document.createElement('div');
    titleEl.className = 'toast-title';
    titleEl.textContent = title;
    var bodyEl = document.createElement('div');
    bodyEl.className = 'toast-body';
    bodyEl.textContent = body;
    toast.appendChild(titleEl);
    toast.appendChild(bodyEl);
    toast.onclick = function() { toast.remove(); };
    container.appendChild(toast);
    setTimeout(function() {
      toast.classList.add('toast-fade');
      setTimeout(function() { toast.remove(); }, 300);
    }, 5000);
  }

  function handleNotification(msg) {
    var badge = document.querySelector('.badge');
    if (badge) {
      var count = parseInt(badge.textContent || '0') + 1;
      badge.textContent = count;
    }
    var title = 'websessions: ' + msg.event;
    var body = 'Session ' + msg.sessionID + ': ' + msg.event;
    if ('Notification' in window && Notification.permission === 'granted') {
      new Notification(title, {
        body: body,
        tag: 'ws-' + msg.sessionID + '-' + msg.event,
      });
    } else {
      showToast(title, body, msg.event);
    }
  }

  if ('Notification' in window && Notification.permission === 'default') {
    Notification.requestPermission();
  }

  // Clean up terminals whose DOM elements were removed before swapping new ones in
  document.addEventListener('htmx:beforeSwap', function(event) {
    if (event.detail.target.id !== 'terminal-area') return;
    // Disconnect all terminals in the area being replaced
    for (var sid in terminals) {
      var container = document.getElementById('term-' + sid);
      if (container && event.detail.target.contains(container)) {
        disconnectSession(sid);
      }
    }
  });

  document.addEventListener('htmx:afterSwap', function(event) {
    // Initialize terminal panes after swap
    var panes = event.detail.target.querySelectorAll('.terminal-pane[data-session-id]');
    panes.forEach(function(pane) {
      var sessionID = pane.dataset.sessionId;
      var containerID = 'term-' + sessionID;
      // Always reconnect — old instance was cleaned up in beforeSwap
      if (terminals[sessionID]) {
        disconnectSession(sessionID);
      }
      connectSession(sessionID, containerID);
    });

    // If a terminal was loaded into the terminal area, refresh sidebar to update states
    if (event.detail.target.id === 'terminal-area' && panes.length > 0) {
      htmx.ajax('GET', '/sidebar', { target: '#sidebar', swap: 'innerHTML' });
    }
  });

  // Close new session modal after successful form submission
  document.addEventListener('htmx:afterRequest', function(event) {
    var form = event.detail.elt;
    if (form && form.id === 'new-session-form' && event.detail.successful) {
      var modal = form.closest('.modal-overlay');
      if (modal) modal.remove();
      // Auto-open the newly created session
      var xhr = event.detail.xhr;
      if (xhr) {
        var sessionID = xhr.getResponseHeader('X-Session-ID');
        var sessionName = xhr.getResponseHeader('X-Session-Name');
        if (sessionID) {
          openTab(sessionID, sessionName || sessionID, 'running');
        }
      }
    }
  });

  var dirDebounce = null;
  function dirAutocomplete(input) {
    clearTimeout(dirDebounce);
    dirDebounce = setTimeout(function() {
      var q = input.value;
      if (!q) return;
      fetch('/api/dirs?q=' + encodeURIComponent(q))
        .then(function(r) { return r.json(); })
        .then(function(dirs) {
          var box = document.getElementById('dir-suggestions');
          if (!box) return;
          while (box.firstChild) box.removeChild(box.firstChild);
          if (!dirs || dirs.length === 0) return;
          dirs.forEach(function(d) {
            var div = document.createElement('div');
            div.className = 'dir-suggestion';
            // Show folder name highlighted, full path muted
            var name = d.split('/').pop();
            var nameSpan = document.createElement('span');
            nameSpan.className = 'dir-name';
            nameSpan.textContent = name;
            var pathSpan = document.createElement('span');
            pathSpan.className = 'dir-path';
            pathSpan.textContent = d;
            div.appendChild(nameSpan);
            div.appendChild(pathSpan);
            // Single click = select this dir and close
            div.addEventListener('click', function(e) {
              e.stopPropagation();
              selectDir(d, false);
            });
            // Double click = drill into subdirectories
            div.addEventListener('dblclick', function(e) {
              e.stopPropagation();
              selectDir(d, true);
            });
            box.appendChild(div);
          });
        });
    }, 200);
  }

  function selectDir(path, drillDown) {
    var input = document.getElementById('work_dir');
    if (input) input.value = path;
    var box = document.getElementById('dir-suggestions');
    if (box) { while (box.firstChild) box.removeChild(box.firstChild); }
    if (drillDown && input) {
      input.value = path + '/';
      dirAutocomplete(input);
    } else {
      var nameInput = document.getElementById('name');
      if (nameInput && !nameInput.value) {
        nameInput.value = path.split('/').pop();
      }
      if (nameInput) nameInput.focus();
      // Load claude sessions for the selected directory
      loadClaudeSessions(path);
    }
  }

  function closeDirSuggestions() {
    var box = document.getElementById('dir-suggestions');
    if (box) { while (box.firstChild) box.removeChild(box.firstChild); }
  }

  // Close suggestions when clicking outside
  document.addEventListener('click', function(e) {
    if (!e.target.closest('.form-group')) { closeDirSuggestions(); }
  });

  // Rename session — double-click on pane title
  function startRename(titleEl) {
    var sessionID = titleEl.getAttribute('data-session-id');
    var currentName = titleEl.textContent.trim();
    var input = document.createElement('input');
    input.type = 'text';
    input.value = currentName;
    input.className = 'rename-input';
    input.addEventListener('keydown', function(e) {
      if (e.key === 'Enter') { finishRename(titleEl, input, sessionID); }
      if (e.key === 'Escape') { cancelRename(titleEl, input, currentName); }
    });
    input.addEventListener('blur', function() { finishRename(titleEl, input, sessionID); });
    titleEl.textContent = '';
    titleEl.appendChild(input);
    input.focus();
    input.select();
  }

  function finishRename(titleEl, input, sessionID) {
    var newName = input.value.trim();
    if (!newName) newName = sessionID;
    titleEl.textContent = newName;
    // Persist rename to server
    fetch('/sessions/' + encodeURIComponent(sessionID) + '/rename', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: 'name=' + encodeURIComponent(newName),
    }).then(function() {
      // Refresh sidebar to show new name
      htmx.ajax('GET', '/sidebar', { target: '#sidebar', swap: 'innerHTML' });
    });
  }

  function cancelRename(titleEl, input, originalName) {
    titleEl.textContent = originalName;
  }

  // Populate form fields from a recent project selection
  function quickSession(btn) {
    var dir = btn.getAttribute('data-dir');
    var name = btn.getAttribute('data-name');
    var nameInput = document.getElementById('name');
    var dirInput = document.getElementById('work_dir');
    var promptInput = document.getElementById('prompt');
    var resumeInput = document.getElementById('resume_id');
    if (dirInput) dirInput.value = dir;
    if (nameInput) { nameInput.value = name; nameInput.focus(); nameInput.select(); }
    if (promptInput) promptInput.value = '';
    if (resumeInput) resumeInput.value = '';
    // Load claude sessions for this directory
    loadClaudeSessions(dir);
    // Scroll to form
    var form = document.getElementById('new-session-form');
    if (form) form.scrollIntoView({ behavior: 'smooth' });
  }

  // Tab management
  function openTab(sessionID, name, state) {
    // Add to tabs if not already open
    var existing = openTabs.find(function(t) { return t.id === sessionID; });
    if (!existing) {
      openTabs.push({ id: sessionID, name: name || sessionID, state: state || 'running' });
      saveTabState();
    }
    activeTabId = sessionID;
    renderTabs();
    // Load terminal
    htmx.ajax('POST', '/sessions/' + encodeURIComponent(sessionID) + '/open', {
      target: '#terminal-area',
      swap: 'innerHTML'
    });
  }

  function closeTab(sessionID, e) {
    if (e) { e.stopPropagation(); e.preventDefault(); }
    openTabs = openTabs.filter(function(t) { return t.id !== sessionID; });
    if (terminals[sessionID]) disconnectSession(sessionID);
    saveTabState();
    if (activeTabId === sessionID) {
      if (openTabs.length > 0) {
        openTab(openTabs[openTabs.length - 1].id, openTabs[openTabs.length - 1].name, openTabs[openTabs.length - 1].state);
      } else {
        activeTabId = null;
        renderTabs();
        var area = document.getElementById('terminal-area');
        if (area) {
          while (area.firstChild) area.removeChild(area.firstChild);
          var empty = document.createElement('div');
          empty.className = 'empty-state';
          var p = document.createElement('p');
          p.textContent = 'Select a session from the sidebar or create a new one';
          empty.appendChild(p);
          area.appendChild(empty);
        }
      }
    } else {
      renderTabs();
    }
  }

  function renderTabs() {
    var bar = document.getElementById('tab-bar');
    if (!bar) return;
    while (bar.firstChild) bar.removeChild(bar.firstChild);
    openTabs.forEach(function(tab) {
      var btn = document.createElement('div');
      btn.className = 'tab' + (tab.id === activeTabId ? ' tab-active' : '');
      btn.setAttribute('data-tab-id', tab.id);
      btn.setAttribute('draggable', 'true');
      btn.addEventListener('click', function() { openTab(tab.id, tab.name, tab.state); });
      btn.addEventListener('dragstart', function(e) { tabDragStart(e, tab.id); });
      btn.addEventListener('dragover', function(e) { tabDragOver(e); });
      btn.addEventListener('drop', function(e) { tabDrop(e, tab.id); });
      btn.addEventListener('dragend', tabDragEnd);

      var dot = document.createElement('span');
      dot.className = 'tab-dot state-' + (tab.state || 'running');
      btn.appendChild(dot);

      var nameSpan = document.createElement('span');
      nameSpan.textContent = tab.name;
      btn.appendChild(nameSpan);

      var closeSpan = document.createElement('span');
      closeSpan.className = 'tab-close';
      closeSpan.textContent = '\u00d7';
      closeSpan.title = 'Close tab (session keeps running)';
      closeSpan.addEventListener('click', function(e) { closeTab(tab.id, e); });
      btn.appendChild(closeSpan);

      // Right-click context menu
      btn.addEventListener('contextmenu', function(e) {
        e.preventDefault();
        showTabContextMenu(e, tab.id, tab.name);
      });

      bar.appendChild(btn);
    });

    // Add "+" button
    var newBtn = document.createElement('div');
    newBtn.className = 'tab tab-new';
    newBtn.textContent = '+';
    newBtn.addEventListener('click', function() {
      htmx.ajax('GET', '/sessions/new', { target: '#modal', swap: 'innerHTML' });
    });
    bar.appendChild(newBtn);
  }

  // Tab drag and drop (reorder tabs + drop-to-split on terminal area)
  var draggedTabId = null;

  function tabDragStart(e, tabId) {
    draggedTabId = tabId;
    e.target.classList.add('dragging');
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', tabId);
    // Show drop zones on terminal area after a short delay
    setTimeout(function() { showDropZones(); }, 100);
  }

  function tabDragOver(e) { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; }

  function tabDrop(e, targetId) {
    e.preventDefault();
    hideDropZones();
    if (!draggedTabId || draggedTabId === targetId) return;
    var fromIdx = openTabs.findIndex(function(t) { return t.id === draggedTabId; });
    var toIdx = openTabs.findIndex(function(t) { return t.id === targetId; });
    if (fromIdx < 0 || toIdx < 0) return;
    var item = openTabs.splice(fromIdx, 1)[0];
    openTabs.splice(toIdx, 0, item);
    saveTabState();
    renderTabs();
  }

  function tabDragEnd(e) {
    e.target.classList.remove('dragging');
    draggedTabId = null;
    hideDropZones();
  }

  // Drop zones on terminal area for split-by-drag
  function showDropZones() {
    var area = document.getElementById('terminal-area');
    if (!area || !draggedTabId) return;
    // Don't show if no active session to split with
    if (!activeTabId || activeTabId === draggedTabId) return;

    var zones = document.createElement('div');
    zones.id = 'drop-zones';
    zones.className = 'drop-zones';

    ['left', 'right', 'top', 'bottom'].forEach(function(side) {
      var zone = document.createElement('div');
      zone.className = 'drop-zone drop-zone-' + side;
      zone.addEventListener('dragover', function(e) {
        e.preventDefault();
        e.stopPropagation();
        e.dataTransfer.dropEffect = 'move';
        zone.classList.add('drop-zone-active');
      });
      zone.addEventListener('dragleave', function() {
        zone.classList.remove('drop-zone-active');
      });
      zone.addEventListener('drop', function(e) {
        e.preventDefault();
        e.stopPropagation();
        hideDropZones();
        if (!draggedTabId || draggedTabId === activeTabId) return;
        var direction = (side === 'left' || side === 'right') ? 'horizontal' : 'vertical';
        // For left/top, the dragged tab goes first; for right/bottom, it goes second
        if (side === 'left' || side === 'top') {
          doSplit(draggedTabId, activeTabId, direction);
        } else {
          doSplit(activeTabId, draggedTabId, direction);
        }
        draggedTabId = null;
      });
      zones.appendChild(zone);
    });

    area.style.position = 'relative';
    area.appendChild(zones);
  }

  function hideDropZones() {
    var zones = document.getElementById('drop-zones');
    if (zones) zones.remove();
  }

  function saveTabState() {
    try { localStorage.setItem('ws-open-tabs', JSON.stringify(openTabs)); } catch(e) {}
    try { localStorage.setItem('ws-active-tab', activeTabId || ''); } catch(e) {}
  }

  function loadTabState() {
    try {
      var saved = JSON.parse(localStorage.getItem('ws-open-tabs'));
      if (saved && saved.length) openTabs = saved;
      activeTabId = localStorage.getItem('ws-active-tab') || null;
    } catch(e) {}
  }

  // Load tabs on page load
  loadTabState();
  document.addEventListener('DOMContentLoaded', function() {
    renderTabs();
    // Reopen the active tab
    if (activeTabId) {
      var tab = openTabs.find(function(t) { return t.id === activeTabId; });
      if (tab) {
        htmx.ajax('POST', '/sessions/' + encodeURIComponent(activeTabId) + '/open', {
          target: '#terminal-area',
          swap: 'innerHTML'
        });
      }
    }
  });

  // Quick terminal creation
  function openTerminal() {
    var form = new FormData();
    form.append('work_dir', '~');
    fetch('/sessions/terminal', { method: 'POST', body: form })
      .then(function(r) {
        var sid = r.headers.get('X-Session-ID');
        var sname = r.headers.get('X-Session-Name');
        if (sid) {
          openTab(sid, sname || 'terminal', 'running');
        }
        htmx.ajax('GET', '/sidebar', { target: '#sidebar', swap: 'innerHTML' });
        return r.text(); // consume body
      });
  }

  function killSession(sessionID) {
    if (!confirm('Kill session "' + sessionID + '"?')) return;
    fetch('/sessions/' + encodeURIComponent(sessionID) + '/kill', { method: 'POST' })
      .then(function(r) {
        if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
        closeTab(sessionID);
        // Small delay to let mgr.Wait() finish removing from active list
        setTimeout(function() {
          htmx.ajax('GET', '/sidebar', { target: '#sidebar', swap: 'innerHTML' });
        }, 500);
      })
      .catch(function(err) { console.error('kill failed:', err); });
  }

  // Close a split pane without killing the session
  function unsplitPane(sessionID) {
    var area = document.getElementById('terminal-area');
    if (!area) return;

    var panes = area.querySelectorAll('.split-pane');
    // If not in a split layout, just close the tab
    if (panes.length < 2) {
      closeTab(sessionID);
      return;
    }

    // Find the other pane(s) — keep those, remove this one
    var keepSessionID = null;
    panes.forEach(function(pane) {
      var termPane = pane.querySelector('.terminal-pane[data-session-id]');
      if (termPane) {
        var sid = termPane.getAttribute('data-session-id');
        if (sid === sessionID) {
          // Disconnect this terminal
          disconnectSession(sid);
        } else {
          keepSessionID = sid;
        }
      }
    });

    // Rebuild area with just the remaining session
    if (keepSessionID) {
      // Clear everything and reload the remaining session
      area.style.flexDirection = '';
      htmx.ajax('POST', '/sessions/' + encodeURIComponent(keepSessionID) + '/open', {
        target: '#terminal-area',
        swap: 'innerHTML'
      });
    }
  }

  // Git diff viewer
  function showDiff(sessionID) {
    fetch('/sessions/' + encodeURIComponent(sessionID) + '/diff')
      .then(function(r) { return r.json(); })
      .then(function(data) {
        var overlay = document.createElement('div');
        overlay.className = 'modal-overlay';
        overlay.addEventListener('click', function() { overlay.remove(); });

        var content = document.createElement('div');
        content.className = 'modal-content diff-modal';
        content.addEventListener('click', function(e) { e.stopPropagation(); });

        var header = document.createElement('div');
        header.className = 'diff-header';

        var title = document.createElement('h2');
        title.textContent = 'Changes in ' + data.work_dir.split('/').pop();
        header.appendChild(title);

        var closeBtn = document.createElement('button');
        closeBtn.className = 'btn-cancel';
        closeBtn.textContent = 'Close';
        closeBtn.addEventListener('click', function() { overlay.remove(); });
        header.appendChild(closeBtn);

        content.appendChild(header);

        // File list summary
        if (data.files && data.files.length > 0) {
          var summary = document.createElement('div');
          summary.className = 'diff-summary';
          data.files.forEach(function(f) {
            var fileDiv = document.createElement('div');
            fileDiv.className = 'diff-file-entry';
            var status = f.substring(0, 2).trim();
            var fname = f.substring(3);
            var statusSpan = document.createElement('span');
            statusSpan.className = 'diff-file-status diff-status-' + status.charAt(0).toLowerCase();
            statusSpan.textContent = status;
            var nameSpan = document.createElement('span');
            nameSpan.textContent = fname;
            fileDiv.appendChild(statusSpan);
            fileDiv.appendChild(nameSpan);
            summary.appendChild(fileDiv);
          });
          content.appendChild(summary);
        }

        // Diff output
        if (data.diff) {
          var diffContainer = document.createElement('div');
          diffContainer.className = 'diff-content';
          renderDiff(diffContainer, data.diff);
          content.appendChild(diffContainer);
        } else {
          var noChanges = document.createElement('p');
          noChanges.className = 'diff-empty';
          noChanges.textContent = 'No changes detected';
          content.appendChild(noChanges);
        }

        overlay.appendChild(content);
        document.body.appendChild(overlay);
      });
  }

  function renderDiff(container, diffText) {
    var lines = diffText.split('\n');
    var pre = document.createElement('pre');
    pre.className = 'diff-pre';

    lines.forEach(function(line) {
      var span = document.createElement('div');
      span.className = 'diff-line';
      if (line.startsWith('+++') || line.startsWith('---')) {
        span.className += ' diff-line-file';
      } else if (line.startsWith('@@')) {
        span.className += ' diff-line-hunk';
      } else if (line.startsWith('+')) {
        span.className += ' diff-line-add';
      } else if (line.startsWith('-')) {
        span.className += ' diff-line-del';
      } else if (line.startsWith('diff ')) {
        span.className += ' diff-line-header';
      }
      span.textContent = line;
      pre.appendChild(span);
    });

    container.appendChild(pre);
  }

  // Update check
  function checkForUpdate() {
    var feedback = document.getElementById('update-feedback');
    var status = document.getElementById('update-status');
    if (feedback) { feedback.textContent = 'Checking...'; feedback.className = 'hooks-feedback'; }

    fetch('/api/check-update')
      .then(function(r) { return r.json(); })
      .then(function(data) {
        if (data.error) {
          if (feedback) { feedback.textContent = 'Error: ' + data.error; feedback.className = 'hooks-feedback hooks-feedback-err'; }
          return;
        }
        if (!data.UpdateAvail) {
          if (feedback) { feedback.textContent = 'You are on the latest version (' + data.LatestVersion + ')'; feedback.className = 'hooks-feedback hooks-feedback-ok'; }
          return;
        }
        // Update available — show details and update button
        if (feedback) { feedback.className = ''; }
        if (status) {
          while (status.firstChild) status.removeChild(status.firstChild);

          var info = document.createElement('div');
          info.className = 'update-available';

          var badge = document.createElement('span');
          badge.className = 'hooks-badge hooks-active';
          badge.textContent = data.LatestVersion + ' available';
          info.appendChild(badge);

          var btn = document.createElement('button');
          btn.type = 'button';
          btn.className = 'btn-create btn-small';
          btn.textContent = 'Update now';
          btn.addEventListener('click', function() { selfUpdate(feedback); });
          info.appendChild(btn);

          var link = document.createElement('a');
          link.href = data.ReleaseURL;
          link.target = '_blank';
          link.className = 'update-release-link';
          link.textContent = 'Release notes';
          info.appendChild(link);

          status.appendChild(info);
        }
      })
      .catch(function(err) {
        if (feedback) { feedback.textContent = 'Error: ' + err.message; feedback.className = 'hooks-feedback hooks-feedback-err'; }
      });
  }

  function selfUpdate(feedback) {
    if (feedback) { feedback.textContent = 'Downloading update...'; feedback.className = 'hooks-feedback'; }

    fetch('/api/self-update', { method: 'POST' })
      .then(function(r) { return r.json(); })
      .then(function(data) {
        if (data.error) {
          if (feedback) { feedback.textContent = 'Error: ' + data.error; feedback.className = 'hooks-feedback hooks-feedback-err'; }
          return;
        }
        if (feedback) {
          feedback.textContent = data.message;
          feedback.className = 'hooks-feedback hooks-feedback-ok';
        }
      })
      .catch(function(err) {
        if (feedback) { feedback.textContent = 'Error: ' + err.message; feedback.className = 'hooks-feedback hooks-feedback-err'; }
      });
  }

  // Background service management (systemd on Linux, launchd on macOS)
  function manageService(action) {
    var feedback = document.getElementById('service-feedback');
    if (feedback) { feedback.textContent = 'Processing...'; feedback.className = 'hooks-feedback'; }

    fetch('/settings/service', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: action }),
    })
    .then(function(r) { return r.json().then(function(d) { return { ok: r.ok, data: d }; }); })
    .then(function(result) {
      if (feedback) {
        if (result.ok) {
          feedback.textContent = 'Done: ' + result.data.status;
          feedback.className = 'hooks-feedback hooks-feedback-ok';
        } else {
          feedback.textContent = 'Error: ' + (result.data.error || 'Unknown');
          feedback.className = 'hooks-feedback hooks-feedback-err';
        }
        setTimeout(function() { feedback.textContent = ''; feedback.className = ''; }, 4000);
      }
      // Reload settings page to update buttons
      setTimeout(function() { window.location.reload(); }, 500);
    })
    .catch(function(err) {
      if (feedback) {
        feedback.textContent = 'Error: ' + err.message;
        feedback.className = 'hooks-feedback hooks-feedback-err';
      }
    });
  }

  // Load claude sessions for a directory
  function loadClaudeSessions(dir) {
    if (!dir) return;
    var section = document.getElementById('claude-sessions-section');
    var list = document.getElementById('claude-sessions-list');
    if (!section || !list) return;

    fetch('/api/claude-sessions?dir=' + encodeURIComponent(dir))
      .then(function(r) { return r.json(); })
      .then(function(sessions) {
        while (list.firstChild) list.removeChild(list.firstChild);
        if (!sessions || sessions.length === 0) {
          section.style.display = 'none';
          return;
        }
        section.style.display = 'block';
        sessions.forEach(function(s) {
          var div = document.createElement('div');
          div.className = 'recent-item claude-session-item';
          div.setAttribute('role', 'button');
          div.setAttribute('tabindex', '0');

          var nameSpan = document.createElement('span');
          nameSpan.className = 'recent-name';
          nameSpan.textContent = s.summary || s.id.substring(0, 8) + '...';

          var metaSpan = document.createElement('span');
          metaSpan.className = 'recent-path';
          metaSpan.textContent = s.date + ' · ' + s.size_kb + 'KB';

          div.appendChild(nameSpan);
          div.appendChild(metaSpan);

          div.addEventListener('click', function() {
            // Set resume ID in hidden field
            var resumeInput = document.getElementById('resume_id');
            if (resumeInput) resumeInput.value = s.id;
            // Highlight selected
            list.querySelectorAll('.claude-session-item').forEach(function(el) {
              el.classList.remove('selected');
            });
            div.classList.add('selected');
          });
          list.appendChild(div);
        });
      });
  }

  // Hooks management
  function manageHooks(action) {
    var feedback = document.getElementById('hooks-feedback');
    if (feedback) { feedback.textContent = action === 'install' ? 'Installing...' : 'Uninstalling...'; feedback.className = 'hooks-feedback'; }

    fetch('/settings/hooks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: action }),
    })
    .then(function(r) { return r.json().then(function(d) { return { ok: r.ok, data: d }; }); })
    .then(function(result) {
      var status = document.getElementById('hooks-status');
      if (!status) return;
      while (status.firstChild) status.removeChild(status.firstChild);

      if (result.ok && result.data.installed) {
        var badge = document.createElement('span');
        badge.className = 'hooks-badge hooks-active';
        badge.textContent = 'Installed';
        status.appendChild(badge);

        var uninstBtn = document.createElement('button');
        uninstBtn.type = 'button';
        uninstBtn.className = 'btn-cancel btn-small';
        uninstBtn.textContent = 'Uninstall';
        uninstBtn.addEventListener('click', function() { manageHooks('uninstall'); });
        status.appendChild(uninstBtn);

      } else {
        var badge2 = document.createElement('span');
        badge2.className = 'hooks-badge hooks-inactive';
        badge2.textContent = 'Not installed';
        status.appendChild(badge2);

        var instBtn = document.createElement('button');
        instBtn.type = 'button';
        instBtn.className = 'btn-create btn-small';
        instBtn.textContent = 'Install Hooks';
        instBtn.addEventListener('click', function() { manageHooks('install'); });
        status.appendChild(instBtn);
      }

      if (feedback) {
        if (result.ok) {
          feedback.textContent = action === 'install' ? 'Hooks installed successfully' : 'Hooks removed successfully';
          feedback.className = 'hooks-feedback hooks-feedback-ok';
        } else {
          feedback.textContent = 'Error: ' + (result.data.error || 'Unknown error');
          feedback.className = 'hooks-feedback hooks-feedback-err';
        }
        setTimeout(function() { feedback.textContent = ''; feedback.className = ''; }, 4000);
      }
    })
    .catch(function(err) {
      if (feedback) {
        feedback.textContent = 'Error: ' + err.message;
        feedback.className = 'hooks-feedback hooks-feedback-err';
      }
    });
  }

  // Settings page directory picker
  var settingsDirDebounce = null;
  function settingsDirAutocomplete(input) {
    clearTimeout(settingsDirDebounce);
    settingsDirDebounce = setTimeout(function() {
      var q = input.value;
      if (!q) return;
      fetch('/api/dirs?q=' + encodeURIComponent(q))
        .then(function(r) { return r.json(); })
        .then(function(dirs) {
          var box = document.getElementById('settings-dir-suggestions');
          if (!box) return;
          while (box.firstChild) box.removeChild(box.firstChild);
          if (!dirs || dirs.length === 0) return;
          dirs.forEach(function(d) {
            var div = document.createElement('div');
            div.className = 'dir-suggestion';
            var nameSpan = document.createElement('span');
            nameSpan.className = 'dir-name';
            nameSpan.textContent = d.split('/').pop();
            var pathSpan = document.createElement('span');
            pathSpan.className = 'dir-path';
            pathSpan.textContent = d;
            div.appendChild(nameSpan);
            div.appendChild(pathSpan);
            div.addEventListener('click', function(e) {
              e.stopPropagation();
              input.value = d;
              while (box.firstChild) box.removeChild(box.firstChild);
            });
            div.addEventListener('dblclick', function(e) {
              e.stopPropagation();
              input.value = d + '/';
              settingsDirAutocomplete(input);
            });
            box.appendChild(div);
          });
        });
    }, 200);
  }

  // Tab context menu
  function showTabContextMenu(e, tabId, tabName) {
    closeTabContextMenu();
    var menu = document.createElement('div');
    menu.id = 'tab-context-menu';
    menu.className = 'tab-context-menu';
    menu.style.left = e.clientX + 'px';
    menu.style.top = e.clientY + 'px';

    var items = [
      { label: 'Close tab', action: function() { closeTab(tabId); } },
      { label: 'Close & stop session', cls: 'ctx-danger', action: function() { killSession(tabId); } },
      { type: 'separator' },
      { label: 'Close other tabs', action: function() {
        var keep = openTabs.filter(function(t) { return t.id === tabId; });
        openTabs.forEach(function(t) { if (t.id !== tabId && terminals[t.id]) disconnectSession(t.id); });
        openTabs = keep;
        activeTabId = tabId;
        saveTabState();
        renderTabs();
        openTab(tabId, tabName, 'running');
      }},
      { label: 'Close all tabs', action: function() {
        openTabs.forEach(function(t) { if (terminals[t.id]) disconnectSession(t.id); });
        openTabs = [];
        activeTabId = null;
        saveTabState();
        renderTabs();
        var area = document.getElementById('terminal-area');
        if (area) {
          while (area.firstChild) area.removeChild(area.firstChild);
          var empty = document.createElement('div');
          empty.className = 'empty-state';
          var p = document.createElement('p');
          p.textContent = 'Select a session from the sidebar or create a new one';
          empty.appendChild(p);
          area.appendChild(empty);
        }
      }},
    ];

    items.forEach(function(item) {
      if (item.type === 'separator') {
        var sep = document.createElement('div');
        sep.className = 'ctx-separator';
        menu.appendChild(sep);
        return;
      }
      var el = document.createElement('div');
      el.className = 'ctx-item' + (item.cls ? ' ' + item.cls : '');
      el.textContent = item.label;
      el.addEventListener('click', function(ev) {
        ev.stopPropagation();
        closeTabContextMenu();
        item.action();
      });
      menu.appendChild(el);
    });

    document.body.appendChild(menu);

    // Close on next click anywhere
    setTimeout(function() {
      document.addEventListener('click', closeTabContextMenu, { once: true });
    }, 0);
  }

  function closeTabContextMenu() {
    var menu = document.getElementById('tab-context-menu');
    if (menu) menu.remove();
  }

  // Focus a session from a notification click
  function focusNotifSession(sessionID, sessionName, eventType) {
    toggleNotifications(); // close the dropdown

    // For active sessions (waiting), open the tab normally
    if (eventType === 'waiting') {
      openTab(sessionID, sessionName, eventType);
      return;
    }

    // For finished sessions, check if it's still active
    fetch('/api/sessions')
      .then(function(r) { return r.json(); })
      .then(function(sessions) {
        var active = sessions && sessions.find(function(s) { return s.id === sessionID; });
        if (active) {
          // Still in active manager — open normally
          openTab(sessionID, sessionName, active.state);
        } else {
          // Not active — switch to History tab and highlight
          var historyBtn = document.querySelector('.sidebar-tab:nth-child(2)');
          if (historyBtn) switchSidebarTab('history', historyBtn);
          // Scroll to the session in history if visible
          setTimeout(function() {
            var items = document.querySelectorAll('.history-list .session-item');
            items.forEach(function(item) {
              // Flash highlight matching session
              var nameEl = item.querySelector('.session-name');
              if (nameEl && nameEl.textContent.trim() === sessionName) {
                item.scrollIntoView({ behavior: 'smooth', block: 'center' });
                item.classList.add('notif-highlight');
                setTimeout(function() { item.classList.remove('notif-highlight'); }, 2000);
              }
            });
          }, 200);
        }
      })
      .catch(function() {
        // Fallback — try opening anyway
        openTab(sessionID, sessionName, eventType);
      });
  }

  // Notification panel
  var notifOpen = false;
  function toggleNotifications() {
    var dropdown = document.getElementById('notification-dropdown');
    if (!dropdown) return;
    if (notifOpen) {
      while (dropdown.firstChild) dropdown.removeChild(dropdown.firstChild);
      dropdown.classList.remove('dropdown-open');
      notifOpen = false;
    } else {
      dropdown.classList.add('dropdown-open');
      notifOpen = true;
      htmx.ajax('GET', '/notifications', { target: '#notification-dropdown', swap: 'innerHTML' });
    }
  }

  function snoozeNotification(id, sessionID) {
    fetch('/notifications/snooze', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ session_id: sessionID, minutes: 15 }),
    }).then(function() {
      // Dismiss the notification from the panel
      dismissNotification(id);
    });
  }

  function dismissNotification(id) {
    fetch('/notifications/' + id + '/read', { method: 'POST' })
      .then(function() {
        var el = document.getElementById('notif-' + id);
        if (el) el.remove();
        updateBadgeCount(-1);
        // If no more items, show empty state
        var body = document.querySelector('.notif-panel-body');
        if (body && !body.querySelector('.notif-item')) {
          while (body.firstChild) body.removeChild(body.firstChild);
          var empty = document.createElement('div');
          empty.className = 'notif-empty';
          var span = document.createElement('span');
          span.textContent = 'No notifications';
          empty.appendChild(span);
          body.appendChild(empty);
          // Hide clear all button
          var clearBtn = document.querySelector('.notif-clear-all');
          if (clearBtn) clearBtn.style.display = 'none';
        }
      });
  }

  function clearAllNotifications() {
    fetch('/notifications/clear', { method: 'POST' })
      .then(function() {
        var dropdown = document.getElementById('notification-dropdown');
        if (dropdown) {
          htmx.ajax('GET', '/notifications', { target: '#notification-dropdown', swap: 'innerHTML' });
        }
        // Reset badge
        var badge = document.getElementById('notif-badge');
        if (badge) badge.remove();
      });
  }

  function updateBadgeCount(delta) {
    var badge = document.getElementById('notif-badge');
    if (!badge) return;
    var count = parseInt(badge.textContent || '0') + delta;
    if (count <= 0) {
      badge.remove();
    } else {
      badge.textContent = String(count);
    }
  }

  // Close notification panel when clicking outside
  document.addEventListener('click', function(e) {
    if (notifOpen && !e.target.closest('.notification-wrapper')) {
      var dropdown = document.getElementById('notification-dropdown');
      if (dropdown) {
        while (dropdown.firstChild) dropdown.removeChild(dropdown.firstChild);
        dropdown.classList.remove('dropdown-open');
      }
      notifOpen = false;
    }
  });

  // Sidebar tab switching
  function switchSidebarTab(tabName, btn) {
    // Toggle tab buttons
    var tabs = document.querySelectorAll('.sidebar-tab');
    tabs.forEach(function(t) { t.classList.remove('sidebar-tab-active'); });
    if (btn) btn.classList.add('sidebar-tab-active');
    // Toggle panels
    document.querySelectorAll('.sidebar-panel').forEach(function(p) { p.classList.remove('sidebar-panel-visible'); });
    var panel = document.getElementById('sidebar-' + tabName);
    if (panel) panel.classList.add('sidebar-panel-visible');
    // Remember which tab is active
    try { localStorage.setItem('ws-sidebar-tab', tabName); } catch(e) {}
  }

  // Restore sidebar tab on load/refresh
  function restoreSidebarTab() {
    var saved = null;
    try { saved = localStorage.getItem('ws-sidebar-tab'); } catch(e) {}
    if (saved && saved !== 'active') {
      var btn = document.querySelector('.sidebar-tab:nth-child(2)');
      switchSidebarTab(saved, btn);
    }
  }

  // Drag and drop reordering
  var draggedEl = null;

  function dragStart(e) {
    draggedEl = e.target.closest('.session-item');
    if (!draggedEl) return;
    draggedEl.classList.add('dragging');
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', ''); // required for Firefox
  }

  function dragOver(e) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    var target = e.target.closest('.session-item');
    if (!target || target === draggedEl) return;
    var list = target.parentNode;
    var items = Array.from(list.children);
    var dragIdx = items.indexOf(draggedEl);
    var targetIdx = items.indexOf(target);
    if (dragIdx < targetIdx) {
      list.insertBefore(draggedEl, target.nextSibling);
    } else {
      list.insertBefore(draggedEl, target);
    }
  }

  function drop(e) {
    e.preventDefault();
    saveSessionOrder();
  }

  function dragEnd(e) {
    if (draggedEl) {
      draggedEl.classList.remove('dragging');
      draggedEl = null;
    }
  }

  function saveSessionOrder() {
    var list = document.getElementById('session-list');
    if (!list) return;
    var order = [];
    list.querySelectorAll('.session-item[data-session-id]').forEach(function(el) {
      order.push(el.getAttribute('data-session-id'));
    });
    try { localStorage.setItem('ws-session-order', JSON.stringify(order)); } catch(e) {}
  }

  function applySessionOrder() {
    var list = document.getElementById('session-list');
    if (!list) return;
    var order;
    try { order = JSON.parse(localStorage.getItem('ws-session-order')); } catch(e) { return; }
    if (!order || !order.length) return;
    var items = {};
    list.querySelectorAll('.session-item[data-session-id]').forEach(function(el) {
      items[el.getAttribute('data-session-id')] = el;
    });
    // Re-insert items in saved order; items not in saved order stay at end
    order.forEach(function(id) {
      if (items[id]) {
        list.appendChild(items[id]);
        delete items[id];
      }
    });
    // Append remaining items not in the saved order
    Object.keys(items).forEach(function(id) {
      list.appendChild(items[id]);
    });
  }

  // Apply saved order after sidebar loads/refreshes
  var origAfterSwap = null;
  document.addEventListener('htmx:afterSettle', function(event) {
    if (event.detail.target.id === 'sidebar' || event.detail.target.querySelector('#session-list')) {
      applySessionOrder();
      restoreSidebarTab();
    }
  });

  // Also apply on initial page load
  document.addEventListener('DOMContentLoaded', function() {
    applySessionOrder();
    restoreSidebarTab();
    // Restore notification sounds toggle on settings page
    var cb = document.getElementById('notif-sounds-toggle');
    if (cb) cb.checked = getNotifSoundsEnabled();
  });

  // Global notification WebSocket
  var notifWs = null;
  function connectNotifications() {
    var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    notifWs = new WebSocket(protocol + '//' + window.location.host + '/ws/notifications');
    notifWs.onmessage = function(event) {
      try {
        var msg = JSON.parse(event.data);
        if (msg.type === 'notification') {
          // Update badge
          var bell = document.querySelector('.notification-bell');
          var badge = bell ? bell.querySelector('.badge') : null;
          if (!badge && bell) {
            badge = document.createElement('span');
            badge.className = 'badge';
            badge.textContent = '0';
            bell.appendChild(badge);
          }
          if (badge) {
            badge.textContent = String(parseInt(badge.textContent || '0') + 1);
          }
          // Play sound
          playNotifSound(msg.event);
          // Desktop notification (or in-app toast fallback for webview/GUI mode)
          var notifTitle = 'websessions: ' + msg.event;
          var notifBody = 'Session ' + msg.sessionID + (msg.message ? ': ' + msg.message : '');
          if ('Notification' in window && Notification.permission === 'granted') {
            new Notification(notifTitle, {
              body: notifBody,
              tag: 'ws-' + msg.sessionID + '-' + msg.event,
            });
          } else {
            showToast(notifTitle, notifBody, msg.event);
          }
          // Refresh sidebar to update session states
          htmx.ajax('GET', '/sidebar', { target: '#sidebar', swap: 'innerHTML' });
        }
      } catch(e) {}
    };
    notifWs.onclose = function() {
      // Reconnect after 3 seconds
      setTimeout(connectNotifications, 3000);
    };
  }
  connectNotifications();

  // ── Periodic update check (every 30 minutes) ──────────────
  var updateBannerShown = false;
  function backgroundUpdateCheck() {
    if (updateBannerShown) return;
    fetch('/api/check-update')
      .then(function(r) { return r.json(); })
      .then(function(data) {
        if (data.UpdateAvail && !updateBannerShown) {
          updateBannerShown = true;
          showUpdateBanner(data.LatestVersion, data.ReleaseURL);
        }
      })
      .catch(function() {}); // silently ignore errors
  }

  function showUpdateBanner(version, url) {
    var existing = document.getElementById('update-banner');
    if (existing) return;

    var banner = document.createElement('div');
    banner.id = 'update-banner';
    banner.className = 'update-banner';

    var text = document.createElement('span');
    text.className = 'update-banner-text';
    text.textContent = 'A new version of websessions is available: ' + version;

    var actions = document.createElement('span');
    actions.className = 'update-banner-actions';

    var viewBtn = document.createElement('a');
    viewBtn.className = 'update-banner-btn';
    viewBtn.textContent = 'View release';
    viewBtn.href = url || 'https://github.com/IgorDeo/claude-websessions/releases/latest';
    viewBtn.target = '_blank';
    viewBtn.rel = 'noopener';

    var updateBtn = document.createElement('button');
    updateBtn.className = 'update-banner-btn update-banner-btn-primary';
    updateBtn.textContent = 'Update now';
    updateBtn.onclick = function() {
      updateBtn.textContent = 'Updating...';
      updateBtn.disabled = true;
      fetch('/api/self-update', { method: 'POST' })
        .then(function(r) { return r.json(); })
        .then(function(data) {
          if (data.error) {
            updateBtn.textContent = 'Failed: ' + data.error;
          } else {
            text.textContent = 'Updated to ' + version + '. Restart websessions to apply.';
            updateBtn.remove();
            viewBtn.remove();
          }
        })
        .catch(function(err) {
          updateBtn.textContent = 'Failed';
        });
    };

    var dismiss = document.createElement('button');
    dismiss.className = 'update-banner-dismiss';
    dismiss.textContent = '\u00d7';
    dismiss.title = 'Dismiss';
    dismiss.onclick = function() { banner.remove(); };

    actions.appendChild(viewBtn);
    actions.appendChild(updateBtn);
    banner.appendChild(text);
    banner.appendChild(actions);
    banner.appendChild(dismiss);
    document.body.insertBefore(banner, document.body.firstChild);
  }

  // Check on startup (after 10s delay) then every 30 minutes
  setTimeout(backgroundUpdateCheck, 10000);
  setInterval(backgroundUpdateCheck, 30 * 60 * 1000);

  return {
    connectSession: connectSession,
    disconnectSession: disconnectSession,
    openTab: openTab,
    closeTab: closeTab,
    showDiff: showDiff,
    unsplitPane: unsplitPane,
    splitPane: splitPane,
    openTerminal: openTerminal,
    killSession: killSession,
    startRename: startRename,
    dirAutocomplete: dirAutocomplete,
    focusNotifSession: focusNotifSession,
    toggleNotifications: toggleNotifications,
    setNotifSoundsEnabled: setNotifSoundsEnabled,
    getNotifSoundsEnabled: getNotifSoundsEnabled,
    testNotifSound: testNotifSound,
    dismissNotification: dismissNotification,
    snoozeNotification: snoozeNotification,
    clearAllNotifications: clearAllNotifications,
    switchSidebarTab: switchSidebarTab,
    manageHooks: manageHooks,
    checkForUpdate: checkForUpdate,
    manageService: manageService,
    settingsDirAutocomplete: settingsDirAutocomplete,
    loadClaudeSessions: loadClaudeSessions,
    selectDir: selectDir,
    quickSession: quickSession,
    dragStart: dragStart,
    dragOver: dragOver,
    drop: drop,
    dragEnd: dragEnd,
    terminals: terminals,
  };
})();
