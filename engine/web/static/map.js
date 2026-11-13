"use strict";

// «Карта» Атласа (T-08-06, UI-05.5, W-20): canvas растягивается на весь
// остаток вкладки и перерисовывается при изменении размера. Данные — только
// чтение из /api/atlas/worlds и /api/atlas/worlds/map.
(function () {
  var IDS = {
    viewport: "atlas-map-viewport",
    canvas: "atlas-map-canvas",
    status: "atlas-map-status"
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

  // Цвета берём из темы, чтобы карта не выпадала из тёмной панели Атласа.
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
      accent: pick("--brass-2", "#c9a86a"),
      ok: pick("--ok", "#7fae88")
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

  function drawMap(data) {
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
    var places = data.places || [];
    if (places.length === 0) {
      setStatus(dict["atlas.map.empty"] || "Известных мест пока нет.");
      return;
    }

    var bounds = fitBounds(places);
    var pad = Math.max(48, Math.min(width, height) * 0.08);

    ctx.save();
    ctx.strokeStyle = colors.edge;
    ctx.lineWidth = 1.6;
    ctx.setLineDash([6, 6]);
    (data.paths || []).forEach(function (p) {
      var a = project(p.from_x, p.from_y, bounds, width, height, pad);
      var b = project(p.to_x, p.to_y, bounds, width, height, pad);
      ctx.beginPath();
      ctx.moveTo(a.px, a.py);
      ctx.lineTo(b.px, b.py);
      ctx.stroke();
    });
    ctx.restore();

    ctx.font = "13px system-ui, sans-serif";
    places.forEach(function (p) {
      var pt = project(p.x, p.y, bounds, width, height, pad);
      ctx.beginPath();
      ctx.arc(pt.px, pt.py, p.known ? 7 : 5, 0, Math.PI * 2);
      if (p.known) {
        ctx.fillStyle = colors.accent;
        ctx.fill();
      } else {
        ctx.fillStyle = colors.bg;
        ctx.fill();
        ctx.strokeStyle = colors.muted;
        ctx.lineWidth = 1.4;
        ctx.stroke();
      }
      ctx.fillStyle = p.known ? colors.text : colors.muted;
      ctx.fillText(String(p.name == null ? "" : p.name), pt.px + 11, pt.py + 4);
    });

    if (data.hero && data.hero.place_id > 0) {
      var hero = project(data.hero.x, data.hero.y, bounds, width, height, pad);
      ctx.beginPath();
      ctx.arc(hero.px, hero.py, 13, 0, Math.PI * 2);
      ctx.strokeStyle = colors.ok;
      ctx.lineWidth = 2;
      ctx.stroke();
      ctx.fillStyle = colors.ok;
      ctx.font = "12px system-ui, sans-serif";
      ctx.fillText(dict["atlas.map.hero"] || "герой", hero.px + 15, hero.py + 17);
    }

    setStatus(dict["atlas.map.status"] || "Карта дорисовывается по мере путешествий героя.");
  }

  function redraw() {
    if (lastData) {
      drawMap(lastData);
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

  function loadMap() {
    if (!byId(IDS.canvas)) {
      return;
    }
    observeViewport();
    apiJSON("/api/atlas/worlds").then(function (worlds) {
      var slug = activeSlug(worlds);
      if (!slug) {
        drawMap({ places: [] });
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
      loadMap();
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", start);
  } else {
    start();
  }
  watchTheme();

  window.TNSMap = { render: loadMap, start: start, redraw: redraw };
})();
