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
    settingsChannelForm: "settings-channel-form",
    settingsChannelEdge: "settings-channel-edge",
    settingsChannelLocal: "settings-channel-local",
    settingsChannelChatId: "settings-channel-chat-id",
    settingsChannelToken: "settings-channel-token",
    settingsChannelConfigured: "settings-channel-configured",
    settingsChannelBot: "settings-channel-bot",
    settingsChannelWarning: "settings-channel-warning",
    settingsChannelSave: "settings-channel-save",
    settingsPlugins: "settings-plugins",
    settingsAdvancedForm: "settings-advanced-form",
    settingsFlags: "settings-flags",
    settingsMemoryMax: "settings-memory-max",
    settingsMemorySimilarity: "settings-memory-similarity",
    settingsMemoryHalflife: "settings-memory-halflife",
    settingsEmbedDim: "settings-embed-dim",
    settingsGlobalLuck: "settings-global-luck",
    settingsAdvancedSave: "settings-advanced-save",
    monitorSlots: "monitor-slots",
    monitorRoles: "monitor-roles",
    monitorMachines: "monitor-machines",
    monitorDegradations: "monitor-degradations",
    monitorJournal: "monitor-journal",
    settingsNav: "settings-nav",
    settingsSectionGeneral: "settings-section-general",
    settingsSectionLLM: "settings-section-llm",
    settingsSectionProviders: "settings-section-providers",
    settingsSectionModels: "settings-section-models",
    settingsSectionRoles: "settings-section-roles",
    settingsSectionWorld: "settings-section-world",
    settingsSectionChannel: "settings-section-channel",
    settingsSectionPlugins: "settings-section-plugins",
    settingsSectionAdvanced: "settings-section-advanced",
    settingsSectionMonitor: "settings-section-monitor",
    settingsDayNorm: "settings-day-norm",
    modelCatalog: "model-catalog",
    modelCatalogGrid: "model-catalog-grid",
    modelCatalogHint: "model-catalog-hint",
    modelFileBrowser: "model-file-browser",
    modelLocalPath: "model-local-path",
    modelLocalApply: "model-local-apply",
    modelLocalFile: "model-local-file",
    modelLocalPick: "model-local-pick",
    modelLocalSelected: "model-local-selected"
  };

  var SETTINGS_SECTIONS = [
    "settings-section-general",
    "settings-section-llm",
    "settings-section-providers",
    "settings-section-models",
    "settings-section-roles",
    "settings-section-world",
    "settings-section-channel",
    "settings-section-plugins",
    "settings-section-advanced",
    "settings-section-monitor"
  ];

  var THEME_STORAGE_KEY = "tns65.theme";

  var MODEL_CATALOG = [
    { role: "weaver", ref: "Llama-3.2-3B-Instruct", source: "huggingface.co/ggml-org/Llama-3.2-3B-Instruct-GGUF", label: "weaver · Llama 3.2 3B (GGUF)" },
    { role: "pythia", ref: "Qwen2.5-7B-Instruct", source: "huggingface.co/Qwen/Qwen2.5-7B-Instruct-GGUF", label: "pythia · Qwen 2.5 7B" },
    { role: "editor", ref: "Llama-3.1-8B-Instruct", source: "huggingface.co/bartowski/Llama-3.1-8B-Instruct-GGUF", label: "editor · Llama 3.1 8B" },
    { role: "analysis", ref: "Qwen2.5-3B-Instruct", source: "huggingface.co/Qwen/Qwen2.5-3B-Instruct-GGUF", label: "analysis · Qwen 2.5 3B" }
  ];

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
    // Роут чата (T-08-15): при входе обновляем состояние и сводку при возврате.
    if (window.Chat && typeof window.Chat.refresh === "function") {
      window.Chat.refresh();
    }
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
  bookButton.className = "book-btn";
  bookButton.setAttribute("aria-label", (window.I18N && window.I18N["book.title"]) || "Моя книга");
  bookButton.innerHTML = '<svg class="ic" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M12 6.5C9.8 5 6.8 4.6 4 5v13c2.8-.4 5.8 0 8 1.5 2.2-1.5 5.2-1.9 8-1.5V5c-2.8-.4-5.8 0-8 1.5Z"/></svg>';
  bookButton.addEventListener("click", openBookWindow);
  var composerDock = document.querySelector(".composer-in");
  if (composerDock) {
    composerDock.insertBefore(bookButton, composerDock.firstChild);
  } else {
    chatLog.appendChild(bookButton);
  }

  var mcpLoaded = false;
  var mcpScriptLoaded = false;

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
      loadMcpScript();
      mcpLoaded = true;
      loadMcpData();
    }).catch(function (err) {
      viewMcp.textContent = err.message;
    });
  }

  // coeffs.js подключается явным элементом script: вставленные через innerHTML
  // теги <script src> браузер не исполняет, поэтому логика коэффициентов и
  // лемматизатора грузится отдельно после вставки разметки вкладки.
  function loadMcpScript() {
    if (mcpScriptLoaded) {
      return;
    }
    mcpScriptLoaded = true;
    var script = document.createElement("script");
    script.src = "/static/coeffs.js";
    document.body.appendChild(script);
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
      var html = '<div class="mcp-book" data-id="' + escapeHTML(item.id || item.name) + '">';
      html += '<div class="mcp-book__name">' + escapeHTML(item.name) + ' <span class="mcp-book__format">' + escapeHTML(item.format) + "</span></div>";
      html += mcpSourceStatusPill(item.status, item.progress);
      html += '<div class="mcp-book__counters">' + (item.chapters || 0) + " " + (dict["mcp.books.chapters"] || "глав") + " · " + (item.templates || 0) + " " + (dict["mcp.books.templates"] || "шаблонов") + "</div>";
      html += '<button type="button" class="mcp-book__delete" data-action="delete-source" data-id="' + escapeHTML(item.id || item.name) + '">' + (dict["mcp.books.delete"] || "Удалить сырьё") + "</button>";
      html += "</div>";
      return html;
    }).join("");
  }

  // Пилюля статуса книги-источника: слово + точка в одном из четырёх состояний
  // (ok/wait/err/off). Прогресс ковки остаётся в подписи, данные не меняются.
  function mcpSourceStatusPill(status, progress) {
    var base = status || "queued";
    var label = (window.I18N && window.I18N["mcp.books.status." + base]) || base;
    if (base === "forging") {
      label = label + " " + Math.round((progress || 0) * 100) + "%";
    }
    var cls = base === "done" ? "status--ok"
      : base === "error" ? "status--err"
      : base === "forging" || base === "queued" ? "status--wait"
      : "status--off";
    return '<div class="mcp-book__status ' + cls + '">' + escapeHTML(label) + "</div>";
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
      loadMonitorData();
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
      loadMonitorData();
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
    bindSettingsSubmit(IDS.settingsChannelForm, submitChannel);
    bindSettingsSubmit(IDS.settingsAdvancedForm, submitSettingsAdvanced);
    bindSettingsNav();
    bindChannelExtras();

    var theme = byId(IDS.settingsTheme);
    if (theme) {
      theme.value = normalizeTheme(resolveTheme());
      theme.addEventListener("change", applyTheme);
      var preview = byId(IDS.settingsThemePreview);
      if (preview) {
        preview.textContent = theme.value;
      }
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

    renderModelCatalog();
    var grid = byId(IDS.modelCatalogGrid);
    if (grid) {
      grid.addEventListener("click", onModelCatalogClick);
    }
    bindModelFileBrowser();
  }

  function bindSettingsNav() {
    var nav = byId(IDS.settingsNav);
    if (!nav) {
      return;
    }
    var links = nav.querySelectorAll("a");
    function markActive(hash) {
      links.forEach(function (link) {
        link.classList.toggle("active", link.getAttribute("href") === hash);
      });
    }
    nav.addEventListener("click", function (event) {
      var link = event.target.closest("a[href^='#settings-section-']");
      if (link) {
        markActive(link.getAttribute("href"));
      }
    });
    markActive("#" + SETTINGS_SECTIONS[0]);
  }

  function bindChannelExtras() {
    var peek = viewSettings.querySelector(".js-token-peek");
    if (peek) {
      peek.addEventListener("click", function () {
        var input = peek.closest(".input-wrap").querySelector("input");
        if (input) {
          input.type = input.type === "password" ? "text" : "password";
        }
      });
    }
    var check = viewSettings.querySelector(".js-channel-check");
    if (check) {
      check.addEventListener("click", function () {
        loadSettingsData();
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

  function readStoredTheme() {
    try {
      return window.localStorage.getItem(THEME_STORAGE_KEY) || "default";
    } catch (err) {
      return "default";
    }
  }

  // Скриншот-параметр ?theme=dark|light|custom сильнее сохранённого выбора
  // (DOCS/10-STATION-SETTINGS.md §8): он действует только на текущую сессию.
  function readQueryTheme() {
    try {
      var q = new URLSearchParams(window.location.search).get("theme");
      return q === "dark" || q === "light" || q === "custom" ? q : "";
    } catch (err) {
      return "";
    }
  }

  function resolveTheme() {
    return readQueryTheme() || readStoredTheme();
  }

  function writeStoredTheme(value) {
    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, value);
    } catch (err) {
      // localStorage может быть недоступен в приватном режиме.
    }
  }

  function normalizeTheme(value) {
    if (value === "default") {
      return "dark";
    }
    return value === "dark" || value === "light" || value === "custom" ? value : "dark";
  }

  function applyThemeClass(value) {
    document.documentElement.dataset.theme = normalizeTheme(value);
  }

  function applyTheme() {
    var theme = byId(IDS.settingsTheme);
    var value = theme ? theme.value : readStoredTheme();
    var normalized = normalizeTheme(value);
    applyThemeClass(normalized);
    writeStoredTheme(normalized);
    var preview = byId(IDS.settingsThemePreview);
    if (preview) {
      preview.textContent = normalized;
    }
  }

  function loadSettingsData() {
    var slug = activeSlugValue();
    var path = slug ? "/api/settings?world=" + encodeURIComponent(slug) : "/api/settings";
    apiJSON(path).then(renderSettings).then(loadProvidersData).catch(function (err) {
      var box = byId(IDS.settingsEmbedDim);
      if (box) {
        box.textContent = err.message;
      }
    });
  }

  function loadProvidersData() {
    apiJSON("/api/providers").then(function (data) {
      renderSettingsProviders(data.providers || []);
    }).catch(function () {
      renderSettingsProviders([]);
    });
  }

  function loadMonitorData() {
    apiJSON("/api/monitor").then(renderMonitor).catch(function (err) {
      var box = byId(IDS.monitorJournal);
      if (box) {
        box.textContent = err.message;
      }
    });
  }

  function renderMonitor(data) {
    var dict = window.I18N || {};
    renderMonitorSlots(data.slots || [], dict);
    renderMonitorRoles(data.roles || [], dict);
    renderMonitorMachines(data.machines || [], dict);
    renderMonitorDegradations(data.degradations || 0, dict);
    renderMonitorJournal(data.journal_tail || [], dict);
  }

  function renderMonitorSlots(slots, dict) {
    var box = byId(IDS.monitorSlots);
    if (!box) {
      return;
    }
    if (slots.length === 0) {
      box.innerHTML = '<p class="monitor-empty">—</p>';
      return;
    }
    box.innerHTML = slots.map(function (s) {
      var online = s.online ? (dict["settings.monitor.slots.online"] || "онлайн") : (dict["settings.monitor.slots.offline"] || "офлайн");
      var key = s.key_valid ? (dict["settings.monitor.slots.key_valid"] || "ключ валиден") : (dict["settings.monitor.slots.key_missing"] || "ключ не задан");
      var line = '<div class="monitor-row">' + escapeHTML(s.name || "") + " · " + online + " · " + key;
      if (s.limit_remaining) {
        line += " · " + (dict["settings.monitor.slots.limit"] || "остаток лимита") + ": " + escapeHTML(s.limit_remaining);
      }
      if (s.addr) {
        line += " · " + escapeHTML(s.addr);
      }
      line += "</div>";
      return line;
    }).join("");
  }

  function renderMonitorRoles(roles, dict) {
    var box = byId(IDS.monitorRoles);
    if (!box) {
      return;
    }
    if (roles.length === 0) {
      box.innerHTML = '<p class="monitor-empty">' + (dict["settings.monitor.roles.empty"] || "Вызовов ролей пока нет") + "</p>";
      return;
    }
    box.innerHTML = roles.map(function (r) {
      var last = r.last_call_at ? " · " + (dict["settings.monitor.roles.last_call"] || "последний вызов") + ": " + escapeHTML(r.last_call_at) : "";
      return '<div class="monitor-row">' + escapeHTML(r.name || "") +
        " · " + (r.calls || 0) + " " + (dict["settings.monitor.roles.calls"] || "вызовов") +
        " · " + (r.errors || 0) + " " + (dict["settings.monitor.roles.errors"] || "ошибок") +
        " · " + (dict["settings.monitor.roles.error_percent"] || "процент ошибок") + ": " + (r.error_percent || 0) + "%" +
        " · " + (dict["settings.monitor.roles.avg_ms"] || "среднее время ответа") + ": " + (r.avg_response_ms || 0) + " мс" +
        last + "</div>";
    }).join("");
  }

  function renderMonitorMachines(machines, dict) {
    var box = byId(IDS.monitorMachines);
    if (!box) {
      return;
    }
    if (machines.length === 0) {
      box.innerHTML = '<p class="monitor-empty">—</p>';
      return;
    }
    box.innerHTML = machines.map(function (m) {
      var state = m.alive ? (dict["settings.monitor.machines.alive"] || "жива") : (dict["settings.monitor.machines.dead"] || "мертва");
      return '<div class="monitor-row">' + escapeHTML(m.addr || "") + " · " + state +
        " · " + (dict["settings.monitor.machines.queue"] || "очередь") + ": " + (m.queue || 0) + "</div>";
    }).join("");
  }

  function renderMonitorDegradations(n, dict) {
    var box = byId(IDS.monitorDegradations);
    if (!box) {
      return;
    }
    box.innerHTML = '<p class="monitor-row">' + (dict["settings.monitor.degradations.count"] || "Сколько раз ход ушёл на запасной путь") + ": " + (n || 0) + "</p>";
  }

  function renderMonitorJournal(lines, dict) {
    var box = byId(IDS.monitorJournal);
    if (!box) {
      return;
    }
    if (lines.length === 0) {
      box.innerHTML = '<p class="monitor-empty">' + (dict["settings.monitor.journal.empty"] || "Журнал пуст") + "</p>";
      return;
    }
    box.innerHTML = lines.map(function (line) {
      return '<div class="monitor-journal-line">' + escapeHTML(line) + "</div>";
    }).join("");
  }

  function subscribeMonitorWS() {
    if (!window.WebSocket) {
      return;
    }
    var proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    var ws = new WebSocket(proto + "//" + window.location.host + "/ws");
    ws.onopen = function () {
      ws.send(JSON.stringify({ type: "subscribe", topics: ["monitor"] }));
    };
    ws.onmessage = function (event) {
      var msg = null;
      try {
        msg = JSON.parse(event.data);
      } catch (err) {
        return;
      }
      if (msg && msg.topic === "monitor") {
        loadMonitorData();
      }
    };
    ws.onclose = function () {
      setTimeout(subscribeMonitorWS, 1000);
    };
  }

  function renderSettings(data) {
    var hasWorld = !!(data && data.world);
    setValue(IDS.settingsServerHost, data.general ? data.general.server_host : "");
    setValue(IDS.settingsServerPort, data.general ? String(data.general.server_port) : "");
    setValue(IDS.settingsDBPath, data.general ? data.general.db_path : "");
    setValue(IDS.settingsAuth, data.general ? data.general.auth : "");
    setValue(IDS.settingsEmbedDim, String(data.embedding_dimension || ""));
    setValue(IDS.settingsMemoryMax, data.advanced ? String(data.advanced.maxMemoryEntries || 0) : "");
    setValue(IDS.settingsMemorySimilarity, data.advanced ? String(data.advanced.similarityThreshold || 0) : "");
    setValue(IDS.settingsMemoryHalflife, data.advanced ? String(data.advanced.halfLifeDays || 0) : "");
    setValue(IDS.settingsGlobalLuck, data.advanced ? String(data.advanced.globalLuck || 0) : "");
    setValue(IDS.settingsDayNorm, data.advanced ? String(data.advanced.dayNormKm || 0) : "");
    setChecked(IDS.settingsVectorSearch, data.general ? !!data.general.external_vector_search : false);
    setChecked(IDS.settingsPaganelEnabled, data.world_section && data.world_section.paganel ? !!data.world_section.paganel.enabled : false);
    setChecked(IDS.settingsScoutEnabled, data.world_section && data.world_section.paganel ? !!data.world_section.paganel.scout_enabled : false);
    renderLLMSettings(data.llm || {});
    renderChannel(data.channel || {});
    renderSettingsPresets(data.presets || []);
    renderSettingsBindings(data.bindings || [], hasWorld);
    renderSettingsModels(data.models || [], hasWorld);
    renderSettingsPlugins();
    renderSettingsFlags(data.advanced ? data.advanced.feature_flags || [] : []);
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

  function renderChannel(channel) {
    setValue(IDS.settingsChannelEdge, channel.edge || "");
    setValue(IDS.settingsChannelLocal, channel.local || "");
    setValue(IDS.settingsChannelChatId, channel.telegram_chat_id || "");
    setKeyPlaceholder(IDS.settingsChannelToken, channel.telegram_token_set);
    var dict = window.I18N || {};
    setValue(IDS.settingsChannelConfigured, channel.telegram_configured
      ? (dict["settings.channel.configured_yes"] || "настроен")
      : (dict["settings.channel.configured_no"] || "не настроен"));
    setValue(IDS.settingsChannelBot, channel.bot_running
      ? (dict["settings.channel.bot_running"] || "бот работает")
      : (dict["settings.channel.bot_stopped"] || "бот остановлен"));
    setValue(IDS.settingsChannelWarning, channel.warning || dict["settings.channel.warning"] || "Публичный туннель не имеет аутентификации");
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
    if (!providers || providers.length === 0) {
      box.innerHTML = '<p class="settings-empty">—</p>';
      return;
    }
    var dict = window.I18N || {};
    var unavailable = dict["settings.providers.unavailable"] || "недоступно";
    box.innerHTML = providers.map(function (p) {
      var model = p.model ? escapeHTML(p.model) : unavailable;
      var quota = p.quota ? escapeHTML(p.quota) : unavailable;
      var cost = formatProviderCost(p.cost_millis);
      return '<div class="settings-provider">' + escapeHTML(p.name) + " · " +
        (p.key_set ? "ключ задан" : "без ключа") + " · " +
        (dict["settings.providers.model"] || "Модель") + ": " + model + " · " +
        (dict["settings.providers.quota"] || "Остаток лимита вызовов") + ": " + quota + " · " +
        (dict["settings.providers.cost"] || "Стоимость по тарифу") + ": " + cost + "</div>";
    }).join("");
  }

  function formatProviderCost(millis) {
    if (typeof millis !== "number" || !isFinite(millis)) {
      return (window.I18N && window.I18N["settings.providers.unavailable"]) || "недоступно";
    }
    return String(millis);
  }

  function renderSettingsBindings(bindings, hasWorld) {
    var box = byId(IDS.settingsRoles);
    if (!box) {
      return;
    }
    if (!hasWorld) {
      renderNoWorldNote(box);
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

  function renderSettingsModels(models, hasWorld) {
    var box = byId(IDS.settingsModels);
    if (!box) {
      return;
    }
    if (!hasWorld) {
      renderNoWorldNote(box);
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

  function renderNoWorldNote(box) {
    var dict = window.I18N || {};
    box.innerHTML = '<p class="settings-note">' + (dict["settings.no_world"] || "выберите мир") + "</p>";
  }

  function renderSettingsPlugins() {
    var box = byId(IDS.settingsPlugins);
    if (!box) {
      return;
    }
    var dict = window.I18N || {};
    box.innerHTML = '<p class="settings-empty">' + (dict["settings.plugins.empty"] || "Плагинов нет.") + "</p>";
  }

  function renderSettingsFlags(flags) {
    var box = byId(IDS.settingsFlags);
    if (!box) {
      return;
    }
    var dict = window.I18N || {};
    if (!flags || flags.length === 0) {
      box.innerHTML = '<p class="settings-empty">' + (dict["settings.advanced.flags_empty"] || "Фича-флаги не заданы.") + "</p>";
      return;
    }
    box.innerHTML = flags.map(function (flag) {
      return '<div class="settings-flag">' + escapeHTML(flag) + "</div>";
    }).join("");
  }

  function modelSourceURL(path) {
    return "https" + "://" + path;
  }

  function renderModelCatalog() {
    var grid = byId(IDS.modelCatalogGrid);
    if (!grid) {
      return;
    }
    grid.innerHTML = MODEL_CATALOG.map(function (m) {
      return '<button type="button" data-catalog-role="' + escapeHTML(m.role) +
        '" data-catalog-source="' + escapeHTML(m.source) +
        '" data-catalog-ref="' + escapeHTML(m.ref) + '">' +
        escapeHTML(m.label) + "</button>";
    }).join("");
  }

  function onModelCatalogClick(event) {
    var btn = event.target.closest("button[data-catalog-role]");
    if (!btn) {
      return;
    }
    setValue(IDS.settingsModelRole, btn.getAttribute("data-catalog-role") || "");
    setValue(IDS.settingsModelRef, btn.getAttribute("data-catalog-ref") || "");
    setValue(IDS.settingsModelSource, modelSourceURL(btn.getAttribute("data-catalog-source") || ""));
    var hint = byId(IDS.modelCatalogHint);
    if (hint) {
      var dict = window.I18N || {};
      hint.textContent = (dict["settings.models.catalog.selected"] || "Выбрано") + ": " +
        (btn.getAttribute("data-catalog-role") || "") + " · " + (btn.getAttribute("data-catalog-ref") || "");
    }
  }

  function bindModelFileBrowser() {
    var pick = byId(IDS.modelLocalPick);
    var file = byId(IDS.modelLocalFile);
    if (pick && file) {
      pick.addEventListener("click", function () {
        file.click();
      });
    }
    if (file) {
      file.addEventListener("change", function () {
        var name = file.files && file.files.length ? file.files[0].name : "";
        setValue(IDS.modelLocalPath, name);
        var selected = byId(IDS.modelLocalSelected);
        if (selected) {
          var dict = window.I18N || {};
          selected.textContent = name || (dict["settings.models.local_empty"] || "Файл не выбран");
        }
      });
    }
    var apply = byId(IDS.modelLocalApply);
    if (apply) {
      apply.addEventListener("click", function () {
        var path = fieldValue(IDS.modelLocalPath);
        if (path) {
          setValue(IDS.settingsModelRef, path);
        }
      });
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
    var vectorSearch = byId(IDS.settingsVectorSearch) ? byId(IDS.settingsVectorSearch).checked : false;
    putJSON("/api/settings/config", {
      host: fieldValue(IDS.settingsServerHost),
      port: parseInt(fieldValue(IDS.settingsServerPort), 10) || 0
    }).then(function () {
      return putJSON("/api/settings/keys", { keys: { external_vector_search: vectorSearch ? "true" : "false" } });
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

  function submitChannel() {
    var body = {
      edge: fieldValue(IDS.settingsChannelEdge),
      local: fieldValue(IDS.settingsChannelLocal),
      telegram_chat_id: fieldValue(IDS.settingsChannelChatId)
    };
    var token = fieldValue(IDS.settingsChannelToken);
    if (token) {
      body.telegram_token = token;
    }
    putJSON("/api/settings/channel", body).then(loadSettingsData).catch(settingsError);
  }

  function submitSettingsAdvanced() {
    putJSON("/api/world/tunables", {
      world: activeSlugValue(),
      tunables: {
        globalLuck: parseFloat(fieldValue(IDS.settingsGlobalLuck)) || 0,
        maxMemoryEntries: parseInt(fieldValue(IDS.settingsMemoryMax), 10) || 0,
        similarityThreshold: parseFloat(fieldValue(IDS.settingsMemorySimilarity)) || 0,
        halfLifeDays: parseFloat(fieldValue(IDS.settingsMemoryHalflife)) || 0,
        dayNormKm: parseFloat(fieldValue(IDS.settingsDayNorm)) || 25
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

  applyThemeClass(resolveTheme());
  translate(document);
  showChat();
  subscribeMcpWS();
  subscribeMonitorWS();
})();
