"use strict";

// Коэффициенты и карточка лемматизатора вкладки MCP (T-08-10, UI-07.10–11).
// Модалка правит станковые коэффициенты математики ходов без LLM — разрез общий
// для всех миров; карточка показывает лемматизатор текущего языкового пакета и
// переключатель режима без лемматизатора.
(function () {
  var IDS = {
    coeffsOpen: "mcp-coeffs-open",
    coeffsModal: "mcp-coeffs-modal",
    coeffsWarning: "mcp-coeffs-warning",
    coeffsForm: "mcp-coeffs-form",
    coeffsSave: "mcp-coeffs-save",
    coeffsClose: "mcp-coeffs-close",
    coeffConfirmDictThreshold: "coeff-confirm-dict-threshold",
    coeffConfirmVectorThreshold: "coeff-confirm-vector-threshold",
    coeffConfirmMarginDict: "coeff-confirm-margin-dict",
    coeffConfirmMarginVector: "coeff-confirm-margin-vector",
    coeffSedimentDedupThreshold: "coeff-sediment-dedup-threshold",
    coeffSawRiseMin: "coeff-saw-rise-min",
    coeffSawRiseMax: "coeff-saw-rise-max",
    coeffSawPeak: "coeff-saw-peak",
    coeffSawFall: "coeff-saw-fall",
    coeffSawFlat: "coeff-saw-flat",
    coeffTimeNightThresholdMin: "coeff-time-night-threshold-min",
    lemmatizer: "mcp-lemmatizer",
    lemmatizerEnabled: "mcp-lemmatizer-enabled"
  };

  var FIELDS = [
    ["confirm", "dict_threshold", IDS.coeffConfirmDictThreshold],
    ["confirm", "vector_threshold", IDS.coeffConfirmVectorThreshold],
    ["confirm", "margin_dict", IDS.coeffConfirmMarginDict],
    ["confirm", "margin_vector", IDS.coeffConfirmMarginVector],
    ["sediment", "dedup_threshold", IDS.coeffSedimentDedupThreshold],
    ["saw", "rise_min", IDS.coeffSawRiseMin],
    ["saw", "rise_max", IDS.coeffSawRiseMax],
    ["saw", "peak", IDS.coeffSawPeak],
    ["saw", "fall", IDS.coeffSawFall],
    ["saw", "flat", IDS.coeffSawFlat]
  ];

  function byId(id) {
    return document.getElementById(id);
  }

  function dict(key, fallback) {
    var d = window.I18N || {};
    return Object.prototype.hasOwnProperty.call(d, key) ? d[key] : fallback;
  }

  function escapeHTML(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }

  function apiJSON(path, options) {
    var opts = options || {};
    opts.headers = opts.headers || {};
    opts.headers.Accept = "application/json";
    return fetch(path, opts).then(function (res) {
      return res.json().catch(function () {
        return {};
      }).then(function (body) {
        if (!res.ok) {
          var msg = (body && body.error && body.error.message) || res.statusText;
          throw new Error(msg);
        }
        return body;
      });
    });
  }

  function setValue(id, value) {
    var el = byId(id);
    if (el) {
      el.value = value;
    }
  }

  function openCoeffsModal() {
    var modal = byId(IDS.coeffsModal);
    if (modal) {
      modal.hidden = false;
    }
    loadCoeffs();
  }

  function closeCoeffsModal() {
    var modal = byId(IDS.coeffsModal);
    if (modal) {
      modal.hidden = true;
    }
  }

  function loadCoeffs() {
    apiJSON("/api/mcp/coeffs").then(renderCoeffs).catch(function (err) {
      var warning = byId(IDS.coeffsWarning);
      if (warning) {
        warning.textContent = err.message;
      }
    });
  }

  function renderCoeffs(data) {
    FIELDS.forEach(function (f) {
      var section = data[f[0]] || {};
      setValue(f[2], section[f[1]]);
    });
    var time = data.time || {};
    setValue(IDS.coeffTimeNightThresholdMin, time.night_threshold_min);
  }

  function submitCoeffs() {
    if (!window.confirm(dict("mcp.coeffs.warning", "Меняет пересчёт и логику ходов."))) {
      return;
    }
    var body = { confirm: {}, sediment: {}, saw: {}, time: {} };
    FIELDS.forEach(function (f) {
      var el = byId(f[2]);
      if (el && el.value !== "") {
        body[f[0]][f[1]] = parseFloat(el.value);
      }
    });
    var night = byId(IDS.coeffTimeNightThresholdMin);
    if (night && night.value !== "") {
      body.time.night_threshold_min = parseInt(night.value, 10);
    }
    apiJSON("/api/mcp/coeffs", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body)
    }).then(function () {
      closeCoeffsModal();
    }).catch(function (err) {
      var warning = byId(IDS.coeffsWarning);
      if (warning) {
        warning.textContent = err.message;
      }
    });
  }

  function loadLemmatizer() {
    apiJSON("/api/mcp/lemmatizer").then(renderLemmatizer).catch(function (err) {
      var box = byId(IDS.lemmatizer);
      if (box) {
        box.textContent = err.message;
      }
    });
  }

  function renderLemmatizer(data) {
    var box = byId(IDS.lemmatizer);
    if (box) {
      if (!data.pack_id) {
        box.textContent = dict("mcp.lemmatizer.empty", "Языковой пакет не найден.");
      } else {
        var html = '<p class="mcp-lemmatizer__line">' +
          dict("mcp.lemmatizer.pack", "Пакет") + ": " +
          escapeHTML(data.name || data.pack_id) + " · " +
          escapeHTML(data.lang || "") + " · " +
          escapeHTML(data.version || "") + "</p>";
        html += '<p class="mcp-lemmatizer__line">' +
          dict("mcp.lemmatizer.kind", "Библиотека") + ": " +
          escapeHTML(data.lemmatizer ? data.lemmatizer.kind : "") + "</p>";
        html += '<p class="mcp-lemmatizer__line">' +
          dict("mcp.lemmatizer.protocol", "Протокол") + ": " +
          escapeHTML(data.lemmatizer ? data.lemmatizer.protocol : "") + "</p>";
        box.innerHTML = html;
      }
    }
    var enabled = byId(IDS.lemmatizerEnabled);
    if (enabled) {
      // Чекбокс подписан «Без лемматизатора»: checked = lemmatizer выключен.
      enabled.checked = !data.enabled;
    }
  }

  function bind() {
    var open = byId(IDS.coeffsOpen);
    if (open) {
      open.addEventListener("click", openCoeffsModal);
    }
    var close = byId(IDS.coeffsClose);
    if (close) {
      close.addEventListener("click", closeCoeffsModal);
    }
    var save = byId(IDS.coeffsSave);
    if (save) {
      save.addEventListener("click", submitCoeffs);
    }
    var form = byId(IDS.coeffsForm);
    if (form) {
      form.addEventListener("submit", function (event) {
        event.preventDefault();
        submitCoeffs();
      });
    }
    var enabled = byId(IDS.lemmatizerEnabled);
    if (enabled) {
      enabled.addEventListener("change", function () {
        apiJSON("/api/mcp/lemmatizer", {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ enabled: !enabled.checked })
        }).catch(function () {
          enabled.checked = !enabled.checked;
        });
      });
    }
  }

  bind();
  loadLemmatizer();
})();
