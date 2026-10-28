"use strict";

// «Прорисовка карты» (T-08-06, UI-05.5, W-20): рендер карты мира в браузере
// без внешних библиотек и тайлов (canvas). Данные — только чтение из
// /api/atlas/worlds и /api/atlas/worlds/map.
(function () {
  var IDS = {
    viewport: "atlas-map-viewport",
    canvas: "atlas-map-canvas",
    status: "atlas-map-status"
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

  function fitBounds(places) {
    var minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
    places.forEach(function (p) {
      if (typeof p.x !== "number" || typeof p.y !== "number") {
        return;
      }
      if (p.x < minX) { minX = p.x; }
      if (p.x > maxX) { maxX = p.x; }
      if (p.y < minY) { minY = p.y; }
      if (p.y > maxY) { maxY = p.y; }
    });
    if (!isFinite(minX) || !isFinite(maxX)) {
      minX = -1; maxX = 1; minY = -1; maxY = 1;
    }
    if (minX === maxX) { minX -= 1; maxX += 1; }
    if (minY === maxY) { minY -= 1; maxY += 1; }
    return { minX: minX, maxX: maxX, minY: minY, maxY: maxY };
  }

  function project(x, y, bounds, width, height, pad) {
    var spanX = bounds.maxX - bounds.minX;
    var spanY = bounds.maxY - bounds.minY;
    var scale = Math.min((width - pad * 2) / spanX, (height - pad * 2) / spanY);
    var cx = (bounds.minX + bounds.maxX) / 2;
    var cy = (bounds.minY + bounds.maxY) / 2;
    return {
      px: width / 2 + (x - cx) * scale,
      py: height / 2 - (y - cy) * scale
    };
  }

  function drawMap(data) {
    var canvas = byId(IDS.canvas);
    if (!canvas) {
      return;
    }
    var dict = window.I18N || {};
    var places = data.places || [];
    if (places.length === 0) {
      canvas.width = 0;
      canvas.height = 0;
      setStatus(dict["atlas.map.empty"] || "Известных мест пока нет.");
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

    var bounds = fitBounds(places);
    var pad = 24;

    (data.paths || []).forEach(function (p) {
      var a = project(p.from_x, p.from_y, bounds, width, height, pad);
      var b = project(p.to_x, p.to_y, bounds, width, height, pad);
      ctx.strokeStyle = "#9a9a9a";
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(a.px, a.py);
      ctx.lineTo(b.px, b.py);
      ctx.stroke();
    });

    places.forEach(function (p) {
      var pt = project(p.x, p.y, bounds, width, height, pad);
      ctx.beginPath();
      ctx.arc(pt.px, pt.py, p.known ? 6 : 4, 0, Math.PI * 2);
      if (p.known) {
        ctx.fillStyle = "#1a6fb0";
        ctx.fill();
      } else {
        ctx.fillStyle = "#ffffff";
        ctx.fill();
        ctx.strokeStyle = "#1a6fb0";
        ctx.stroke();
      }
      ctx.fillStyle = "#1a1a1a";
      ctx.font = "12px system-ui, sans-serif";
      ctx.fillText(String(p.name == null ? "" : p.name), pt.px + 8, pt.py + 4);
    });

    setStatus(dict["atlas.map.status"] || "Карта дорисовывается по мере путешествий героя.");
  }

  function loadMap() {
    if (!byId(IDS.canvas)) {
      return;
    }
    apiJSON("/api/atlas/worlds").then(function (worlds) {
      var slug = activeSlug(worlds);
      if (!slug) {
        setStatus((window.I18N && window.I18N["atlas.map.empty"]) || "Известных мест пока нет.");
        return null;
      }
      return apiJSON("/api/atlas/worlds/map?world=" + encodeURIComponent(slug));
    }).then(function (data) {
      if (data) {
        drawMap(data);
      }
    }).catch(function (err) {
      setStatus(err.message);
    });
  }

  function start() {
    if (byId(IDS.canvas)) {
      loadMap();
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", start);
  } else {
    start();
  }

  window.TNSMap = { render: loadMap, start: start };
})();
