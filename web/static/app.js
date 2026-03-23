window.websessions = (function() {
  const terminals = {};
  const splitInstances = [];

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
      if (document.getElementById('term-' + sid) && area.contains(document.getElementById('term-' + sid))) {
        disconnectSession(sid);
      }
    }

    // Create two pane containers
    var pane1 = document.createElement('div');
    pane1.className = 'split-pane';
    pane1.id = 'split-' + sessionID1;
    var pane2 = document.createElement('div');
    pane2.className = 'split-pane';
    pane2.id = 'split-' + sessionID2;

    // Clear area and add panes
    while (area.firstChild) area.removeChild(area.firstChild);
    area.appendChild(pane1);
    area.appendChild(pane2);

    // Set flex direction based on split direction
    area.style.flexDirection = direction === 'horizontal' ? 'row' : 'column';

    // Use Split.js
    Split(['#split-' + sessionID1, '#split-' + sessionID2], {
      direction: direction === 'horizontal' ? 'horizontal' : 'vertical',
      sizes: [50, 50],
      minSize: 100,
      gutterSize: 4,
    });

    // Load terminal content into each pane via htmx
    htmx.ajax('POST', '/sessions/' + sessionID1 + '/open', {
      target: pane1,
      swap: 'innerHTML'
    });
    htmx.ajax('POST', '/sessions/' + sessionID2 + '/open', {
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
    if (dirInput) dirInput.value = dir;
    if (nameInput) { nameInput.value = name; nameInput.focus(); nameInput.select(); }
    if (promptInput) promptInput.value = '';
    // Scroll to form
    var form = document.getElementById('new-session-form');
    if (form) form.scrollIntoView({ behavior: 'smooth' });
  }

  return {
    connectSession: connectSession,
    disconnectSession: disconnectSession,
    splitPane: splitPane,
    startRename: startRename,
    dirAutocomplete: dirAutocomplete,
    selectDir: selectDir,
    quickSession: quickSession,
    terminals: terminals,
  };
})();
