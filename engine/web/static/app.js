"use strict";

// Каркас меню и раскладка (T-08-01): чат — точка входа, динамическое
// горизонтальное меню из четырёх вкладок; «Чат» появляется при входе во вкладку.
// Роут вкладки Атлас (T-08-05): экран загружается из /atlas.html и наполняется
// через /api/atlas/*.
(function () {
  var IDS = {
    menuBar: "menu-bar",
    views: "views",
    tabChat: "tab-chat",
    tabAtlas: "tab-atlas",
    tabHistory: "tab-history",
    tabMcp: "tab-mcp",
    tabSettings: "tab-settings",
    viewChat: "view-chat",
    viewAtlas: "view-atlas",
    viewHistory: "view-history",
    viewMcp: "view-mcp",
    viewSettings: "view-settings",
    chatLog: "chat-log",
    statePanel: "state-panel",
    worldPanel: "world-panel",
    heroPanel: "hero-panel",
    systemPanel: "system-panel",
    atlasWorlds: "atlas-worlds",
    atlasBirthForm: "atlas-birth-form",
    atlasBirthSlug: "atlas-birth-slug",
    atlasBirthTitle: "atlas-birth-title",
    atlasBirthDescription: "atlas-birth-description",
    atlasBirthSynopsis: "atlas-birth-synopsis",
    atlasBirthPrologue: "atlas-birth-prologue",
    atlasBirthLang: "atlas-birth-lang",
    atlasBirthTravelMode: "atlas-birth-travel-mode",
    atlasBirthGenres: "atlas-birth-genres",
    atlasBirthWorldRules: "atlas-birth-world-rules",
    atlasBirthSocialOrder: "atlas-birth-social-order",
    atlasBirthEconomyMods: "atlas-birth-economy-mods",
    atlasBirthMagic: "atlas-birth-magic",
    atlasBirthHeroBirth: "atlas-birth-hero-birth",
    atlasBirthCensorship: "atlas-birth-censorship",
    atlasBirthSubmit: "atlas-birth-submit",
    atlasWizard: "atlas-wizard",
    atlasWizardSynopsis: "atlas-wizard-synopsis",
    atlasWizardName: "atlas-wizard-name",
    atlasWizardHints: "atlas-wizard-hints",
    atlasWizardAge: "atlas-wizard-age",
    atlasWizardIsekai: "atlas-wizard-isekai",
    atlasWizardStart: "atlas-wizard-start",
    bookButton: "book-button",
    bookClose: "book-close",
    bookDigest: "book-digest",
    bookToc: "book-toc",
    bookChapterTitle: "book-chapter-title",
    bookChapterText: "book-chapter-text",
    bookCuts: "book-cuts",
    bookRaw: "book-raw",
    bookExport: "book-export",
    bookExportScope: "book-export-scope",
    bookExportChapter: "book-export-chapter",
    bookExportSubmit: "book-export-submit",
    bookExportStatus: "book-export-status",
    mcpDashQueue: "mcp-dash-queue",
    mcpDashCurrent: "mcp-dash-current",
    mcpDashStats: "mcp-dash-stats",
    mcpImportForm: "mcp-import-form",
    mcpImportFile: "mcp-import-file",
    mcpImportSubmit: "mcp-import-submit",
    mcpForge: "mcp-forge",
    mcpBooks: "mcp-books",
    mcpRings: "mcp-rings",
    mcpTools: "mcp-tools",
    settingsGeneralForm: "settings-general-form",
    settingsLang: "settings-lang",
    settingsTheme: "settings-theme",
    settingsThemePreview: "settings-theme-preview",
    settingsServerHost: "settings-server-host",
    settingsServerPort: "settings-server-port",
    settingsRestartWarning: "settings-restart-warning",
    settingsDBPath: "settings-db-path",
    settingsAuth: "settings-auth",
    settingsVectorSearch: "settings-vector-search",
    settingsGeneralSave: "settings-general-save",
    settingsLLMForm: "settings-llm-form",
    settingsGenAddr: "settings-gen-addr",
    settingsGenKey: "settings-gen-key",
    settingsGenModel: "settings-gen-model",
    settingsGenTimeout: "settings-gen-timeout",
    settingsGenMaxTokens: "settings-gen-max-tokens",
    settingsGenTemperature: "settings-gen-temperature",
    settingsGenRetries: "settings-gen-retries",
    settingsGenConcurrency: "settings-gen-concurrency",
    settingsEmbAddr: "settings-emb-addr",
    settingsEmbKey: "settings-emb-key",
    settingsEmbModel: "settings-emb-model",
    settingsEmbTimeout: "settings-emb-timeout",
    settingsLLMLocal: "settings-llm-local",
    settingsLLMSampling: "settings-llm-sampling",
    settingsPresets: "settings-presets",
    settingsLLMSave: "settings-llm-save",
    settingsProviders: "settings-providers",
    settingsProviderForm: "settings-provider-form",
    settingsProviderName: "settings-provider-name",
    settingsProviderKey: "settings-provider-key",
    settingsProviderSave: "settings-provider-save",
    settingsModels: "settings-models",
    settingsModelsForm: "settings-models-form",
    settingsModelRole: "settings-model-role",
    settingsModelSource: "settings-model-source",
    settingsModelRef: "settings-model-ref",
    settingsModelsSave: "settings-models-save",
    settingsRoles: "settings-roles",
    settingsRolesForm: "settings-roles-form",
    settingsRoleName: "settings-role-name",
    settingsRoleProvider: "settings-role-provider",
    settingsRoleModel: "settings-role-model",
    settingsRoleTemperature: "settings-role-temperature",
    settingsRolePrompts: "settings-role-prompts",
    settingsRolePriority: "settings-role-priority",
    settingsRolesSave: "settings-roles-save",
    settingsWorldForm: "settings-world-form",
    settingsPaganelEnabled: "settings-paganel-enabled",
    settingsScoutEnabled: "settings-scout-enabled",
    settingsPaganelKey: "settings-paganel-key",
    settingsWorldSave: "settings-world-save",
    settingsPlugins: "settings-plugins",
    settingsAdvancedForm: "settings-advanced-form",
    settingsFlags: "settings-flags",
    settingsMemoryMax: "settings-memory-max",
    settingsMemorySimilarity: "settings-memory-similarity",
    settingsMemoryHalflife: "settings-memory-halflife",
    settingsEmbedDim: "settings-embed-dim",
    settingsGlobalLuck: "settings-global-luck",
    settingsAdvancedSave: "settings-advanced-save"
  };

  function byId(id) {
    return document.getElementById(id);
  }

  var menuBar = byId(IDS.menuBar);
  var views = byId(IDS.views);
  var chatTab = byId(IDS.tabChat);
  var atlasTab = byId(IDS.tabAtlas);
  var historyTab = byId(IDS.tabHistory);
  var mcpTab = byId(IDS.tabMcp);
  var settingsTab = byId(IDS.tabSettings);
  var viewChat = byId(IDS.viewChat);
  var viewAtlas = byId(IDS.viewAtlas);
  var viewHistory = byId(IDS.viewHistory);
  var viewMcp = byId(IDS.viewMcp);
  var viewSettings = byId(IDS.viewSettings);
  var chatLog = byId(IDS.chatLog);
  var statePanel = byId(IDS.statePanel);
  var worldPanel = byId(IDS.worldPanel);
  var heroPanel = byId(IDS.heroPanel);
  var systemPanel = byId(IDS.systemPanel);

  function translate(root) {
    var dict = window.I18N || {};
    root.querySelectorAll("[data-i18n]").forEach(function (node) {
      var key = node.getAttribute("data-i18n");
      node.textContent = Object.prototype.hasOwnProperty.call(dict, key)
        ? dict[key]
        : key;
    });
  }

  function setActiveTab(activeTab) {
    menuBar.querySelectorAll(".tab").forEach(function (tab) {
      tab.classList.toggle("active", tab === activeTab);
    });
  }

  function showView(view, activeTab) {
    views.querySelectorAll(".view").forEach(function (candidate) {
      candidate.hidden = candidate !== view;
    });
    setActiveTab(activeTab);
  }

  function showChat() {
    // «Чат» в меню виден только когда открыта другая вкладка.
    chatTab.hidden = true;
    showView(viewChat, null);
    // Панель мир/герой/система всегда остаётся внутри чата.
    statePanel.hidden = false;
    worldPanel.hidden = false;
    heroPanel.hidden = false;
    systemPanel.hidden = false;
    chatLog.hidden = false;
  }

  var sections = [
    [atlasTab, viewAtlas],
    [historyTab, viewHistory],
    [mcpTab, viewMcp],
    [settingsTab, viewSettings]
  ];

  sections.forEach(function (pair) {
    var tab = pair[0];
    var view = pair[1];
    tab.addEventListener("click", function () {
      chatTab.hidden = false;
      showView(view, tab);
      if (view === viewAtlas) {
        loadAtlasView();
      } else if (view === viewHistory) {
        openBookWindow();
      } else if (view === viewMcp) {
        loadMcpView();
      } else if (view === viewSettings) {
        loadSettingsView();
      }
    });
  });

  chatTab.addEventListener("click", showChat);

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

  function postJSON(path, body) {
    return apiJSON(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body)
    });
  }

  function putJSON(path, body) {
    return apiJSON(path, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body)
    });
  }

  function escapeHTML(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }

  function fieldValue(id) {
    var el = byId(id);
    return el ? el.value.trim() : "";
  }

  function splitList(text) {
    return text.split(/[,;]+/).map(function (s) { return s.trim(); }).filter(Boolean);
  }

  function splitLines(text) {
    return text.split(/\r?\n/).map(function (s) { return s.trim(); }).filter(Boolean);
  }

  var atlasLoaded = false;

  function loadAtlasView() {
    if (atlasLoaded) {
      loadAtlasWorlds();
      return;
    }
    fetch("/atlas.html").then(function (res) {
      if (!res.ok) {
        throw new Error("atlas not found");
      }
      return res.text();
    }).then(function (html) {
      viewAtlas.innerHTML = html;
      translate(viewAtlas);
      bindAtlas();
      atlasLoaded = true;
      loadAtlasWorlds();
    }).catch(function (err) {
      viewAtlas.textContent = err.message;
    });
  }

  function bindAtlas() {
    var form = byId(IDS.atlasBirthForm);
    if (form) {
      form.addEventListener("submit", function (event) {
        event.preventDefault();
        submitAtlasBirth();
      });
    }
    var start = byId(IDS.atlasWizardStart);
    if (start) {
      start.addEventListener("click", startAtlasWizard);
    }
    var worlds = byId(IDS.atlasWorlds);
    if (worlds) {
      worlds.addEventListener("click", onAtlasWorldsClick);
    }
  }

  function loadAtlasWorlds() {
    apiJSON("/api/atlas/worlds").then(renderAtlasWorlds).catch(function (err) {
      var box = byId(IDS.atlasWorlds);
      if (box) {
        box.textContent = err.message;
      }
    });
  }

  function renderAtlasWorlds(data) {
    window.__activeSlug = data.active || "";
    var box = byId(IDS.atlasWorlds);
    if (!box) {
      return;
    }
    var dict = window.I18N || {};
    var worlds = data.worlds || [];
    if (worlds.length === 0) {
      box.innerHTML = '<p data-i18n="atlas.empty">' + (dict["atlas.empty"] || "Пока нет миров.") + "</p>";
      return;
    }
    box.innerHTML = worlds.map(function (w) {
      var active = w.active ? ' <span class="atlas-card__active">' + (dict["atlas.active"] || "Активен") + "</span>" : "";
      var html = '<div class="atlas-card' + (w.active ? " atlas-card--active" : "") + '">';
      html += '<div class="atlas-card__name">' + escapeHTML(w.title || w.slug) + active + "</div>";
      html += '<div class="atlas-card__meta">' + w.entities + " " + (dict["atlas.entities"] || "сущностей") + " · " + w.sessions + " " + (dict["atlas.sessions"] || "сессий") + "</div>";
      html += '<div class="atlas-card__actions">';
      html += '<button type="button" data-action="new-life" data-slug="' + escapeHTML(w.slug) + '">' + (dict["atlas.actions.new_life"] || "Начать новую жизнь") + "</button>";
      if (!w.active) {
        html += '<button type="button" data-action="switch" data-slug="' + escapeHTML(w.slug) + '">' + (dict["atlas.actions.switch"] || "Переключить мир") + "</button>";
      }
      html += '<button type="button" data-action="delete" data-slug="' + escapeHTML(w.slug) + '">' + (dict["atlas.actions.delete"] || "Удалить мир") + "</button>";
      html += '<button type="button" data-action="play" data-slug="' + escapeHTML(w.slug) + '">' + (dict["atlas.actions.play"] || "Играть сейчас") + "</button>";
      html += "</div></div>";
      return html;
    }).join("");
  }

  function onAtlasWorldsClick(event) {
    var btn = event.target.closest("button[data-action]");
    if (!btn) {
      return;
    }
    var action = btn.getAttribute("data-action");
    var slug = btn.getAttribute("data-slug") || "";
    if (action === "switch") {
      switchAtlasWorld(slug);
    } else if (action === "delete") {
      deleteAtlasWorld(slug);
    } else if (action === "play") {
      playAtlasWorld(slug);
    } else if (action === "new-life") {
      openAtlasWizard(slug);
    }
  }

  function switchAtlasWorld(slug) {
    postJSON("/api/atlas/worlds/active", { world: slug }).then(function () {
      window.__activeSlug = slug;
      loadAtlasWorlds();
    }).catch(atlasError);
  }

  function playAtlasWorld(slug) {
    postJSON("/api/atlas/worlds/active", { world: slug }).then(function () {
      window.__activeSlug = slug;
      showChat();
    }).catch(atlasError);
  }

  function deleteAtlasWorld(slug) {
    if (!window.confirm("Удалить мир " + slug + "?")) {
      return;
    }
    var path = "/api/atlas/worlds/delete?world=" + encodeURIComponent(slug) + "&confirm=" + encodeURIComponent(slug);
    apiJSON(path, { method: "DELETE" }).then(loadAtlasWorlds).catch(atlasError);
  }

  function openAtlasWizard(slug) {
    var wizard = byId(IDS.atlasWizard);
    if (!wizard) {
      return;
    }
    wizard.setAttribute("data-slug", slug);
    wizard.hidden = false;
    var isekai = byId(IDS.atlasWizardIsekai);
    if (isekai) {
      isekai.checked = true;
    }
  }

  function startAtlasWizard() {
    var wizard = byId(IDS.atlasWizard);
    var slug = wizard ? (wizard.getAttribute("data-slug") || "") : "";
    var synopsis = fieldValue(IDS.atlasWizardSynopsis) ||
      "Новая жизнь в этом мире. Герой возвращается. Финал неизвестен.";
    var hero = fieldValue(IDS.atlasWizardName) || fieldValue(IDS.atlasWizardHints) || "простолюдин";
    var body = {
      slug: slug,
      title: "Переигровка",
      description: "",
      synopsis: synopsis,
      prologue: "",
      lang: "ru",
      travel_mode: "вымышленная",
      genres: ["фэнтези"],
      world_rules: ["переигровка: новая жизнь"],
      social_order: "феодализм",
      economy_mods: [],
      magic: "",
      hero_birth: hero,
      censorship: "16+"
    };
    postJSON("/api/atlas/worlds/replay", body).then(function () {
      if (wizard) {
        wizard.hidden = true;
      }
      loadAtlasWorlds();
    }).catch(atlasError);
  }

  function submitAtlasBirth() {
    var body = {
      slug: fieldValue(IDS.atlasBirthSlug),
      title: fieldValue(IDS.atlasBirthTitle),
      description: fieldValue(IDS.atlasBirthDescription),
      synopsis: fieldValue(IDS.atlasBirthSynopsis),
      prologue: fieldValue(IDS.atlasBirthPrologue),
      lang: fieldValue(IDS.atlasBirthLang) || "ru",
      travel_mode: fieldValue(IDS.atlasBirthTravelMode) || "вымышленная",
      genres: splitList(fieldValue(IDS.atlasBirthGenres)),
      world_rules: splitLines(fieldValue(IDS.atlasBirthWorldRules)),
      social_order: fieldValue(IDS.atlasBirthSocialOrder),
      economy_mods: splitList(fieldValue(IDS.atlasBirthEconomyMods)),
      magic: fieldValue(IDS.atlasBirthMagic),
      hero_birth: fieldValue(IDS.atlasBirthHeroBirth),
      censorship: fieldValue(IDS.atlasBirthCensorship) || "16+"
    };
    postJSON("/api/atlas/worlds/birth", body).then(function () {
      var form = byId(IDS.atlasBirthForm);
      if (form) {
        form.reset();
      }
      loadAtlasWorlds();
    }).catch(atlasError);
  }

  function atlasError(err) {
    var box = byId(IDS.atlasWorlds);
    if (box) {
      box.textContent = err.message;
    }
  }

  var bookLoaded = false;
  var bookWindow = null;

  function openBookWindow() {
    if (bookLoaded) {
      showBookWindow();
      loadBookData();
      return;
    }
    fetch("/history.html").then(function (res) {
      if (!res.ok) {
        throw new Error("history not found");
      }
      return res.text();
    }).then(function (html) {
      viewHistory.innerHTML = html;
      translate(viewHistory);
      bindBookWindow();
      bookLoaded = true;
      showBookWindow();
      loadBookData();
    }).catch(function (err) {
      viewHistory.textContent = err.message;
    });
  }

  function showBookWindow() {
    if (!bookWindow) {
      return;
    }
    bookWindow.hidden = false;
    switchBookTab("toc");
  }

  function closeBookWindow() {
    if (bookWindow) {
      bookWindow.hidden = true;
    }
  }

  function bindBookWindow() {
    bookWindow = viewHistory.querySelector(".book-modal");
    var close = byId(IDS.bookClose);
    if (close) {
      close.addEventListener("click", closeBookWindow);
    }
    var tabs = viewHistory.querySelectorAll("[data-book-tab]");
    tabs.forEach(function (tab) {
      tab.addEventListener("click", function () {
        switchBookTab(tab.getAttribute("data-book-tab"));
      });
    });
    var form = byId(IDS.bookExport);
    if (form) {
      form.addEventListener("submit", function (event) {
        event.preventDefault();
        submitBookExport();
      });
    }
  }

  function switchBookTab(name) {
    viewHistory.querySelectorAll("[data-book-tab]").forEach(function (tab) {
      tab.classList.toggle("active", tab.getAttribute("data-book-tab") === name);
    });
    viewHistory.querySelectorAll("[data-book-panel]").forEach(function (panel) {
      panel.hidden = panel.getAttribute("data-book-panel") !== name;
    });
  }

  function loadBookData() {
    apiJSON("/api/book/window?world=" + encodeURIComponent(activeSlugValue())).then(renderBookWindow).catch(function (err) {
      var digest = byId(IDS.bookDigest);
      if (digest) {
        digest.textContent = err.message;
      }
    });
  }

  function activeSlugValue() {
    // Активный мир интерфейса хранит Атлас; до выбора мира книга опирается на
    // выбор Атласа. Пустой мир даёт обработчику мира 400, и окно покажет ошибку.
    return window.__activeSlug || "";
  }

  function renderBookWindow(data) {
    var dict = window.I18N || {};
    var digest = byId(IDS.bookDigest);
    if (digest) {
      digest.textContent = data.digest || "";
    }

    var toc = byId(IDS.bookToc);
    if (toc) {
      var entries = data.toc || [];
      if (entries.length === 0) {
        toc.innerHTML = '<li data-i18n="book.toc.empty">' + (dict["book.toc.empty"] || "Обработанных глав пока нет.") + "</li>";
      } else {
        toc.innerHTML = entries.map(function (entry) {
          return "<li data-day=\"" + entry.day + "\">" + escapeHTML(entry.title) + "</li>";
        }).join("");
      }
    }

    var title = byId(IDS.bookChapterTitle);
    if (title) {
      title.textContent = data.chapter ? data.chapter.title : "";
    }
    var text = byId(IDS.bookChapterText);
    if (text) {
      if (data.chapter && data.chapter.text) {
        text.textContent = data.chapter.text;
      } else {
        text.textContent = dict["book.chapter.empty"] || "Обработанная глава появится здесь после полной обработки.";
      }
    }

    var cuts = byId(IDS.bookCuts);
    if (cuts) {
      var items = data.censor_cuts || [];
      if (items.length === 0) {
        cuts.innerHTML = '<li data-i18n="book.censor.empty">' + (dict["book.censor.empty"] || "Правок Цензора пока нет.") + "</li>";
      } else {
        cuts.innerHTML = items.map(function (cut) {
          var arrow = cut.before ? escapeHTML(cut.before) + " → " : "";
          return "<li>" + arrow + escapeHTML(cut.after) + "</li>";
        }).join("");
      }
    }

    var raw = byId(IDS.bookRaw);
    if (raw) {
      var log = data.raw_log || [];
      if (log.length === 0) {
        raw.innerHTML = '<li data-i18n="book.raw.empty">' + (dict["book.raw.empty"] || "Записей лога пока нет.") + "</li>";
      } else {
        raw.innerHTML = log.map(function (entry) {
          return "<li>" + escapeHTML(entry.text) + "</li>";
        }).join("");
      }
    }
  }

  function submitBookExport() {
    var statusEl = byId(IDS.bookExportStatus);
    var scope = byId(IDS.bookExportScope);
    var chapter = byId(IDS.bookExportChapter);
    var path = "/api/book/export?world=" + encodeURIComponent(activeSlugValue()) +
      "&format=md&scope=" + encodeURIComponent(scope ? scope.value : "book");
    if (scope && scope.value === "chapter" && chapter && chapter.value) {
      path += "&chapter=" + encodeURIComponent(chapter.value);
    }
    if (statusEl) {
      statusEl.textContent = "";
    }
    apiJSON(path).then(function (res) {
      if (statusEl) {
        statusEl.textContent = res.path || "ok";
      }
    }).catch(function (err) {
      if (statusEl) {
        statusEl.textContent = err.message;
      }
    });
  }

  var bookButton = document.createElement("button");
  bookButton.type = "button";
  bookButton.id = IDS.bookButton;
  bookButton.className = "book-button";
  bookButton.textContent = "BOOK";
  bookButton.addEventListener("click", openBookWindow);
  chatLog.appendChild(bookButton);

  var mcpLoaded = false;

  function loadMcpView() {
    if (mcpLoaded) {
      loadMcpData();
      return;
    }
    fetch("/mcp.html").then(function (res) {
      if (!res.ok) {
        throw new Error("mcp not found");
      }
      return res.text();
    }).then(function (html) {
      viewMcp.innerHTML = html;
      translate(viewMcp);
      bindMcp();
      mcpLoaded = true;
      loadMcpData();
    }).catch(function (err) {
      viewMcp.textContent = err.message;
    });
  }

  function bindMcp() {
    var form = byId(IDS.mcpImportForm);
    if (form) {
      form.addEventListener("submit", function (event) {
        event.preventDefault();
        submitMcpImport();
      });
    }
    var forge = byId(IDS.mcpForge);
    if (forge) {
      forge.addEventListener("click", startMcpForge);
    }
    var books = byId(IDS.mcpBooks);
    if (books) {
      books.addEventListener("click", onMcpBooksClick);
    }
  }

  function loadMcpData() {
    apiJSON("/api/mcp/dash?world=" + encodeURIComponent(activeSlugValue())).then(renderMcpDash).catch(function (err) {
      var box = byId(IDS.mcpDashQueue);
      if (box) {
        box.textContent = err.message;
      }
    });
    loadMcpSources();
    loadMcpRings();
  }

  function renderMcpDash(data) {
    var dict = window.I18N || {};
    var queue = byId(IDS.mcpDashQueue);
    if (queue) {
      queue.textContent = (dict["mcp.forge.queue"] || "Очередь наполнения") + ": " + (data.queue || 0) + " " + (dict["mcp.units"] || "шт.");
    }
    var current = byId(IDS.mcpDashCurrent);
    if (current) {
      current.textContent = (dict["mcp.forge.current"] || "Текущая работа") + ": " + Math.round((data.current || 0) * 100) + "%";
    }
    var stats = byId(IDS.mcpDashStats);
    if (stats) {
      var s = data.stats || {};
      var labels = [
        ["books_processed", "mcp.stats.books", "Книг обработано"],
        ["chapters_parsed", "mcp.stats.chapters", "Глав разобрано"],
        ["templates", "mcp.stats.templates", "Шаблонов снято"],
        ["censor_rejects", "mcp.stats.censor", "Отклонено Цензором"],
        ["delex_replaces", "mcp.stats.delex", "Замен при делексификации"],
        ["errors", "mcp.stats.errors", "Ошибок"]
      ];
      stats.innerHTML = labels.map(function (item) {
        var key = item[0];
        var label = dict[item[1]] || item[2];
        return "<div class=\"mcp-stat\"><dt>" + escapeHTML(label) + "</dt><dd>" + (s[key] || 0) + "</dd></div>";
      }).join("");
    }
  }

  function loadMcpSources() {
    apiJSON("/api/mcp/sources?world=" + encodeURIComponent(activeSlugValue())).then(renderMcpSources).catch(function (err) {
      var box = byId(IDS.mcpBooks);
      if (box) {
        box.textContent = err.message;
      }
    });
  }

  function renderMcpSources(data) {
    var dict = window.I18N || {};
    var box = byId(IDS.mcpBooks);
    if (!box) {
      return;
    }
    var items = data.sources || [];
    if (items.length === 0) {
      box.innerHTML = '<p data-i18n="mcp.books.empty">' + (dict["mcp.books.empty"] || "Книг пока нет.") + "</p>";
      return;
    }
    box.innerHTML = items.map(function (item) {
      var status = item.status || "queued";
      if (status === "forging") {
        status = status + " " + Math.round((item.progress || 0) * 100) + "%";
      }
      var html = '<div class="mcp-book" data-id="' + escapeHTML(item.id || item.name) + '">';
      html += '<div class="mcp-book__name">' + escapeHTML(item.name) + ' <span class="mcp-book__format">' + escapeHTML(item.format) + "</span></div>";
      html += '<div class="mcp-book__status">' + escapeHTML(status) + "</div>";
      html += '<div class="mcp-book__counters">' + (item.chapters || 0) + " " + (dict["mcp.books.chapters"] || "глав") + " · " + (item.templates || 0) + " " + (dict["mcp.books.templates"] || "шаблонов") + "</div>";
      html += '<button type="button" class="mcp-book__delete" data-action="delete-source" data-id="' + escapeHTML(item.id || item.name) + '">' + (dict["mcp.books.delete"] || "Удалить сырьё") + "</button>";
      html += "</div>";
      return html;
    }).join("");
  }

  function submitMcpImport() {
    var fileEl = byId(IDS.mcpImportFile);
    var file = fileEl && fileEl.files ? fileEl.files[0] : null;
    if (!file) {
      return;
    }
    var form = new FormData();
    form.append("world", activeSlugValue());
    form.append("file", file);
    fetch("/api/mcp/import", { method: "POST", body: form, headers: { Accept: "application/json" } })
      .then(function (res) {
        return res.json().catch(function () {
          return {};
        }).then(function (body) {
          if (!res.ok) {
            var msg = (body && body.error && body.error.message) || res.statusText;
            throw new Error(msg);
          }
          return body;
        });
      })
      .then(renderMcpSources)
      .catch(function (err) {
        var box = byId(IDS.mcpBooks);
        if (box) {
          box.textContent = err.message;
        }
      });
  }

  function startMcpForge() {
    postJSON("/api/mcp/forge", { world: activeSlugValue() }).then(function () {
      loadMcpSources();
      loadMcpData();
    }).catch(function (err) {
      var box = byId(IDS.mcpBooks);
      if (box) {
        box.textContent = err.message;
      }
    });
  }

  function onMcpBooksClick(event) {
    var btn = event.target.closest("button[data-action]");
    if (!btn || btn.getAttribute("data-action") !== "delete-source") {
      return;
    }
    var id = btn.getAttribute("data-id") || "";
    if (!window.confirm("Удалить сырьё " + id + "?")) {
      return;
    }
    apiJSON("/api/mcp/source/" + encodeURIComponent(id) + "?world=" + encodeURIComponent(activeSlugValue()), { method: "DELETE" })
      .then(renderMcpSources)
      .catch(function (err) {
        var box = byId(IDS.mcpBooks);
        if (box) {
          box.textContent = err.message;
        }
      });
  }

  function loadMcpRings() {
    apiJSON("/api/mcp/rings?world=" + encodeURIComponent(activeSlugValue())).then(renderMcpRings).catch(function (err) {
      var box = byId(IDS.mcpRings);
      if (box) {
        box.textContent = err.message;
      }
    });
  }

  // Живое обновление вкладки MCP (UI-07.2): подписка на WS-топик mcp; событие
  // импорта/ковки/удаления перезагружает дашборд и карточки.
  function subscribeMcpWS() {
    if (!window.WebSocket) {
      return;
    }
    var proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    var ws = new WebSocket(proto + "//" + window.location.host + "/ws");
    ws.onopen = function () {
      ws.send(JSON.stringify({ type: "subscribe", topics: ["mcp"] }));
    };
    ws.onmessage = function (event) {
      var msg = null;
      try {
        msg = JSON.parse(event.data);
      } catch (err) {
        return;
      }
      if (msg && msg.topic === "mcp") {
        loadMcpData();
      }
    };
    ws.onclose = function () {
      setTimeout(subscribeMcpWS, 1000);
    };
  }

  function renderMcpRings(data) {
    var dict = window.I18N || {};
    var box = byId(IDS.mcpRings);
    if (!box) {
      return;
    }
    var j = data.journal || {};
    var current = j.current || {};
    var queue = j.queue || [];
    var forge = j.forge || {};
    var html = '<p class="mcp-rings__current">' +
      (dict["mcp.rings.current"] || "Текущее кольцо") + ": " + (current.status || "—") + " " + Math.round((current.progress || 0) * 100) + "%" +
      " · " + (current.places_count || 0) + " " + (dict["mcp.rings.places"] || "мест") +
      " · " + (current.paths_count || 0) + " " + (dict["mcp.rings.paths"] || "путей") +
      " · " + (current.people_count || 0) + " " + (dict["mcp.rings.people"] || "людей") + "</p>";
    html += '<p class="mcp-rings__queue">' + (dict["mcp.rings.queue"] || "Очередь колец") + ": " + queue.length + "</p>";
    html += '<p class="mcp-rings__forge">' + (dict["mcp.rings.forge"] || "Статусы записей") + ": draft → baked · " +
      (forge.places_draft || 0) + "/" + (forge.places_baked || 0) + " " + (dict["mcp.rings.places"] || "мест") + " · " +
      (forge.paths_draft || 0) + "/" + (forge.paths_baked || 0) + " " + (dict["mcp.rings.paths"] || "путей") + "</p>";
    box.innerHTML = html;
  }

  var settingsLoaded = false;

  function ensureThemeCSS() {
    if (document.querySelector('link[href="/static/theme.css"]')) {
      return;
    }
    var link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = "/static/theme.css";
    document.head.appendChild(link);
  }

  function loadSettingsView() {
    ensureThemeCSS();
    if (settingsLoaded) {
      loadSettingsData();
      return;
    }
    fetch("/settings.html").then(function (res) {
      if (!res.ok) {
        throw new Error("settings not found");
      }
      return res.text();
    }).then(function (html) {
      viewSettings.innerHTML = html;
      translate(viewSettings);
      bindSettings();
      settingsLoaded = true;
      loadSettingsData();
    }).catch(function (err) {
      viewSettings.textContent = err.message;
    });
  }

  function bindSettings() {
    bindSettingsSubmit(IDS.settingsGeneralForm, submitSettingsGeneral);
    bindSettingsSubmit(IDS.settingsLLMForm, submitSettingsLLM);
    bindSettingsSubmit(IDS.settingsProviderForm, submitSettingsProvider);
    bindSettingsSubmit(IDS.settingsModelsForm, submitSettingsModel);
    bindSettingsSubmit(IDS.settingsRolesForm, submitSettingsRoles);
    bindSettingsSubmit(IDS.settingsWorldForm, submitSettingsWorld);
    bindSettingsSubmit(IDS.settingsAdvancedForm, submitSettingsAdvanced);
    var theme = byId(IDS.settingsTheme);
    if (theme) {
      theme.addEventListener("change", applyTheme);
    }
    var modelsBox = byId(IDS.settingsModels);
    if (modelsBox) {
      modelsBox.addEventListener("click", function (event) {
        var btn = event.target.closest("button[data-model-role]");
        if (!btn) {
          return;
        }
        rollbackSettingsModel(btn.getAttribute("data-model-role"));
      });
    }
  }

  function bindSettingsSubmit(id, handler) {
    var form = byId(id);
    if (form) {
      form.addEventListener("submit", function (event) {
        event.preventDefault();
        handler();
      });
    }
  }

  function applyTheme() {
    var theme = byId(IDS.settingsTheme);
    var value = theme ? theme.value : "default";
    document.body.classList.remove("theme-dark", "theme-light");
    if (value === "dark" || value === "light") {
      document.body.classList.add("theme-" + value);
    }
    var preview = byId(IDS.settingsThemePreview);
    if (preview) {
      preview.textContent = value;
    }
  }

  function loadSettingsData() {
    apiJSON("/api/settings?world=" + encodeURIComponent(activeSlugValue())).then(renderSettings).catch(function (err) {
      var box = byId(IDS.settingsEmbedDim);
      if (box) {
        box.textContent = err.message;
      }
    });
  }

  function renderSettings(data) {
    setValue(IDS.settingsServerHost, data.general ? data.general.server_host : "");
    setValue(IDS.settingsServerPort, data.general ? String(data.general.server_port) : "");
    setValue(IDS.settingsDBPath, data.general ? data.general.db_path : "");
    setValue(IDS.settingsAuth, data.general ? data.general.auth : "");
    setValue(IDS.settingsEmbedDim, String(data.embedding_dimension || ""));
    setValue(IDS.settingsMemoryMax, data.advanced ? String(data.advanced.maxMemoryEntries || 0) : "");
    setValue(IDS.settingsMemorySimilarity, data.advanced ? String(data.advanced.similarityThreshold || 0) : "");
    setValue(IDS.settingsMemoryHalflife, data.advanced ? String(data.advanced.halfLifeDays || 0) : "");
    setValue(IDS.settingsGlobalLuck, data.advanced ? String(data.advanced.globalLuck || 0) : "");
    setChecked(IDS.settingsVectorSearch, data.general ? !!data.general.external_vector_search : false);
    setChecked(IDS.settingsPaganelEnabled, data.world_section && data.world_section.paganel ? !!data.world_section.paganel.enabled : false);
    setChecked(IDS.settingsScoutEnabled, data.world_section && data.world_section.paganel ? !!data.world_section.paganel.scout_enabled : false);
    renderLLMSettings(data.llm || {});
    renderSettingsPresets(data.presets || []);
    renderSettingsProviders(data.providers || []);
    renderSettingsBindings(data.bindings || []);
    renderSettingsModels(data.models || []);
    renderEmptyState(IDS.settingsPlugins, "settings.plugins");
    renderEmptyState(IDS.settingsFlags, "settings.flags");
  }

  function renderLLMSettings(llm) {
    var gen = llm.gen || {};
    var embed = llm.embed || {};
    setValue(IDS.settingsGenAddr, gen.addr || "");
    setValue(IDS.settingsGenModel, gen.model || "");
    setValue(IDS.settingsGenTimeout, gen.timeout || "");
    setValue(IDS.settingsGenMaxTokens, String(gen.max_tokens || 0));
    setValue(IDS.settingsGenTemperature, String(gen.temperature || 0));
    setValue(IDS.settingsGenRetries, String(gen.retries || 0));
    setValue(IDS.settingsGenConcurrency, String(gen.concurrency || 1));
    setValue(IDS.settingsEmbAddr, embed.addr || "");
    setValue(IDS.settingsEmbModel, embed.model || "");
    setValue(IDS.settingsEmbTimeout, embed.timeout || "");
    setChecked(IDS.settingsLLMLocal, llm.local !== false);
    setValue(IDS.settingsLLMSampling, llm.sampling || "greedy");
    setKeyPlaceholder(IDS.settingsGenKey, gen.key_set);
    setKeyPlaceholder(IDS.settingsEmbKey, embed.key_set);
  }

  function setKeyPlaceholder(id, keySet) {
    var el = byId(id);
    if (el) {
      el.placeholder = keySet ? "ключ задан" : "";
    }
  }

  function renderSettingsPresets(presets) {
    var box = byId(IDS.settingsPresets);
    if (!box) {
      return;
    }
    if (presets.length === 0) {
      box.textContent = "";
      return;
    }
    box.innerHTML = presets.map(function (p) {
      return '<div class="settings-preset">' + escapeHTML(p.provider) +
        (p.gen_addr ? " · " + escapeHTML(p.gen_addr) : "") +
        (p.embed_addr ? " · " + escapeHTML(p.embed_addr) : "") + "</div>";
    }).join("");
  }

  function renderSettingsProviders(providers) {
    var box = byId(IDS.settingsProviders);
    if (!box) {
      return;
    }
    if (providers.length === 0) {
      box.innerHTML = '<p class="settings-empty">—</p>';
      return;
    }
    box.innerHTML = providers.map(function (p) {
      return '<div class="settings-provider">' + escapeHTML(p.name) + " · " + (p.key_set ? "ключ задан" : "без ключа") + "</div>";
    }).join("");
  }

  function renderSettingsBindings(bindings) {
    var box = byId(IDS.settingsRoles);
    if (!box) {
      return;
    }
    if (bindings.length === 0) {
      box.innerHTML = '<p class="settings-empty">—</p>';
      return;
    }
    box.innerHTML = bindings.map(function (b) {
      var html = '<div class="settings-role">' + escapeHTML(b.role) + " · " + escapeHTML(b.provider) + " · " + escapeHTML(b.model_ref || "");
      if (b.temperature) {
        html += " · t=" + escapeHTML(String(b.temperature));
      }
      if (b.prompts) {
        html += " · " + escapeHTML(b.prompts);
      }
      if (b.priority) {
        html += " · p=" + escapeHTML(String(b.priority));
      }
      html += "</div>";
      return html;
    }).join("");
  }

  function renderSettingsModels(models) {
    var box = byId(IDS.settingsModels);
    if (!box) {
      return;
    }
    if (!models || models.length === 0) {
      box.innerHTML = '<p class="settings-empty">—</p>';
      return;
    }
    var dict = window.I18N || {};
    box.innerHTML = models.map(function (m) {
      var html = '<div class="settings-model">' + escapeHTML(m.role) + " · " + escapeHTML(m.model_ref || "");
      if (m.status) {
        html += " · " + escapeHTML(m.status);
      }
      if (typeof m.progress === "number") {
        html += " · " + Math.round(m.progress * 100) + "%";
      }
      html += " · " + (m.integrity_ok ? "целостность ок" : "целостность нарушена");
      if (m.has_previous) {
        html += ' <button type="button" data-model-role="' + escapeHTML(m.role) + '">' + (dict["settings.models.rollback"] || "Откатить") + "</button>";
      }
      html += "</div>";
      return html;
    }).join("");
  }

  function renderEmptyState(id) {
    var box = byId(id);
    if (box) {
      box.innerHTML = '<p class="settings-empty">—</p>';
    }
  }

  function setValue(id, value) {
    var el = byId(id);
    if (!el) {
      return;
    }
    if (el.tagName === "INPUT" || el.tagName === "SELECT" || el.tagName === "TEXTAREA") {
      el.value = value;
    } else {
      el.textContent = value;
    }
  }

  function setChecked(id, value) {
    var el = byId(id);
    if (el) {
      el.checked = value;
    }
  }

  function submitSettingsGeneral() {
    putJSON("/api/settings/config", {
      host: fieldValue(IDS.settingsServerHost),
      port: parseInt(fieldValue(IDS.settingsServerPort), 10) || 0
    }).then(loadSettingsData).catch(settingsError);
  }

  function submitSettingsLLM() {
    var config = {
      gen_addr: fieldValue(IDS.settingsGenAddr),
      gen_model: fieldValue(IDS.settingsGenModel),
      gen_timeout: fieldValue(IDS.settingsGenTimeout),
      gen_max_tokens: parseInt(fieldValue(IDS.settingsGenMaxTokens), 10) || 0,
      gen_temperature: parseFloat(fieldValue(IDS.settingsGenTemperature)) || 0,
      gen_retries: parseInt(fieldValue(IDS.settingsGenRetries), 10) || 0,
      gen_concurrency: parseInt(fieldValue(IDS.settingsGenConcurrency), 10) || 1,
      embed_addr: fieldValue(IDS.settingsEmbAddr),
      embed_model: fieldValue(IDS.settingsEmbModel),
      embed_timeout: fieldValue(IDS.settingsEmbTimeout),
      local: byId(IDS.settingsLLMLocal) ? byId(IDS.settingsLLMLocal).checked : true,
      sampling: fieldValue(IDS.settingsLLMSampling) || "greedy"
    };
    var keys = {};
    var genKey = fieldValue(IDS.settingsGenKey);
    var embedKey = fieldValue(IDS.settingsEmbKey);
    if (genKey) {
      keys.llm_gen = genKey;
    }
    if (embedKey) {
      keys.llm_embed = embedKey;
    }
    putJSON("/api/settings/config", config).then(function () {
      if (Object.keys(keys).length === 0) {
        return loadSettingsData();
      }
      return putJSON("/api/settings/keys", { keys: keys }).then(loadSettingsData);
    }).catch(settingsError);
  }

  function submitSettingsWorld() {
    putJSON("/api/settings/config", {
      paganel_enabled: byId(IDS.settingsPaganelEnabled) ? byId(IDS.settingsPaganelEnabled).checked : false,
      scout_enabled: byId(IDS.settingsScoutEnabled) ? byId(IDS.settingsScoutEnabled).checked : false
    }).then(function () {
      var key = fieldValue(IDS.settingsPaganelKey);
      if (!key) {
        return loadSettingsData();
      }
      return putJSON("/api/settings/keys", { keys: { paganel: key } }).then(loadSettingsData);
    }).catch(settingsError);
  }

  function submitSettingsAdvanced() {
    putJSON("/api/world/tunables", {
      world: activeSlugValue(),
      tunables: {
        globalLuck: parseFloat(fieldValue(IDS.settingsGlobalLuck)) || 0,
        maxMemoryEntries: parseInt(fieldValue(IDS.settingsMemoryMax), 10) || 0,
        similarityThreshold: parseFloat(fieldValue(IDS.settingsMemorySimilarity)) || 0,
        halfLifeDays: parseFloat(fieldValue(IDS.settingsMemoryHalflife)) || 0
      }
    }).then(loadSettingsData).catch(settingsError);
  }

  function submitSettingsProvider() {
    var name = fieldValue(IDS.settingsProviderName);
    var key = fieldValue(IDS.settingsProviderKey);
    if (!name || !key) {
      return;
    }
    var body = { keys: {} };
    body.keys[name] = key;
    putJSON("/api/settings/keys", body).then(loadSettingsData).catch(settingsError);
  }

  function submitSettingsModel() {
    var role = fieldValue(IDS.settingsModelRole);
    var source = fieldValue(IDS.settingsModelSource);
    var ref = fieldValue(IDS.settingsModelRef);
    if (!role || !source || !ref) {
      return;
    }
    postJSON("/api/settings/models/install", {
      world: activeSlugValue(),
      role: role,
      source: source,
      model_ref: ref
    }).then(function () {
      loadSettingsData();
    }).catch(settingsError);
  }

  function rollbackSettingsModel(role) {
    if (!role) {
      return;
    }
    postJSON("/api/settings/models/rollback", {
      world: activeSlugValue(),
      role: role
    }).then(function () {
      loadSettingsData();
    }).catch(settingsError);
  }

  function submitSettingsRoles() {
    var role = fieldValue(IDS.settingsRoleName);
    var provider = fieldValue(IDS.settingsRoleProvider);
    var model = fieldValue(IDS.settingsRoleModel);
    if (!role || !provider) {
      return;
    }
    putJSON("/api/settings/bindings", {
      world: activeSlugValue(),
      bindings: [{
        role: role,
        model_ref: model,
        provider: provider,
        temperature: parseFloat(fieldValue(IDS.settingsRoleTemperature)) || 0,
        prompts: fieldValue(IDS.settingsRolePrompts),
        priority: parseInt(fieldValue(IDS.settingsRolePriority), 10) || 0
      }]
    }).then(loadSettingsData).catch(settingsError);
  }

  function settingsError(err) {
    var box = byId(IDS.settingsEmbedDim);
    if (box) {
      box.textContent = err.message;
    }
  }

  translate(document);
  showChat();
  subscribeMcpWS();
})();
