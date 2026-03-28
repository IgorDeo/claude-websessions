// Metrics Dashboard — fetches /api/metrics/history and renders uPlot charts.
(function() {
  "use strict";

  var currentRange = "1h";
  var charts = {};
  var refreshTimer = null;

  var isDark = localStorage.getItem("theme") !== "light";

  function colors() {
    return isDark
      ? { bg: "#1a1c27", axes: "#8288a8", grid: "#323548", text: "#a8adc8" }
      : { bg: "#ffffff", axes: "#666", grid: "#e0e0e0", text: "#333" };
  }

  function toggleDashboardTheme() {
    isDark = !isDark;
    localStorage.setItem("theme", isDark ? "dark" : "light");
    document.body.classList.toggle("light-theme", !isDark);
    fetchAndRender();
  }
  window.toggleDashboardTheme = toggleDashboardTheme;

  function baseOpts(el) {
    var c = colors();
    return {
      width: el.clientWidth,
      height: 180,
      cursor: { show: true, drag: { x: false, y: false } },
      legend: { show: false },
      axes: [
        {
          stroke: c.axes,
          grid: { stroke: c.grid, width: 1 },
          ticks: { stroke: c.grid },
          font: "11px 'IBM Plex Mono', monospace",
          values: function(u, vals) {
            return vals.map(function(v) {
              var d = new Date(v * 1000);
              return d.getHours().toString().padStart(2, "0") + ":" +
                     d.getMinutes().toString().padStart(2, "0");
            });
          },
        },
        {
          stroke: c.axes,
          grid: { stroke: c.grid, width: 1 },
          ticks: { stroke: c.grid },
          font: "11px 'IBM Plex Mono', monospace",
          size: 50,
        },
      ],
    };
  }

  function makeSeries(label, color) {
    return { label: label, stroke: color, width: 2, fill: color + "18" };
  }

  function showEmpty(el) {
    var div = document.createElement("div");
    div.className = "chart-empty";
    div.textContent = "No data yet";
    el.appendChild(div);
  }

  function renderChart(id, metricKeys, data, seriesColors, transform) {
    var el = document.getElementById(id);
    if (!el) return;

    // Destroy existing chart and clear container
    if (charts[id]) {
      charts[id].destroy();
      delete charts[id];
    }
    while (el.firstChild) el.removeChild(el.firstChild);

    // Collect timestamps and build aligned series
    var timestamps = [];
    var tsSet = {};
    metricKeys.forEach(function(key) {
      var pts = data[key] || [];
      pts.forEach(function(p) {
        if (!tsSet[p.t]) { tsSet[p.t] = true; timestamps.push(p.t); }
      });
    });
    timestamps.sort(function(a, b) { return a - b; });

    if (timestamps.length === 0) {
      showEmpty(el);
      return;
    }

    // Build lookup maps
    var tsIndex = {};
    timestamps.forEach(function(t, i) { tsIndex[t] = i; });

    var uData = [new Float64Array(timestamps)];
    metricKeys.forEach(function(key) {
      var arr = new Float64Array(timestamps.length);
      var pts = data[key] || [];
      pts.forEach(function(p) {
        var val = transform ? transform(p.v) : p.v;
        arr[tsIndex[p.t]] = val;
      });
      uData.push(arr);
    });

    var opts = baseOpts(el);
    opts.series = [{}];
    metricKeys.forEach(function(key, i) {
      var label = key.replace("sessions_", "").replace("_", " ");
      opts.series.push(makeSeries(label, seriesColors[i % seriesColors.length]));
    });

    charts[id] = new uPlot(opts, uData, el);
  }

  function fetchAndRender() {
    var statusEl = document.getElementById("dashboard-status");
    if (statusEl) statusEl.textContent = "Loading...";

    fetch("/api/metrics/history?range=" + currentRange)
      .then(function(r) { return r.json(); })
      .then(function(resp) {
        var m = resp.metrics || {};

        // Active Sessions
        renderChart("chart-sessions-active", ["sessions_active"], m, ["#6c8cff"]);

        // Sessions by State — find all sessions_* keys except sessions_active
        var stateKeys = Object.keys(m).filter(function(k) {
          return k.startsWith("sessions_") && k !== "sessions_active";
        }).sort();
        var stateColors = ["#7ec87e", "#d4a843", "#e87070", "#5eb5e0", "#c084fc", "#f59e0b"];
        renderChart("chart-sessions-state", stateKeys, m, stateColors);

        // Memory (convert bytes to MB)
        renderChart("chart-memory", ["memory_alloc_bytes"], m, ["#c084fc"], function(v) {
          return v / (1024 * 1024);
        });

        // Goroutines
        renderChart("chart-goroutines", ["goroutines"], m, ["#5eb5e0"]);

        // Notifications
        renderChart("chart-notifications", ["notifications_pending"], m, ["#d4a843"]);

        // Uptime (convert seconds to hours)
        renderChart("chart-uptime", ["uptime_seconds"], m, ["#7ec87e"], function(v) {
          return v / 3600;
        });

        if (statusEl) {
          var now = new Date();
          statusEl.textContent = "Updated " + now.getHours().toString().padStart(2, "0") + ":" +
                                 now.getMinutes().toString().padStart(2, "0") + ":" +
                                 now.getSeconds().toString().padStart(2, "0");
        }
      })
      .catch(function(err) {
        if (statusEl) statusEl.textContent = "Error: " + err.message;
      });
  }

  // Range button handlers
  document.querySelectorAll(".range-btn").forEach(function(btn) {
    btn.addEventListener("click", function() {
      document.querySelector(".range-btn-active").classList.remove("range-btn-active");
      btn.classList.add("range-btn-active");
      currentRange = btn.getAttribute("data-range");
      fetchAndRender();
    });
  });

  // Resize charts on window resize
  var resizeTimeout;
  window.addEventListener("resize", function() {
    clearTimeout(resizeTimeout);
    resizeTimeout = setTimeout(function() {
      Object.keys(charts).forEach(function(id) {
        var el = document.getElementById(id);
        if (el && charts[id]) {
          charts[id].setSize({ width: el.clientWidth, height: 180 });
        }
      });
    }, 150);
  });

  // Apply saved theme
  if (!isDark) document.body.classList.add("light-theme");

  // Initial fetch
  fetchAndRender();

  // Auto-refresh every 60 seconds
  refreshTimer = setInterval(fetchAndRender, 60000);
})();
