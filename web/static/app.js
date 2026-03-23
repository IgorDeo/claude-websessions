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

  function splitPane(sessionID, direction) {
    var area = document.getElementById('terminal-area');
    if (!area) return;
    htmx.ajax('POST', '/sessions/' + sessionID + '/split?direction=' + direction, {
      target: '#terminal-area',
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

  document.addEventListener('htmx:afterSwap', function(event) {
    var panes = event.detail.target.querySelectorAll('.terminal-pane[data-session-id]');
    panes.forEach(function(pane) {
      var sessionID = pane.dataset.sessionId;
      var containerID = 'term-' + sessionID;
      if (!terminals[sessionID]) {
        connectSession(sessionID, containerID);
      }
    });
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

  function quickSession(btn) {
    var dir = btn.getAttribute('data-dir');
    var name = btn.getAttribute('data-name');
    var form = new FormData();
    form.append('name', name);
    form.append('work_dir', dir);
    form.append('prompt', '');
    fetch('/sessions', { method: 'POST', body: form })
      .then(function(r) { return r.text(); })
      .then(function(html) {
        // Close modal
        var modal = btn.closest('.modal-overlay');
        if (modal) modal.remove();
        // Refresh sidebar
        htmx.ajax('GET', '/sidebar', { target: '#sidebar', swap: 'innerHTML' });
      });
  }

  return {
    connectSession: connectSession,
    disconnectSession: disconnectSession,
    splitPane: splitPane,
    dirAutocomplete: dirAutocomplete,
    selectDir: selectDir,
    quickSession: quickSession,
    terminals: terminals,
  };
})();
