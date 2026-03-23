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

  return {
    connectSession: connectSession,
    disconnectSession: disconnectSession,
    splitPane: splitPane,
    terminals: terminals,
  };
})();
