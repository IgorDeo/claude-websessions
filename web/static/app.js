window.websessions = (function() {
  const terminals = {};
  const splitInstances = [];
  var openTabs = []; // [{id, name, state}]
  var activeTabId = null;

  function connectSession(sessionID, containerID) {
    const container = document.getElementById(containerID);
    if (!container) return;

    const term = new Terminal({
      cursorBlink: true,
      theme: {
        background: '#1a1b26',
        foreground: '#c0caf5',
        cursor: '#c0caf5',
        selectionBackground: '#33467c',
      },
      fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
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

  function handleNotification(msg) {
    var badge = document.querySelector('.badge');
    if (badge) {
      var count = parseInt(badge.textContent || '0') + 1;
      badge.textContent = count;
    }
    if ('Notification' in window && Notification.permission === 'granted') {
      new Notification('websessions: ' + msg.event, {
        body: 'Session ' + msg.sessionID + ': ' + msg.event,
        tag: 'ws-' + msg.sessionID + '-' + msg.event,
      });
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
      // Focus name field so user can proceed
      var nameInput = document.getElementById('name');
      if (nameInput && !nameInput.value) {
        // Auto-fill session name from directory name
        nameInput.value = path.split('/').pop();
      }
      if (nameInput) nameInput.focus();
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
      closeSpan.addEventListener('click', function(e) { closeTab(tab.id, e); });
      btn.appendChild(closeSpan);

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

  function killSession(sessionID) {
    if (!confirm('Kill session "' + sessionID + '"?')) return;
    fetch('/sessions/' + encodeURIComponent(sessionID) + '/kill', { method: 'POST' })
      .then(function(r) {
        if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
        closeTab(sessionID);
        htmx.ajax('GET', '/sidebar', { target: '#sidebar', swap: 'innerHTML' });
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
    }
  });

  // Also apply on initial page load
  document.addEventListener('DOMContentLoaded', function() { applySessionOrder(); });

  return {
    connectSession: connectSession,
    disconnectSession: disconnectSession,
    openTab: openTab,
    closeTab: closeTab,
    showDiff: showDiff,
    unsplitPane: unsplitPane,
    splitPane: splitPane,
    killSession: killSession,
    startRename: startRename,
    dirAutocomplete: dirAutocomplete,
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
