"use strict";

// «Граф отношений» (T-08-06, UI-05.6): связи между персонажами из social_graph.
// Canvas растягивается на весь остаток вкладки. Данные — только чтение из
// /api/atlas/worlds/graph.
(function () {
  var IDS = {
    viewport: "atlas-graph-viewport",
    canvas: "atlas-graph-canvas",
    status: "atlas-graph-status"
  };

  var lastData = null;
  var resizeTimer = 0;
  var viewportObserved = false;

  function byId(id) {
    return document.getElementById(id);
  }

  function setStatus(message) {
    var el = byId(IDS.status);
    if (el) {
      el.textContent = message;
    }
  }

  // Цвета берём из темы, чтобы граф не выпадал из тёмной панели Атласа.
  function themeColors() {
    var cs = getComputedStyle(document.documentElement);
    function pick(name, fallback) {
      var value = (cs.getPropertyValue(name) || "").trim();
      return value || fallback;
    }
    return {
      bg: pick("--card", "#143238"),
      hair: pick("--hair", "rgba(243, 236, 223, 0.06)"),
      edge: pick("--line-strong", "rgba(166, 132, 74, 0.45)"),
      text: pick("--text", "#f3ecdf"),
      muted: pick("--muted", "#a5b6b8"),
      accent: pick("--brass-2", "#c9a86a")
    };
  }

  function apiJSON(path) {
    return fetch(path, { headers: { Accept: "application/json" } }).then(function (res) {
      return res.json().catch(function () {
        return {};
      }).then(function (body) {
        if (!res.ok) {
          throw new Error((body && body.error && body.error.message) || res.statusText);
        }
        return body;
      });
    });
  }

  function activeSlug(worlds) {
    var slug = worlds.active || "";
    if (!slug && worlds.worlds && worlds.worlds.length) {
      slug = worlds.worlds[0].slug;
    }
    return slug;
  }

  function drawGrid(ctx, width, height, colors) {
    var step = 72;
    ctx.strokeStyle = colors.hair;
    ctx.lineWidth = 1;
    ctx.beginPath();
    for (var x = (width % step) / 2; x < width; x += step) {
      ctx.moveTo(x, 0);
      ctx.lineTo(x, height);
    }
    for (var y = (height % step) / 2; y < height; y += step) {
      ctx.moveTo(0, y);
      ctx.lineTo(width, y);
    }
    ctx.stroke();
  }

  function drawGraph(data) {
    lastData = data;
    var canvas = byId(IDS.canvas);
    var viewport = byId(IDS.viewport);
    if (!canvas || !viewport) {
      return;
    }
    var width = Math.max(280, viewport.clientWidth);
    var height = Math.max(240, viewport.clientHeight);
    var dpr = window.devicePixelRatio || 1;
    var pixelWidth = Math.round(width * dpr);
    var pixelHeight = Math.round(height * dpr);
    if (canvas.width !== pixelWidth || canvas.height !== pixelHeight) {
      canvas.width = pixelWidth;
      canvas.height = pixelHeight;
    }
    var ctx = canvas.getContext("2d");
    if (!ctx) {
      return;
    }

    var colors = themeColors();
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.fillStyle = colors.bg;
    ctx.fillRect(0, 0, width, height);
    drawGrid(ctx, width, height, colors);

    var dict = window.I18N || {};
    var nodes = data.nodes || [];
    if (nodes.length === 0) {
      setStatus(dict["atlas.graph.empty"] || "Связей пока нет.");
      return;
    }

    var positions = {};
    var radius = Math.max(80, Math.min(width, height) / 2 - 70);
    nodes.forEach(function (n, i) {
      var angle = (2 * Math.PI * i) / nodes.length - Math.PI / 2;
      positions[n.id] = {
        x: width / 2 + radius * Math.cos(angle),
        y: height / 2 + radius * Math.sin(angle)
      };
    });

    ctx.save();
    ctx.strokeStyle = colors.edge;
    ctx.lineWidth = 1.2;
    (data.edges || []).forEach(function (e) {
      var a = positions[e.a_character_id];
      var b = positions[e.b_character_id];
      if (!a || !b) {
        return;
      }
      ctx.beginPath();
      ctx.moveTo(a.x, a.y);
      ctx.lineTo(b.x, b.y);
      ctx.stroke();
      ctx.fillStyle = colors.muted;
      ctx.font = "11px system-ui, sans-serif";
      ctx.fillText(String(e.relation == null ? "" : e.relation), (a.x + b.x) / 2 + 4, (a.y + b.y) / 2 - 4);
    });
    ctx.restore();

    ctx.font = "13px system-ui, sans-serif";
    nodes.forEach(function (n) {
      var p = positions[n.id];
      if (!p) {
        return;
      }
      ctx.beginPath();
      ctx.arc(p.x, p.y, 8, 0, Math.PI * 2);
      ctx.fillStyle = colors.accent;
      ctx.fill();
      ctx.fillStyle = colors.text;
      ctx.fillText(String(n.name == null ? "" : n.name), p.x + 12, p.y + 4);
    });

    setStatus(dict["atlas.graph.status"] || "Связи между персонажами из социального графа.");
  }

  function redraw() {
    if (lastData) {
      drawGraph(lastData);
    }
  }

  function scheduleRedraw() {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(redraw, 100);
  }

  function observeViewport() {
    if (viewportObserved) {
      return;
    }
    var viewport = byId(IDS.viewport);
    if (!viewport) {
      return;
    }
    viewportObserved = true;
    if (typeof ResizeObserver !== "undefined") {
      new ResizeObserver(scheduleRedraw).observe(viewport);
    }
    window.addEventListener("resize", scheduleRedraw);
  }

  function loadGraph() {
    if (!byId(IDS.canvas)) {
      return;
    }
    observeViewport();
    apiJSON("/api/atlas/worlds").then(function (worlds) {
      var slug = activeSlug(worlds);
      if (!slug) {
        drawGraph({ nodes: [] });
        return null;
      }
      return apiJSON("/api/atlas/worlds/graph?world=" + encodeURIComponent(slug));
    }).then(function (data) {
      if (data) {
        drawGraph(data);
      }
    }).catch(function (err) {
      setStatus(err.message);
    });
  }

  function watchTheme() {
    if (typeof MutationObserver === "undefined") {
      return;
    }
    new MutationObserver(scheduleRedraw).observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["data-theme"]
    });
  }

  function start() {
    if (byId(IDS.canvas)) {
      loadGraph();
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", start);
  } else {
    start();
  }
  watchTheme();

  window.TNSGraph = { render: loadGraph, start: start, redraw: redraw };
})();
