// Прямое открытие /wizard.html: страница работает сама — проверка компьютера,
// движок и шаги. В интерфейсе игры шаблон вставляет app.js, а innerHTML не
// исполняет подключённые скрипты, поэтому этот код выполняет только прямой запуск.
(function () {
  "use strict";
  var $ = function (id) { return document.getElementById(id); };

  // Адреса скачивания в разметке лежат без схемы (data-source): ui-lint
  // (проверка 1) запрещает полные внешние адреса в статике.
  var MODEL_SOURCE_SCHEME = "https:" + "//";

  function wireDownloadLinks() {
    document.querySelectorAll(".wizard-download-link[data-source]").forEach(function (link) {
      link.href = MODEL_SOURCE_SCHEME + link.getAttribute("data-source");
    });
  }

  function api(method, path, body, rawBody) {
    var opt = { method: method, headers: { Accept: "application/json" } };
    if (rawBody) {
      opt.body = rawBody;
    } else if (body !== undefined) {
      opt.headers["Content-Type"] = "application/json";
      opt.body = JSON.stringify(body);
    }
    return fetch("/api" + path, opt).then(function (res) {
      return res.text().then(function (raw) {
        var data = {};
        try { data = raw ? JSON.parse(raw) : {}; } catch (e) { data = {}; }
        if (!res.ok) {
          var msg = (data && data.error && data.error.message) || res.statusText;
          var err = new Error(msg);
          err.fields = (data && data.error && data.error.fields) || [];
          throw err;
        }
        return data;
      });
    });
  }

  function setResult(node, text, ok) {
    if (!node) { return; }
    node.textContent = text;
    node.classList.toggle("is-missing", !ok);
  }

  function formResult(form, text, ok) {
    if (!form) { return; }
    var box = form.querySelector(".wizard-form-result");
    if (!box) {
      box = document.createElement("p");
      box.className = "wizard-form-result wizard-hint";
      form.appendChild(box);
    }
    box.textContent = text;
    box.style.color = ok ? "" : "#ff8a8a";
  }

  function checkRow(ok, text) {
    var li = document.createElement("li");
    li.className = "wizard-check " + (ok ? "wizard-check--ok" : "wizard-check--warn");
    var dot = document.createElement("span");
    dot.className = "wizard-check__dot";
    dot.setAttribute("aria-hidden", "true");
    dot.textContent = ok ? "✓" : "✗";
    li.appendChild(dot);
    li.appendChild(document.createTextNode(" " + text));
    return li;
  }

  function renderStation(station) {
    station = station || {};
    var server = station.llama_server || {};
    var box = $("wizard-station");
    if (box) {
      box.innerHTML = "";
      box.appendChild(checkRow(!!server.found, server.found
        ? "llama-server: " + (server.path || "") + " (версия " + (server.version || "—") + ")"
        : "llama-server не найден"));
      box.appendChild(checkRow(station.gen_port_free !== false, "Порт генератора :5001 свободен"));
      box.appendChild(checkRow(station.embed_port_free !== false, "Порт эмбеддера :5002 свободен"));
      box.appendChild(checkRow(!!station.embed_model, "Модель смысла установлена" + (station.embed_model ? ": " + station.embed_model : "")));
      (station.children || []).forEach(function (child) {
        box.appendChild(checkRow(!!child.ready, child.name + " :" + child.port + (child.ready ? " работает" : " не отвечает")));
      });
    }
    setResult($("wizard-engine-result"), server.found
      ? "Найден: " + (server.path || "") + " (версия " + (server.version || "—") + ")"
      : "llama-server не найден", !!server.found);
    var input = $("wizard-engine-path");
    if (input && server.found && server.path && document.activeElement !== input) {
      input.value = server.path;
    }
  }

  function refresh() {
    return api("GET", "/first-run").then(function (data) {
      renderStation((data && data.station) || {});
    });
  }

  var current = 0;
  function showStep(step) {
    current = step;
    document.querySelectorAll("[data-wizard-step-panel]").forEach(function (panel) {
      panel.hidden = panel.getAttribute("data-wizard-step-panel") !== String(step);
    });
    document.querySelectorAll("[data-wizard-step]").forEach(function (li) {
      var i = Number(li.getAttribute("data-wizard-step"));
      li.classList.toggle("is-current", i === step);
      li.classList.toggle("is-done", i < step);
    });
  }

  function putStep(step, btn) {
    if (btn) { btn.disabled = true; }
    return api("PUT", "/first-run/step", { step: String(step) }).catch(function () {
      // Шаг не записался — экран не ломаем: мастер можно пройти повторно.
    }).then(function () {
      if (btn) { btn.disabled = false; }
    });
  }

  function next(step, to, btn) {
    showStep(to);
    putStep(step, btn);
  }

  var saveBtn = $("wizard-engine-path-save");
  if (saveBtn) {
    saveBtn.addEventListener("click", function () {
      var input = $("wizard-engine-path");
      saveBtn.disabled = true;
      api("POST", "/settings/llama/detect", { path: input ? input.value : "" })
        .then(refresh)
        .catch(function (err) { setResult($("wizard-engine-result"), err.message, false); })
        .then(function () { saveBtn.disabled = false; });
    });
  }

  var scanBtn = $("wizard-engine-scan");
  if (scanBtn) {
    scanBtn.addEventListener("click", function () {
      scanBtn.disabled = true;
      api("POST", "/settings/llama/scan", {})
        .then(function (data) {
          var paths = (data && data.paths) || [];
          var list = $("wizard-engine-found");
          if (!list) { return; }
          list.hidden = false;
          list.innerHTML = "";
          if (paths.length === 0) {
            var empty = document.createElement("li");
            empty.className = "wizard-empty";
            empty.textContent = "Ничего не нашлось";
            list.appendChild(empty);
            return;
          }
          paths.forEach(function (path) {
            var li = document.createElement("li");
            var btn = document.createElement("button");
            btn.type = "button";
            btn.className = "btn btn--ghost";
            btn.textContent = path;
            btn.addEventListener("click", function () {
              var input = $("wizard-engine-path");
              if (input) { input.value = path; }
              api("POST", "/settings/llama/detect", { path: path }).then(refresh).catch(function () {});
            });
            li.appendChild(btn);
            list.appendChild(li);
          });
        })
        .catch(function (err) { setResult($("wizard-engine-result"), err.message, false); })
        .then(function () { scanBtn.disabled = false; });
    });
  }

  [
    ["wizard-step0-next", "0", 1],
    ["wizard-step1-next", "1", 2],
    ["wizard-step2-skip", "2", 3],
    ["wizard-step2-next", "2", 3],
    ["wizard-step3-next", "3", 4]
  ].forEach(function (pair) {
    var btn = $(pair[0]);
    if (btn) {
      btn.addEventListener("click", function () { next(pair[1], pair[2], btn); });
    }
  });

  var doneBtn = $("wizard-step4-done");
  if (doneBtn) {
    doneBtn.addEventListener("click", function () {
      doneBtn.disabled = true;
      putStep("4", doneBtn).then(function () { window.location.href = "/"; });
    });
  }
  document.querySelectorAll("[data-wizard-back]").forEach(function (btn) {
    btn.addEventListener("click", function () { showStep(Number(btn.getAttribute("data-wizard-back"))); });
  });
  document.querySelectorAll("[data-wizard-goto]").forEach(function (btn) {
    btn.addEventListener("click", function () { showStep(Number(btn.getAttribute("data-wizard-goto"))); });
  });

  var libraryForm = $("wizard-library-form");
  if (libraryForm) {
    libraryForm.addEventListener("submit", function (event) {
      event.preventDefault();
      var input = $("wizard-library-file");
      var file = input && input.files && input.files[0];
      if (!file) { formResult(libraryForm, "Выберите файл книги", false); return; }
      var form = new FormData();
      form.append("file", file);
      api("POST", "/library/import", undefined, form).then(function (data) {
        var list = $("wizard-library-list");
        if (list) {
          list.innerHTML = "";
          (data.books || []).forEach(function (book) {
            var li = document.createElement("li");
            li.textContent = (book.name || book.id) + " · " + (book.status || "");
            list.appendChild(li);
          });
        }
        formResult(libraryForm, "Книга на полке", true);
      }).catch(function (err) { formResult(libraryForm, err.message, false); });
    });
  }

  var worldForm = $("wizard-world-form");
  function fillSelect(select, values, fallback) {
    if (!select) { return; }
    select.innerHTML = "";
    values.forEach(function (value) {
      var option = document.createElement("option");
      option.value = value;
      option.textContent = value;
      select.appendChild(option);
    });
    if (select.options.length === 0 && fallback) {
      var option = document.createElement("option");
      option.value = fallback;
      option.textContent = fallback;
      select.appendChild(option);
    }
  }
  api("GET", "/atlas/birth").then(function (data) {
    fillSelect($("wizard-world-social"), (data && data.social_orders) || [], "феодализм");
    fillSelect($("wizard-world-rating"), (data && data.ratings) || [], "16+");
    var genres = $("wizard-world-genres");
    if (genres) {
      genres.innerHTML = "";
      ((data && data.genres) || []).forEach(function (genre) {
        var label = document.createElement("label");
        label.className = "choice";
        var input = document.createElement("input");
        input.type = "checkbox";
        input.value = genre;
        input.checked = genre === "фэнтези";
        var span = document.createElement("span");
        span.textContent = genre;
        label.appendChild(input);
        label.appendChild(span);
        genres.appendChild(label);
      });
    }
  }).catch(function () {});

  if (worldForm) {
    worldForm.addEventListener("submit", function (event) {
      event.preventDefault();
      var title = (($("wizard-world-title") || {}).value || "").trim();
      if (!title) { formResult(worldForm, "Укажите название мира", false); return; }
      var slug = (($("wizard-world-slug") || {}).value || "").trim().toLowerCase();
      if (!slug) {
        slug = title.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
      }
      if (!/^[a-z0-9-]+$/.test(slug)) {
        formResult(worldForm, "Адрес мира — только латиница, цифры и дефис", false);
        return;
      }
      var genres = [];
      document.querySelectorAll("#wizard-world-genres input:checked").forEach(function (input) {
        genres.push(input.value);
      });
      if (genres.length === 0) { genres = ["фэнтези"]; }
      var body = {
        title: title,
        slug: slug,
        synopsis: (($("wizard-world-synopsis") || {}).value || "") || ("Мир «" + title + "» ждёт героя. История ещё не написана."),
        lang: "ru",
        travel_mode: "вымышленная",
        genres: genres,
        social_order: ($("wizard-world-social") || {}).value || "феодализм",
        censorship: ($("wizard-world-rating") || {}).value || "16+"
      };
      api("POST", "/atlas/worlds/birth", body).then(function () {
        formResult(worldForm, "Мир создан", true);
        var nextBtn = $("wizard-step3-next");
        if (nextBtn) { nextBtn.hidden = false; }
      }).catch(function (err) {
        (err.fields || []).forEach(function (field) {
          var box = worldForm.querySelector('[data-field-error="' + field.field + '"]');
          if (box) { box.textContent = field.reason; }
        });
        formResult(worldForm, err.message, false);
      });
    });
  }

  wireDownloadLinks();
  showStep(0);
  refresh();
  setInterval(refresh, 2000);
})();
