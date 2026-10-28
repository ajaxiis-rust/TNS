"use strict";

// «Граф отношений» (T-08-06, UI-05.6): связи между персонажами из social_graph.
// Рендер в браузере без внешних библиотек; данные — только чтение из
// /api/atlas/worlds/graph.
(function () {
  var IDS = {
    viewport: "atlas-graph-viewport",
    canvas: "atlas-graph-canvas",
    status: "atlas-graph-status"
  };

  function byId(id) {
    return document.getElementById(id);
  }

  function setStatus(message) {
    var el = byId(IDS.status);
    if (el) {
      el.textContent = message;
    }
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

  function drawGraph(data) {
    var canvas = byId(IDS.canvas);
    if (!canvas) {
      return;
    }
    var dict = window.I18N || {};
    var nodes = data.nodes || [];
    if (nodes.length === 0) {
      canvas.width = 0;
      canvas.height = 0;
      setStatus(dict["atlas.graph.empty"] || "Связей пока нет.");
      return;
    }

    var viewport = byId(IDS.viewport);
    var width = Math.max(320, viewport ? viewport.clientWidth : 0) || 480;
    var height = 320;
    canvas.width = width;
    canvas.height = height;

    var ctx = canvas.getContext("2d");
    if (!ctx) {
      return;
    }

    ctx.fillStyle = "#ffffff";
    ctx.fillRect(0, 0, width, height);

    var positions = {};
    var radius = Math.min(width, height) / 2 - 48;
    nodes.forEach(function (n, i) {
      var angle = (2 * Math.PI * i) / nodes.length - Math.PI / 2;
      positions[n.id] = {
        x: width / 2 + radius * Math.cos(angle),
        y: height / 2 + radius * Math.sin(angle)
      };
    });

    (data.edges || []).forEach(function (e) {
      var a = positions[e.a_character_id];
      var b = positions[e.b_character_id];
      if (!a || !b) {
        return;
      }
      ctx.strokeStyle = "#9a9a9a";
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(a.x, a.y);
      ctx.lineTo(b.x, b.y);
      ctx.stroke();
      ctx.fillStyle = "#555555";
      ctx.font = "11px system-ui, sans-serif";
      ctx.fillText(String(e.relation == null ? "" : e.relation), (a.x + b.x) / 2, (a.y + b.y) / 2);
    });

    nodes.forEach(function (n) {
      var p = positions[n.id];
      if (!p) {
        return;
      }
      ctx.beginPath();
      ctx.arc(p.x, p.y, 8, 0, Math.PI * 2);
      ctx.fillStyle = "#1a6fb0";
      ctx.fill();
      ctx.fillStyle = "#1a1a1a";
      ctx.font = "12px system-ui, sans-serif";
      ctx.fillText(String(n.name == null ? "" : n.name), p.x + 12, p.y + 4);
    });

    setStatus(dict["atlas.graph.status"] || "Связи между персонажами из социального графа.");
  }

  function loadGraph() {
    if (!byId(IDS.canvas)) {
      return;
    }
    apiJSON("/api/atlas/worlds").then(function (worlds) {
      var slug = activeSlug(worlds);
      if (!slug) {
        setStatus((window.I18N && window.I18N["atlas.graph.empty"]) || "Связей пока нет.");
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

  window.TNSGraph = { render: loadGraph, start: start };
})();
