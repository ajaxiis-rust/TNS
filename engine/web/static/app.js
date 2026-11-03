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
    atlasBirthError: "atlas-birth-error",
    atlasBirthFanfic: "atlas-birth-fanfic",
    atlasBirthSourceBook: "atlas-birth-source-book",
    fanficModal: "fanfic-modal",
    fanficCancel: "fanfic-cancel",
    fanficConfirm: "fanfic-confirm",
    fanficShelf: "fanfic-shelf",
    fanficImportForm: "fanfic-import-form",
    fanficImportFile: "fanfic-import-file",
    fanficImportSubmit: "fanfic-import-submit",
    fanficProgress: "fanfic-progress",
    atlasSynopsisCount: "atlas-synopsis-count",
    atlasGenres: "atlas-birth-genres",
    atlasGenresFrequent: "atlas-birth-genres-frequent",
    atlasGenresGroups: "atlas-birth-genres-groups",
    atlasGenresMore: "atlas-genres-more",
    atlasEconomy: "atlas-birth-economy-mods",
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
    settingsModelsCurrent: "settings-models-current",
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
    settingsWorldSelect: "settings-world-select",
    settingsWorldOpen: "settings-world-open",
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
    modelCatalogNote: "model-catalog-note",
    settingsInstalledModels: "settings-installed-models",
    modelFileBrowser: "model-file-browser",
    modelLocalPath: "model-local-path",
    modelLocalApply: "model-local-apply",
    modelLocalFile: "model-local-file",
    modelLocalPick: "model-local-pick",
    modelLocalSelected: "model-local-selected",
    wizardRoot: "wizard-root",
    wizardStep0Next: "wizard-step0-next",
    wizardStep1Next: "wizard-step1-next",
    wizardStep2Skip: "wizard-step2-skip",
    wizardStep2Next: "wizard-step2-next",
    wizardStep3Next: "wizard-step3-next",
    wizardStep4Done: "wizard-step4-done",
    wizardStation: "wizard-station",
    wizardModelCards: "wizard-model-cards",
    wizardLibraryForm: "wizard-library-form",
    wizardLibraryFile: "wizard-library-file",
    wizardLibraryList: "wizard-library-list",
    wizardWorldForm: "wizard-world-form",
    wizardWorldTitle: "wizard-world-title",
    wizardWorldSynopsis: "wizard-world-synopsis",
    wizardWorldSocial: "wizard-world-social",
    wizardWorldRating: "wizard-world-rating",
    wizardWorldGenres: "wizard-world-genres"
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
    {
      role: "weaver",
      ref: "Llama-3.2-3B-Instruct-Q4_K_M",
      source: "https://huggingface.co/bartowski/Llama-3.2-3B-Instruct-GGUF/resolve/main/Llama-3.2-3B-Instruct-Q4_K_M.gguf",
      label: "Llama 3.2 3B",
      size: "2,0 ГБ",
      note: "Быстрая модель для живой прозы: меньше память, выше скорость."
    },
    {
      role: "pythia",
      ref: "Qwen2.5-7B-Instruct-Q4_K_M",
      source: "https://huggingface.co/bartowski/Qwen2.5-7B-Instruct-GGUF/resolve/main/Qwen2.5-7B-Instruct-Q4_K_M.gguf",
      label: "Qwen 2.5 7B",
      size: "4,7 ГБ",
      note: "Сильнее держит длинный контекст и русский язык."
    },
    {
      role: "editor",
      ref: "Meta-Llama-3.1-8B-Instruct-Q4_K_M",
      source: "https://huggingface.co/bartowski/Meta-Llama-3.1-8B-Instruct-GGUF/resolve/main/Meta-Llama-3.1-8B-Instruct-Q4_K_M.gguf",
      label: "Llama 3.1 8B",
      size: "4,9 ГБ",
      note: "Для редактуры глав и длинных сцен."
    },
    {
      role: "analysis",
      ref: "Qwen2.5-3B-Instruct-Q4_K_M",
      source: "https://huggingface.co/bartowski/Qwen2.5-3B-Instruct-GGUF/resolve/main/Qwen2.5-3B-Instruct-Q4_K_M.gguf",
      label: "Qwen 2.5 3B",
      size: "1,9 ГБ",
      note: "Для разбора сцен и служебных задач."
    },
    {
      role: "embed",
      ref: "bge-m3-Q4_K_M",
      source: "https://huggingface.co/gpustack/bge-m3-GGUF/resolve/main/bge-m3-Q4_K_M.gguf",
      label: "BGE-M3 Q4_K_M",
      size: "438 МБ",
      note: "Эмбеддинг-модель для векторов смысла (dim 1024).",
      ram: "RAM < 8 ГБ"
    },
    {
      role: "embed",
      ref: "bge-m3-Q8_0",
      source: "https://huggingface.co/gpustack/bge-m3-GGUF/resolve/main/bge-m3-Q8_0.gguf",
      label: "BGE-M3 Q8_0",
      size: "634 МБ",
      note: "Эмбеддинг-модель для векторов смысла (dim 1024).",
      ram: "RAM 8–16 ГБ"
    },
    {
      role: "embed",
      ref: "bge-m3-F16",
      source: "https://huggingface.co/gpustack/bge-m3-GGUF/resolve/main/bge-m3-F16.gguf",
      label: "BGE-M3 F16",
      size: "1,16 ГБ",
      note: "Эмбеддинг-модель для векторов смысла (dim 1024).",
      ram: "RAM ≥ 16 ГБ"
    }
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
          var error = new Error(msg);
          error.code = body && body.error ? body.error.code : "";
          error.fields = body && body.error && body.error.fields ? body.error.fields : [];
          throw error;
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
      loadAtlasBirthOptions();
      loadAtlasWorlds();
    }).catch(function (err) {
      viewAtlas.textContent = err.message;
    });
  }

  // Канонические перечни анкеты приходят из движка: форма не может предложить
  // жанр или строй, который birth отвергнет.
  function loadAtlasBirthOptions() {
    apiJSON("/api/atlas/birth").then(renderAtlasBirthOptions).catch(function (err) {
      showAtlasBirthError((err && err.message) || String(err));
    });
  }

  var LANG_NAMES = {
    ru: "Русский", en: "English", de: "Deutsch", fr: "Français",
    es: "Español", ja: "日本語", zh: "中文"
  };

  function renderAtlasBirthOptions(data) {
    var genres = data.genres || [];
    var economy = data.economy_mods || [];
    renderGenreChoices(genres);
    fillBirthChoices(IDS.atlasEconomy, economy, []);
    fillBirthSelect(IDS.atlasBirthSocialOrder, data.social_orders || [], "феодализм");
    fillBirthSelect(IDS.atlasBirthTravelMode, data.travel_modes || [], "вымышленная");
    fillBirthSelect(IDS.atlasBirthLang, data.langs || [], "ru", LANG_NAMES);
    fillBirthSelect(IDS.atlasBirthCensorship, data.ratings || [], "16+");
  }

  // Жанры приходят плоским списком из движка; в форме они собраны в группы
  // (раздел 3 плана P11), а частые жанры вынесены быстрыми чипами над «ещё».
  var GENRE_GROUPS = [
    { key: "atlas.genre_group.fiction", items: ["фэнтези", "тёмное фэнтези", "городское фэнтези", "научная фантастика", "космоопера", "киберпанк", "стимпанк", "постапокалипсис", "LitRPG"] },
    { key: "atlas.genre_group.myth", items: ["мифология", "исторический", "альтернативная история"] },
    { key: "atlas.genre_group.tension", items: ["хоррор", "мистика", "триллер", "детектив"] },
    { key: "atlas.genre_group.adventure", items: ["приключения", "морское приключение", "вестерн"] },
    { key: "atlas.genre_group.people", items: ["любовный", "семейная сага", "подростковый", "драма"] },
    { key: "atlas.genre_group.ideas", items: ["философский", "социальная фантастика", "антиутопия", "сатира", "юмор"] },
    { key: "atlas.genre_group.form", items: ["фанфик", "современная проза"] }
  ];
  var FREQUENT_GENRES = ["фэнтези", "научная фантастика", "детектив", "приключения", "любовный", "хоррор", "исторический", "современная проза"];

  function renderGenreChoices(genres) {
    var dict = window.I18N || {};
    var frequent = byId(IDS.atlasGenresFrequent);
    if (frequent) {
      frequent.innerHTML = FREQUENT_GENRES.map(function (genre) {
        return '<button type="button" class="choice choice--quick" data-genre="' + escapeHTML(genre) + '">' +
          escapeHTML(displayChoice(genre)) + "</button>";
      }).join("");
    }
    var grouped = byId(IDS.atlasGenres);
    if (grouped) {
      grouped.innerHTML = GENRE_GROUPS.map(function (group) {
        var title = dict[group.key] || group.key;
        var chips = group.items.map(function (genre) {
          var checked = genre === "фэнтези" ? " checked" : "";
          return '<label class="choice"><input type="checkbox" value="' + escapeHTML(genre) + '"' + checked + '>' +
            "<span>" + escapeHTML(displayChoice(genre)) + "</span></label>";
        }).join("");
        return '<div class="genre-group"><h3 class="genre-group__title">' + escapeHTML(title) + "</h3>" +
          '<div class="choice-grid">' + chips + "</div></div>";
      }).join("");
    }
    syncFrequentGenres();
  }

  function findGenreInput(genre) {
    var grouped = byId(IDS.atlasGenres);
    if (!grouped) {
      return null;
    }
    var inputs = grouped.querySelectorAll('input[type="checkbox"]');
    for (var i = 0; i < inputs.length; i++) {
      if (inputs[i].value === genre) {
        return inputs[i];
      }
    }
    return null;
  }

  function syncFrequentGenres() {
    var frequent = byId(IDS.atlasGenresFrequent);
    if (!frequent) {
      return;
    }
    frequent.querySelectorAll("[data-genre]").forEach(function (btn) {
      var genre = btn.getAttribute("data-genre");
      var input = findGenreInput(genre);
      btn.classList.toggle("active", !!(input && input.checked));
      btn.setAttribute("aria-pressed", input && input.checked ? "true" : "false");
    });
  }

  function onFrequentGenreClick(event) {
    var btn = event.target.closest("[data-genre]");
    if (!btn) {
      return;
    }
    var input = findGenreInput(btn.getAttribute("data-genre"));
    if (!input) {
      return;
    }
    input.checked = !input.checked;
    syncFrequentGenres();
  }

  function toggleGenreGroups() {
    var more = byId(IDS.atlasGenresMore);
    var groups = byId(IDS.atlasGenresGroups);
    if (!more) {
      return;
    }
    var expanded = more.getAttribute("aria-expanded") === "true";
    more.setAttribute("aria-expanded", String(!expanded));
    if (groups) {
      groups.hidden = expanded;
    }
    var dict = window.I18N || {};
    var label = more.querySelector("[data-i18n]");
    if (label) {
      label.textContent = expanded ? (dict["atlas.genres_more"] || "Ещё жанры") : (dict["atlas.genres_less"] || "Свернуть");
    }
  }

  function fillBirthSelect(id, values, selected, labels) {
    var select = byId(id);
    if (!select) {
      return;
    }
    select.innerHTML = values.map(function (value) {
      var label = labels && labels[value] ? labels[value] : displayChoice(value);
      return '<option value="' + escapeHTML(value) + '"' +
        (value === selected ? " selected" : "") + ">" + escapeHTML(label) + "</option>";
    }).join("");
  }

  function fillBirthChoices(id, values, selected) {
    var box = byId(id);
    if (!box) {
      return;
    }
    box.innerHTML = values.map(function (value) {
      var checked = selected.indexOf(value) >= 0 ? " checked" : "";
      return '<label class="choice"><input type="checkbox" value="' + escapeHTML(value) + '"' + checked + '>' +
        "<span>" + escapeHTML(displayChoice(value)) + "</span></label>";
    }).join("");
  }

  function displayChoice(value) {
    if (!value) {
      return "";
    }
    return value.charAt(0).toUpperCase() + value.slice(1);
  }

  function checkedBirthChoices(id) {
    var box = byId(id);
    if (!box) {
      return [];
    }
    return Array.prototype.map.call(box.querySelectorAll('input[type="checkbox"]:checked'), function (input) {
      return input.value;
    });
  }

  // Фанфик (T-12-15): галочка открывает модалку с полкой, импортом и прогрессом;
  // отмена снимает галочку и очищает выбранную книгу-источник.
  function isFanficChecked() {
    var box = byId(IDS.atlasBirthFanfic);
    return !!(box && box.checked);
  }

  function setSourceBook(id) {
    var input = byId(IDS.atlasBirthSourceBook);
    if (input) {
      input.value = id || "";
    }
  }

  function setFanficState(checked, openModal) {
    var box = byId(IDS.atlasBirthFanfic);
    if (box) {
      box.checked = checked;
    }
    var genre = findGenreInput("фанфик");
    if (genre) {
      genre.checked = checked;
    }
    if (!checked) {
      setSourceBook("");
      closeFanficModal();
    } else if (openModal) {
      openFanficModal();
    }
    syncFrequentGenres();
  }

  function openFanficModal() {
    var modal = byId(IDS.fanficModal);
    if (modal) {
      modal.hidden = false;
    }
    loadFanficShelf();
  }

  function closeFanficModal() {
    var modal = byId(IDS.fanficModal);
    if (modal) {
      modal.hidden = true;
    }
  }

  function cancelFanficModal() {
    setFanficState(false, false);
  }

  function confirmFanficModal() {
    if (!fieldValue(IDS.atlasBirthSourceBook)) {
      toast((window.I18N && window.I18N["atlas.fanfic.pick"]) || "Выберите книгу или импортируйте её.", false);
      return;
    }
    closeFanficModal();
  }

  function fanficStatusLabel(status, progress) {
    var dict = window.I18N || {};
    var base = status || "imported";
    var label = dict["atlas.fanfic.status." + base] || base;
    if (base === "forging") {
      label = label + " " + Math.round((progress || 0) * 100) + "%";
    }
    return label;
  }

  function loadFanficShelf() {
    apiJSON("/api/library").then(renderFanficShelf).catch(function (err) {
      var shelf = byId(IDS.fanficShelf);
      if (shelf) {
        shelf.textContent = (err && err.message) || String(err);
      }
    });
  }

  function renderFanficShelf(data) {
    var shelf = byId(IDS.fanficShelf);
    if (!shelf) {
      return;
    }
    var dict = window.I18N || {};
    var books = (data && data.books) || [];
    var selected = fieldValue(IDS.atlasBirthSourceBook);
    if (books.length === 0) {
      shelf.innerHTML = '<p class="fanfic-shelf__empty" data-i18n="atlas.fanfic.empty">' +
        (dict["atlas.fanfic.empty"] || "Полка пуста — импортируйте книгу.") + "</p>";
      return;
    }
    shelf.innerHTML = books.map(function (b) {
      var isSelected = b.id === selected;
      return '<button type="button" class="fanfic-book' + (isSelected ? " is-selected" : "") + '" data-book-id="' + escapeHTML(b.id) + '">' +
        '<span class="fanfic-book__name">' + escapeHTML(b.name || b.id) + "</span>" +
        '<span class="fanfic-book__meta">' + escapeHTML(b.format || "") + " · " + escapeHTML(fanficStatusLabel(b.status, b.progress)) + "</span>" +
        "</button>";
    }).join("");
  }

  function onFanficShelfClick(event) {
    var btn = event.target.closest("[data-book-id]");
    if (!btn) {
      return;
    }
    var id = btn.getAttribute("data-book-id");
    setSourceBook(id);
    var shelf = byId(IDS.fanficShelf);
    if (shelf) {
      shelf.querySelectorAll(".fanfic-book").forEach(function (node) {
        node.classList.toggle("is-selected", node.getAttribute("data-book-id") === id);
      });
    }
  }

  function submitFanficImport() {
    var fileEl = byId(IDS.fanficImportFile);
    var file = fileEl && fileEl.files ? fileEl.files[0] : null;
    if (!file) {
      return;
    }
    var progress = byId(IDS.fanficProgress);
    var btn = byId(IDS.fanficImportSubmit);
    var release = setBusy(btn);
    if (progress) {
      progress.hidden = false;
      progress.textContent = (window.I18N && window.I18N["atlas.fanfic.importing"]) || "Импорт…";
    }
    var xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/library/import");
    xhr.setRequestHeader("Accept", "application/json");
    xhr.upload.onprogress = function (e) {
      if (e.lengthComputable && progress) {
        progress.textContent = ((window.I18N && window.I18N["atlas.fanfic.importing"]) || "Импорт…") +
          " " + Math.round(e.loaded / e.total * 100) + "%";
      }
    };
    xhr.onload = function () {
      release();
      var body = {};
      try {
        body = JSON.parse(xhr.responseText);
      } catch (err) {
        body = {};
      }
      if (xhr.status >= 200 && xhr.status < 300) {
        var books = body.books || [];
        if (books.length) {
          setSourceBook(books[books.length - 1].id);
        }
        renderFanficShelf(body);
        if (progress) {
          progress.hidden = true;
        }
        if (fileEl) {
          fileEl.value = "";
        }
      } else {
        var msg = (body && body.error && body.error.message) || xhr.statusText;
        if (progress) {
          progress.textContent = msg;
        }
      }
    };
    xhr.onerror = function () {
      release();
      if (progress) {
        progress.textContent = (window.I18N && window.I18N["atlas.fanfic.import_error"]) || "Ошибка импорта";
      }
    };
    var form = new FormData();
    form.append("file", file);
    xhr.send(form);
  }

  function bindAtlas() {
    var form = byId(IDS.atlasBirthForm);
    if (form) {
      form.addEventListener("submit", function (event) {
        event.preventDefault();
        submitAtlasBirth();
      });
      form.addEventListener("reset", function () {
        clearAtlasBirthErrors();
        var slug = byId(IDS.atlasBirthSlug);
        if (slug) {
          slug.dataset.manual = "";
        }
        updateSynopsisCount();
      });
    }
    bindSlugAutofill();
    var genresFrequent = byId(IDS.atlasGenresFrequent);
    if (genresFrequent) {
      genresFrequent.addEventListener("click", onFrequentGenreClick);
    }
    var genresGrouped = byId(IDS.atlasGenres);
    if (genresGrouped) {
      genresGrouped.addEventListener("change", function (event) {
        syncFrequentGenres();
        var target = event.target;
        if (target && target.type === "checkbox" && target.value === "фанфик") {
          setFanficState(target.checked, true);
        }
      });
    }
    var fanfic = byId(IDS.atlasBirthFanfic);
    if (fanfic) {
      fanfic.addEventListener("change", function (event) {
        setFanficState(event.target.checked, true);
      });
    }
    var fanficCancel = byId(IDS.fanficCancel);
    if (fanficCancel) {
      fanficCancel.addEventListener("click", cancelFanficModal);
    }
    var fanficConfirm = byId(IDS.fanficConfirm);
    if (fanficConfirm) {
      fanficConfirm.addEventListener("click", confirmFanficModal);
    }
    var fanficShelf = byId(IDS.fanficShelf);
    if (fanficShelf) {
      fanficShelf.addEventListener("click", onFanficShelfClick);
    }
    var fanficImportForm = byId(IDS.fanficImportForm);
    if (fanficImportForm) {
      fanficImportForm.addEventListener("submit", function (event) {
        event.preventDefault();
        submitFanficImport();
      });
    }
    var genresMore = byId(IDS.atlasGenresMore);
    if (genresMore) {
      genresMore.addEventListener("click", toggleGenreGroups);
    }
    var synopsis = byId(IDS.atlasBirthSynopsis);
    if (synopsis) {
      synopsis.addEventListener("input", updateSynopsisCount);
    }
    var start = byId(IDS.atlasWizardStart);
    if (start) {
      start.addEventListener("click", startAtlasWizard);
    }
    var worlds = byId(IDS.atlasWorlds);
    if (worlds) {
      worlds.addEventListener("click", onAtlasWorldsClick);
    }
    updateSynopsisCount();
  }

  // Слаг заполняется из названия, пока игрок не поправил его руками.
  function bindSlugAutofill() {
    var title = byId(IDS.atlasBirthTitle);
    var slug = byId(IDS.atlasBirthSlug);
    if (!title || !slug) {
      return;
    }
    title.addEventListener("input", function () {
      if (!slug.dataset.manual) {
        slug.value = transliterateSlug(title.value);
      }
    });
    slug.addEventListener("input", function () {
      slug.dataset.manual = slug.value ? "1" : "";
    });
  }

  var TRANSLIT = {
    а: "a", б: "b", в: "v", г: "g", д: "d", е: "e", ё: "e", ж: "zh", з: "z",
    и: "i", й: "y", к: "k", л: "l", м: "m", н: "n", о: "o", п: "p", р: "r",
    с: "s", т: "t", у: "u", ф: "f", х: "h", ц: "ts", ч: "ch", ш: "sh", щ: "sch",
    ъ: "", ы: "y", ь: "", э: "e", ю: "yu", я: "ya"
  };

  function transliterateSlug(text) {
    return String(text || "").toLowerCase().split("").map(function (ch) {
      if (TRANSLIT[ch] !== undefined) {
        return TRANSLIT[ch];
      }
      if (/[a-z0-9]/.test(ch)) {
        return ch;
      }
      return " ";
    }).join("").trim().replace(/\s+/g, "-").replace(/-+/g, "-").replace(/^-|-$/g, "");
  }

  function countSentences(text) {
    var trimmed = String(text || "").trim();
    if (!trimmed) {
      return 0;
    }
    var matches = trimmed.match(/[.!?…]+(?=\s|$)/g);
    return matches ? matches.length : 1;
  }

  function updateSynopsisCount() {
    var box = byId(IDS.atlasSynopsisCount);
    if (box) {
      box.textContent = String(countSentences(fieldValue(IDS.atlasBirthSynopsis)));
    }
  }

  function showAtlasBirthError(text) {
    var box = byId(IDS.atlasBirthError);
    if (box) {
      box.textContent = text || "";
      box.hidden = !text;
    }
  }

  function setAtlasFieldError(field, reason) {
    var box = document.querySelector('#atlas-birth-form [data-field-error="' + field + '"]');
    if (box) {
      box.textContent = reason || "";
      box.hidden = !reason;
    }
    var input = document.getElementById("atlas-birth-" + field.replace(/_/g, "-"));
    if (input) {
      input.classList.toggle("is-invalid", !!reason);
    }
  }

  function clearAtlasBirthErrors() {
    showAtlasBirthError("");
    var form = byId(IDS.atlasBirthForm);
    if (!form) {
      return;
    }
    form.querySelectorAll("[data-field-error]").forEach(function (node) {
      node.textContent = "";
      node.hidden = true;
    });
    form.querySelectorAll(".is-invalid").forEach(function (node) {
      node.classList.remove("is-invalid");
    });
  }

  function validateAtlasBirth(body) {
    clearAtlasBirthErrors();
    var ok = true;
    function fail(field, reason) {
      setAtlasFieldError(field, reason);
      ok = false;
    }
    if (!body.slug) {
      fail("slug", "Укажите адрес мира — он станет именем папки.");
    } else if (!/^[a-z0-9-]+$/.test(body.slug)) {
      fail("slug", "Только латиница, цифры и дефис.");
    }
    var sentences = countSentences(body.synopsis);
    if (sentences === 0) {
      fail("synopsis", "Расскажите, о чём история: 1–3 предложения.");
    } else if (sentences > 3) {
      fail("synopsis", "Слишком длинно: нужно 1–3 предложения.");
    }
    if (body.genres.length === 0) {
      fail("genres", "Выберите хотя бы один жанр.");
    }
    if (isFanficChecked() && !body.source_book) {
      fail("source_book", "Выберите книгу-источник фанфика.");
      openFanficModal();
    }
    if (!body.social_order) {
      fail("social_order", "Выберите строй мира.");
    }
    for (var i = 0; i < body.world_rules.length; i++) {
      if (body.world_rules[i].indexOf(":") <= 0) {
        fail("world_rules", "Строка " + (i + 1) + ": нужен формат «название: описание».");
        break;
      }
    }
    return ok;
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
    window.__activeWorldChecked = true;
    window.__languageReady = window.__languageReady || {};
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
      window.__languageReady[w.slug] = !!w.language_ready;
      var active = w.active ? ' <span class="atlas-card__active">' + (dict["atlas.active"] || "Активен") + "</span>" : "";
      var html = '<div class="atlas-card' + (w.active ? " atlas-card--active" : "") + '">';
      html += '<div class="atlas-card__name">' + escapeHTML(w.title || w.slug) + active + "</div>";
      html += '<div class="atlas-card__meta">' + w.entities + " " + (dict["atlas.entities"] || "сущностей") + " · " + w.sessions + " " + (dict["atlas.sessions"] || "сессий") + "</div>";
      if (!w.language_ready) {
        html += '<div class="atlas-card__status">' + (dict["atlas.language.preparing"] || "Язык книги ещё готовится…") + "</div>";
      }
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
    }).catch(atlasActionError);
  }

  function playAtlasWorld(slug) {
    if (!window.__languageReady || !window.__languageReady[slug]) {
      // Язык книги ещё не готов (T-12-14): ковка фанфика до первого хода.
      toast((window.I18N && window.I18N["atlas.language.preparing"]) || "Язык книги ещё готовится…", false);
      forgeAtlasWorld(slug);
      return;
    }
    postJSON("/api/atlas/worlds/active", { world: slug }).then(function () {
      window.__activeSlug = slug;
      showChat();
    }).catch(atlasActionError);
  }

  // Синхронная ковка книги после рождения (T-12-14): метка language_ready и
  // прогресс выставляются на сервере; отказ (нет модели) приходит предупреждением.
  function forgeAtlasWorld(slug) {
    postJSON("/api/atlas/worlds/forge", { world: slug }).then(function (res) {
      if (res && res.warning) {
        toast(res.warning, false);
      }
    }).catch(atlasActionError).then(function () {
      loadAtlasWorlds();
    });
  }

  function deleteAtlasWorld(slug) {
    if (!window.confirm("Удалить мир " + slug + "?")) {
      return;
    }
    var path = "/api/atlas/worlds/delete?world=" + encodeURIComponent(slug) + "&confirm=" + encodeURIComponent(slug);
    apiJSON(path, { method: "DELETE" }).then(loadAtlasWorlds).catch(atlasActionError);
  }

  function atlasActionError(err) {
    toast((err && err.message) || String(err), false);
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
    }).catch(atlasActionError);
  }

  function submitAtlasBirth() {
    var body = {
      slug: fieldValue(IDS.atlasBirthSlug),
      title: fieldValue(IDS.atlasBirthTitle) || fieldValue(IDS.atlasBirthSlug),
      description: fieldValue(IDS.atlasBirthDescription),
      synopsis: fieldValue(IDS.atlasBirthSynopsis),
      prologue: fieldValue(IDS.atlasBirthPrologue),
      lang: fieldValue(IDS.atlasBirthLang) || "ru",
      travel_mode: fieldValue(IDS.atlasBirthTravelMode) || "вымышленная",
      genres: checkedBirthChoices(IDS.atlasGenres),
      world_rules: splitLines(fieldValue(IDS.atlasBirthWorldRules)),
      social_order: fieldValue(IDS.atlasBirthSocialOrder),
      economy_mods: checkedBirthChoices(IDS.atlasEconomy),
      magic: fieldValue(IDS.atlasBirthMagic),
      hero_birth: fieldValue(IDS.atlasBirthHeroBirth),
      censorship: fieldValue(IDS.atlasBirthCensorship) || "16+",
      source_book: fieldValue(IDS.atlasBirthSourceBook)
    };
    if (!validateAtlasBirth(body)) {
      return;
    }
    var btn = byId(IDS.atlasBirthSubmit);
    var release = setBusy(btn);
    var slug = body.slug;
    postJSON("/api/atlas/worlds/birth", body).then(function () {
      var form = byId(IDS.atlasBirthForm);
      if (form) {
        form.reset();
      }
      var slugInput = byId(IDS.atlasBirthSlug);
      if (slugInput) {
        slugInput.dataset.manual = "";
      }
      loadAtlasBirthOptions();
      showAtlasBirthError("");
      // Синхронная ковка книги после рождения (T-12-14): до первого хода.
      return forgeAtlasWorld(slug);
    }).catch(function (err) {
      atlasError(err);
    }).then(release);
  }

  function atlasError(err) {
    clearAtlasBirthErrors();
    var fields = (err && err.fields) || [];
    if (fields.length > 0) {
      fields.forEach(function (f) {
        setAtlasFieldError(f.field, f.reason);
      });
      showAtlasBirthError("Проверьте отмеченные поля.");
      return;
    }
    showAtlasBirthError((err && err.message) || String(err));
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
      loadWorldChoices().then(function () {
        loadSettingsData();
        loadMonitorData();
      });
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
      loadWorldChoices().then(function () {
        loadSettingsData();
        loadMonitorData();
      });
    }).catch(function (err) {
      viewSettings.textContent = err.message;
    });
  }

  // Выбор мира прямо в Настройках: активный мир живёт в памяти webapi и после
  // перезапуска ядра пуст, поэтому без этого списка разделы моделей и ролей
  // выглядят пустыми, хотя миры есть.
  function loadWorldChoices() {
    return apiJSON("/api/atlas/worlds").then(function (data) {
      window.__activeWorldChecked = true;
      window.__activeSlug = data.active || "";
      var select = byId(IDS.settingsWorldSelect);
      if (!select) {
        return;
      }
      var worlds = data.worlds || [];
      if (worlds.length === 0) {
        select.innerHTML = '<option value="">— нет миров —</option>';
        return;
      }
      select.innerHTML = '<option value="">— выберите мир —</option>' + worlds.map(function (w) {
        return '<option value="' + escapeHTML(w.slug) + '">' + escapeHTML(w.title || w.slug) + "</option>";
      }).join("");
      if (!window.__activeSlug && worlds.length === 1) {
        window.__activeSlug = worlds[0].slug;
        postJSON("/api/atlas/worlds/active", { world: worlds[0].slug }).catch(function () {
          // Настройки уже открываются на этом мире; активный мир для чата
          // выставится при следующем клике по карточке, если запрос не прошёл.
        });
      }
      select.value = window.__activeSlug || "";
    }).catch(function () {
      var select = byId(IDS.settingsWorldSelect);
      if (select) {
        select.innerHTML = '<option value="">— нет миров —</option>';
      }
    });
  }

  // Подсказки к полям: всплывают при наведении на подпись (и при фокусе).
  var FIELD_HINTS = {
    "settings-lang": "Язык пакетов интерфейса. Сейчас доступен только русский.",
    "settings-theme": "Тема применяется сразу и запоминается в этом браузере.",
    "settings-server-host": "Адрес, на котором слушает движок. Только 127.0.0.1 или localhost.",
    "settings-server-port": "Порт веб-интерфейса. После изменения нужен перезапуск движка.",
    "settings-gen-addr": "Локальный генератор (llama.cpp): хост:порт, например 127.0.0.1:5001.",
    "settings-gen-key": "API-ключ генератора, если он внешний. Хранится в settings.toml, в config не попадает.",
    "settings-gen-model": "Модель генерации: реплики игрока, проза мира.",
    "settings-gen-timeout": "Сколько ждать ответа генератора (например 30s).",
    "settings-gen-max-tokens": "Предел длины ответа в токенах. 0 — без ограничения.",
    "settings-gen-temperature": "0 — строго по замыслу мира, выше — свободнее и неожиданнее.",
    "settings-gen-retries": "Сколько раз повторить запрос при сбое генератора.",
    "settings-gen-concurrency": "Сколько запросов к генератору можно держать параллельно.",
    "settings-emb-addr": "Локальный эмбеддинг-сервер (BGE-M3): хост:порт, по умолчанию 127.0.0.1:5002.",
    "settings-emb-key": "API-ключ эмбеддинг-сервера, если он внешний.",
    "settings-emb-model": "Модель эмбеддингов для памяти и поиска.",
    "settings-provider-name": "Короткое имя провайдера, под ним хранится ключ.",
    "settings-provider-key": "Ключ провайдера. Сохраняется в settings.toml с правами 0600.",
    "settings-provider-base": "Базовый URL API провайдера, если он не из каталога.",
    "settings-model-role": "Роль из раздела «Роли», для которой ставится модель.",
    "settings-model-source": "Откуда качать: huggingface.co/организация/модель.",
    "settings-model-ref": "Тег модели внутри источника, например Q4_K_M.",
    "settings-role-name": "Код-имя роли (например, teller). Менять существующие не стоит.",
    "settings-role-provider": "Провайдер из раздела «Провайдеры».",
    "settings-role-model": "Ссылка на модель для этой роли.",
    "settings-role-temperature": "Температура роли: 0 — детерминированно.",
    "settings-role-prompts": "Дополнительные промпты роли, по одному на строку.",
    "settings-role-priority": "Порядок применения роли: больше — важнее.",
    "settings-paganel-key": "Ключ внешней модели для карты; нужен только для походов наружу.",
    "settings-channel-edge": "Край туннеля Cloudflare: только имя хоста, без схемы и слешей.",
    "settings-channel-local": "Куда смотрит туннель: локальный адрес веб-интерфейса.",
    "settings-channel-token": "Токен бота от @BotFather. Хранится в settings.toml с правами 0600.",
    "settings-channel-chat-id": "Ваш chat_id в Telegram. Бот отвечает только этому чату.",
    "settings-memory-max": "Сколько записей памяти станка хранить.",
    "settings-memory-similarity": "Порог похожести для поиска по памяти: 0..1.",
    "settings-memory-halflife": "За сколько дней запись памяти «остывает» вдвое.",
    "settings-global-luck": "Общая удача мира: множитель на случайные события.",
    "settings-day-norm": "Норма километров в день для пути героя (20..30)."
  };

  function applyFieldHints(root) {
    Object.keys(FIELD_HINTS).forEach(function (id) {
      var field = byId(id);
      if (!field) {
        return;
      }
      var label = field.closest(".settings-field") || field.closest("label");
      if (label && !label.hasAttribute("data-tip")) {
        label.setAttribute("data-tip", FIELD_HINTS[id]);
      }
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
    applyFieldHints(viewSettings);

    var theme = byId(IDS.settingsTheme);
    if (theme) {
      theme.value = normalizeTheme(resolveTheme());
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

    var rolesBox = byId(IDS.settingsRoles);
    if (rolesBox) {
      rolesBox.addEventListener("click", function (event) {
        var card = event.target.closest(".settings-role[data-role]");
        if (!card) {
          return;
        }
        setValue(IDS.settingsRoleName, card.getAttribute("data-role") || "");
        setValue(IDS.settingsRoleProvider, ensureOption(IDS.settingsRoleProvider, card.getAttribute("data-provider") || "local"));
        setValue(IDS.settingsRoleModel, ensureOption(IDS.settingsRoleModel, card.getAttribute("data-model") || ""));
        setValue(IDS.settingsRoleTemperature, card.getAttribute("data-temperature") || "0");
        setValue(IDS.settingsRolePrompts, card.getAttribute("data-prompts") || "");
        setValue(IDS.settingsRolePriority, card.getAttribute("data-priority") || "0");
      });
    }

    var providerCatalog = byId("settings-provider-catalog");
    if (providerCatalog) {
      providerCatalog.addEventListener("change", function () {
        if (providerCatalog.value) {
          setValue(IDS.settingsProviderName, providerCatalog.value);
        }
      });
    }

    var worldSelect = byId(IDS.settingsWorldSelect);
    if (worldSelect) {
      worldSelect.addEventListener("change", function () {
        var slug = worldSelect.value;
        if (!slug) {
          return;
        }
        worldSelect.disabled = true;
        postJSON("/api/atlas/worlds/active", { world: slug }).then(function () {
          window.__activeSlug = slug;
          window.__activeWorldChecked = true;
          return loadSettingsData();
        }).then(loadMonitorData).catch(settingsError).then(function () {
          worldSelect.disabled = false;
        });
      });
    }
    var worldOpen = byId(IDS.settingsWorldOpen);
    if (worldOpen) {
      worldOpen.addEventListener("click", function () {
        atlasTab.click();
      });
    }

    renderModelCatalog([]);
    var grid = byId(IDS.modelCatalogGrid);
    if (grid) {
      grid.addEventListener("click", onModelCatalogClick);
    }
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
        var release = setBusy(check);
        loadSettingsData().then(function () {
          var form = byId(IDS.settingsChannelForm);
          if (form) {
            formResult(form, "Проверено: настройки перечитаны из конфига", true);
          }
        }).catch(function (err) {
          var form = byId(IDS.settingsChannelForm);
          if (form) {
            formResult(form, (err && err.message) || String(err), false);
          }
        }).then(release);
      });
    }
  }

  // Каждая форма настроек обязана отвечать: кнопка блокируется на время
  // запроса, результат — строка под формой (успех или текст ошибки).
  function bindSettingsSubmit(id, handler) {
    var form = byId(id);
    if (!form) {
      return;
    }
    form.addEventListener("submit", function (event) {
      event.preventDefault();
      var btn = form.querySelector('button[type="submit"]');
      var release = setBusy(btn);
      var job;
      try {
        job = Promise.resolve(handler());
      } catch (err) {
        job = Promise.reject(err);
      }
      job.then(function () {
        formResult(form, savedText(), true);
      }).catch(function (err) {
        formResult(form, (err && err.message) || String(err), false);
      }).then(release);
    });
  }

  function setBusy(btn) {
    if (!btn || btn.dataset.busy === "1") {
      return function () {};
    }
    btn.dataset.busy = "1";
    btn.disabled = true;
    btn.classList.add("is-busy");
    return function () {
      btn.dataset.busy = "";
      btn.disabled = false;
      btn.classList.remove("is-busy");
    };
  }

  function savedText() {
    return (window.I18N && window.I18N["settings.saved"]) || "Сохранено";
  }

  function formResult(form, text, ok) {
    if (!form) {
      return;
    }
    var box = form.querySelector(".form-result");
    if (!box) {
      box = document.createElement("p");
      box.className = "form-result";
      form.appendChild(box);
    }
    box.textContent = (ok ? "\u2713 " : "\u2717 ") + text;
    box.classList.toggle("form-result--ok", !!ok);
    box.classList.toggle("form-result--err", !ok);
    clearTimeout(form.__resultTimer);
    form.__resultTimer = setTimeout(function () {
      box.textContent = "";
      box.classList.remove("form-result--ok", "form-result--err");
    }, 6000);
  }

  // Плавающая плашка для ошибок загрузки и действий вне форм.
  function toast(text, ok) {
    var box = byId("tns-toast");
    if (!box) {
      box = document.createElement("div");
      box.id = "tns-toast";
      box.className = "tns-toast";
      document.body.appendChild(box);
    }
    box.textContent = text;
    box.classList.toggle("tns-toast--err", !ok);
    box.classList.add("tns-toast--show");
    clearTimeout(toast.__timer);
    toast.__timer = setTimeout(function () {
      box.classList.remove("tns-toast--show");
    }, 5000);
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
  }

  function loadSettingsData() {
    return ensureActiveWorld().then(function () {
      var slug = activeSlugValue();
      var path = slug ? "/api/settings?world=" + encodeURIComponent(slug) : "/api/settings";
      return apiJSON(path).then(renderSettings).then(loadProvidersData).catch(settingsError);
    });
  }

  // Настройки открываются и без визита в Атлас: активный мир подтягиваем сами,
  // иначе разделы «Модели» и «Роли» выглядят пустыми при живом мире.
  function ensureActiveWorld() {
    if (window.__activeSlug) {
      return Promise.resolve(window.__activeSlug);
    }
    if (window.__activeWorldChecked) {
      return Promise.resolve("");
    }
    return apiJSON("/api/atlas/worlds").then(function (data) {
      window.__activeWorldChecked = true;
      window.__activeSlug = data.active || "";
      return window.__activeSlug;
    }).catch(function () {
      window.__activeWorldChecked = true;
      return "";
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
    renderSettingsModelsCurrent(data.bindings || [], hasWorld);
    var rolesForm = byId(IDS.settingsRolesForm);
    if (rolesForm) {
      rolesForm.hidden = !hasWorld;
    }
    renderSettingsModels(data.models || [], hasWorld);
    renderModelChoices(data.models || []);
    renderRoleProviderChoices(data.presets || [], data.providers || []);
    renderModelCatalog(data.models || [], data.ram_gb);
    renderModelCatalogNote(hasWorld);
    renderSettingsPlugins();
    renderSettingsFlags(data.advanced ? data.advanced.feature_flags || [] : []);
    updateSettingsStatuses(data, hasWorld);
  }

  function renderModelCatalogNote(hasWorld) {
    var note = byId(IDS.modelCatalogNote);
    if (!note) {
      return;
    }
    note.hidden = hasWorld;
    note.textContent = hasWorld
      ? ""
      : "Мир ещё не рождён: модель скачается сейчас, привязка роли появится после рождения мира.";
  }

  function updateSettingsStatuses(data, hasWorld) {
    var dict = window.I18N || {};
    setStatusText("settings-nav-general-status", "готово", "st--ok");
    var channel = data.channel || {};
    setStatusText("settings-nav-channel-status",
      channel.telegram_configured ? (dict["settings.channel.configured_yes"] || "настроен") : (dict["settings.channel.configured_no"] || "не настроен"),
      channel.telegram_configured ? "st--ok" : "st--wait");
    var llm = data.llm || {};
    setStatusText("settings-nav-llm-status", llm.local === false ? "внешний" : "локально", "");
    var keyed = (data.providers || []).filter(function (p) { return p.key_set; }).length;
    setStatusText("settings-nav-providers-status", keyed ? String(keyed) : "нет ключей", keyed ? "st--ok" : "");
    var installed = (data.models || []).filter(function (m) { return m.status === "installed"; }).length;
    setStatusText("settings-nav-models-status", installed ? String(installed) : "0", installed ? "st--ok" : "");
    setStatusText("settings-nav-world-status", hasWorld ? "активен" : "нет мира", hasWorld ? "st--ok" : "st--wait");
    setStatusText("settings-nav-plugins-status", "нет", "");
    setStatusText("settings-nav-monitor-status", "онлайн", "st--ok");
  }

  function setStatusText(id, text, cls) {
    var el = byId(id);
    if (!el) {
      return;
    }
    el.textContent = text;
    el.classList.remove("st--ok", "st--wait", "st--err");
    if (cls) {
      el.classList.add(cls);
    }
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
    fillProviderCatalog(providers);
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
      return '<div class="settings-provider">' +
        '<div class="settings-provider__head"><b>' + escapeHTML(p.name) + '</b>' +
        '<span class="st ' + (p.key_set ? "st--ok" : "st--err") + '">' +
        (p.key_set ? "ключ задан" : "без ключа") + '</span></div>' +
        '<div class="settings-provider__row">' + (dict["settings.providers.model"] || "Модель") + ': <b>' + model + '</b></div>' +
        '<div class="settings-provider__row">' + (dict["settings.providers.quota"] || "Остаток лимита") + ': ' + quota +
        ' · ' + (dict["settings.providers.cost"] || "Стоимость") + ': ' + cost + '</div>' +
        '</div>';
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
      var html = '<div class="settings-role" data-role="' + escapeHTML(b.role) +
        '" data-provider="' + escapeHTML(b.provider || "local") +
        '" data-model="' + escapeHTML(b.model_ref || "") +
        '" data-temperature="' + escapeHTML(String(b.temperature || 0)) +
        '" data-prompts="' + escapeHTML(b.prompts || "") +
        '" data-priority="' + escapeHTML(String(b.priority || 0)) + '">' +
        '<b>' + escapeHTML(b.role) + '</b>' +
        '<span>' + escapeHTML(b.provider) + (b.model_ref ? ' · ' + escapeHTML(b.model_ref) : '') + '</span>' +
        (b.temperature ? '<span class="chip">t=' + escapeHTML(String(b.temperature)) + '</span>' : '') +
        (b.prompts ? '<span class="chip">' + escapeHTML(b.prompts) + '</span>' : '') +
        (b.priority ? '<span class="chip">p=' + escapeHTML(String(b.priority)) + '</span>' : '') +
        '</div>';
      return html;
    }).join("");
  }

  // Что сейчас привязано к ролям — чтобы не искать это по форме установки.
  function renderSettingsModelsCurrent(bindings, hasWorld) {
    var box = byId(IDS.settingsModelsCurrent);
    if (!box) {
      return;
    }
    if (!hasWorld) {
      box.innerHTML = '<p class="settings-empty">Мир не выбран — привязка ролей появится после рождения мира.</p>';
      return;
    }
    if (!bindings || bindings.length === 0) {
      box.innerHTML = '<p class="settings-empty">Ни одна роль пока не привязана к модели.</p>';
      return;
    }
    box.innerHTML = bindings.map(function (b) {
      return '<div class="settings-current__row"><span class="settings-current__role">' +
        escapeHTML(b.role) + '</span><span>' + escapeHTML(b.provider) +
        (b.model_ref ? ' · ' + escapeHTML(b.model_ref) : '') + '</span>' +
        (b.temperature ? '<span class="chip">t=' + escapeHTML(String(b.temperature)) + '</span>' : '') +
        '</div>';
    }).join("");
  }

  function renderSettingsModels(models, hasWorld) {
    var box = byId(IDS.settingsModels);
    if (!box) {
      return;
    }
    if (!models || models.length === 0) {
      box.innerHTML = '<p class="settings-empty">Модели ещё не скачаны. Выберите карточку выше.</p>';
      return;
    }
    box.innerHTML = models.map(function (m) {
      var pct = typeof m.progress === "number" ? Math.round(m.progress * 100) : 0;
      var downloading = m.status === "downloading";
      var status = m.status === "installed" ? "установлена" : (m.status || "—");
      var html = '<div class="settings-model model-card' + (m.status === "installed" ? ' model-card--installed' : '') + '">';
      html += '<div class="model-card__head"><span class="model-card__title">' + escapeHTML(m.role) + '</span>' +
        '<span class="chip ' + (m.integrity_ok ? "chip--ok" : "chip--danger") + '">' +
        (m.integrity_ok ? "целостность ок" : "целостность нарушена") + "</span></div>";
      html += '<div class="model-card__meta">' + escapeHTML(m.model_ref || "") + "</div>";
      html += '<div class="model-card__status' + (m.status === "error" ? " model-card__status--err" : "") + '">' +
        escapeHTML(status) + " · " + pct + "%</div>";
      if (downloading) {
        html += '<div class="model-card__progress"><span style="width:' + pct + '%"></span></div>';
      }
      if (m.has_previous && hasWorld) {
        html += '<div class="model-card__actions"><button type="button" data-model-role="' +
          escapeHTML(m.role) + '">Откатить</button></div>';
      }
      html += "</div>";
      return html;
    }).join("");
  }

  // Набор известных ролей и установленных моделей для подсказок в формах.
  function renderModelChoices(models) {
    var list = byId(IDS.settingsInstalledModels);
    if (list) {
      list.innerHTML = (models || []).map(function (m) {
        return '<option value="' + escapeHTML(m.model_ref || "") + '"></option>';
      }).join("");
    }
    var select = byId(IDS.settingsRoleModel);
    if (select) {
      var current = select.value;
      select.innerHTML = '<option value="">— не задана —</option>' + (models || []).map(function (m) {
        var ref = m.model_ref || "";
        return '<option value="' + escapeHTML(ref) + '">' + escapeHTML(ref) + "</option>";
      }).join("");
      select.value = current;
    }
  }

  function renderRoleProviderChoices(presets, providers) {
    var select = byId(IDS.settingsRoleProvider);
    if (!select) {
      return;
    }
    var names = ["local"];
    (presets || []).forEach(function (p) {
      if (p.provider && names.indexOf(p.provider) < 0) {
        names.push(p.provider);
      }
    });
    (providers || []).forEach(function (p) {
      if (p.name && names.indexOf(p.name) < 0) {
        names.push(p.name);
      }
    });
    var current = select.value;
    select.innerHTML = names.map(function (name) {
      return '<option value="' + escapeHTML(name) + '">' + escapeHTML(name) + "</option>";
    }).join("");
    select.value = current || "local";
  }

  function fillProviderCatalog(providers) {
    var select = byId("settings-provider-catalog");
    if (!select) {
      return;
    }
    select.innerHTML = '<option value="">— Выберите провайдера —</option>' +
      (providers || []).map(function (p) {
        return '<option value="' + escapeHTML(p.name) + '">' + escapeHTML(p.name) + "</option>";
      }).join("");
  }

  function renderModelCatalog(installed, ramGB) {
    var grid = byId(IDS.modelCatalogGrid);
    if (!grid) {
      return;
    }
    var byRole = {};
    (installed || []).forEach(function (m) {
      byRole[m.role] = m;
    });
    var recommended = recommendedEmbedRef(ramGB);
    var dict = window.I18N || {};
    grid.innerHTML = MODEL_CATALOG.map(function (m) {
      var current = byRole[m.role];
      var isInstalled = !!current && (!current.model_ref || current.model_ref === m.ref);
      var status = isInstalled
        ? "Установлена: " + (current.model_ref || m.ref)
        : "Не установлена";
      var ram = m.ram
        ? '<div class="model-card__meta">' + (dict["settings.models.ram"] || "RAM") + ": " + escapeHTML(m.ram) + "</div>"
        : "";
      var badge = m.ref === recommended
        ? '<span class="chip chip--ok">' + (dict["settings.models.recommended"] || "рекомендуется") + "</span>"
        : "";
      return '<article class="model-card' + (isInstalled ? " model-card--installed" : "") + '"' +
        ' data-catalog-role="' + escapeHTML(m.role) + '"' +
        ' data-catalog-source="' + escapeHTML(m.source) + '"' +
        ' data-catalog-ref="' + escapeHTML(m.ref) + '">' +
        '<div class="model-card__head"><span class="model-card__title">' + escapeHTML(m.label) + "</span>" +
        '<span class="chip">' + escapeHTML(m.size) + "</span></div>" +
        '<div class="model-card__meta">роль: ' + escapeHTML(m.role) + "</div>" +
        '<p class="model-card__meta">' + escapeHTML(m.note) + "</p>" +
        ram +
        badge +
        '<div class="model-card__status" data-model-status>' + escapeHTML(status) + "</div>" +
        '<div class="model-card__progress" hidden><span></span></div>' +
        '<div class="model-card__percent" hidden></div>' +
        '<div class="model-card__actions"><button type="button" data-model-install>' +
        (isInstalled ? "Переустановить" : "Установить") + "</button></div>" +
        "</article>";
    }).join("");
  }

  function recommendedEmbedRef(ramGB) {
    if (typeof ramGB === "number" && ramGB > 0) {
      if (ramGB < 8) {
        return "bge-m3-Q4_K_M";
      }
      if (ramGB >= 16) {
        return "bge-m3-F16";
      }
    }
    return "bge-m3-Q8_0";
  }

  function onModelCatalogClick(event) {
    var btn = event.target.closest("button[data-model-install]");
    if (!btn) {
      return;
    }
    installModelFromCard(btn.closest(".model-card"));
  }

  function installModelFromCard(card) {
    if (!card) {
      return;
    }
    var role = card.getAttribute("data-catalog-role") || "";
    var source = card.getAttribute("data-catalog-source") || "";
    var ref = card.getAttribute("data-catalog-ref") || "";
    if (!role || !source || !ref) {
      return;
    }
    var btn = card.querySelector("button[data-model-install]");
    var release = setBusy(btn);
    setCardProgress(card, 0, "Загрузка…");
    var stopPolling = startModelProgressPoll(role, card);
    postJSON("/api/settings/models/install", {
      world: activeSlugValue(),
      role: role,
      source: source,
      model_ref: ref
    }).then(function (res) {
      setCardProgress(card, 1, res && res.bound === false
        ? "Скачана. Привяжите роль после рождения мира."
        : "Установлена и привязана.");
      return loadSettingsData();
    }).catch(function (err) {
      setCardProgress(card, 0, (err && err.message) || String(err), true);
    }).then(function () {
      stopPolling();
      release();
    });
  }

  function startModelProgressPoll(role, card) {
    var stopped = false;
    var timer = setInterval(function () {
      if (stopped) {
        return;
      }
      apiJSON("/api/settings/models/status?role=" + encodeURIComponent(role)).then(function (data) {
        var job = (data.jobs || [])[0];
        if (!job) {
          return;
        }
        if (job.status === "downloading") {
          setCardProgress(card, job.progress || 0, "Загрузка…");
        } else if (job.status === "error") {
          setCardProgress(card, job.progress || 0, job.error || "Ошибка загрузки", true);
          stopped = true;
          clearInterval(timer);
        }
      }).catch(function () {
        // Опрос прогресса не должен ломать саму установку.
      });
    }, 500);
    return function () {
      stopped = true;
      clearInterval(timer);
    };
  }

  function setCardProgress(card, progress, text, isError) {
    var bar = card.querySelector(".model-card__progress");
    var fill = bar ? bar.querySelector("span") : null;
    var percent = card.querySelector(".model-card__percent");
    var status = card.querySelector("[data-model-status]");
    var pct = Math.max(0, Math.min(100, Math.round((progress || 0) * 100)));
    if (bar) {
      bar.hidden = false;
    }
    if (fill) {
      fill.style.width = pct + "%";
    }
    if (percent) {
      percent.hidden = false;
      percent.textContent = pct + "%";
    }
    if (status && text) {
      status.textContent = text;
      status.classList.toggle("model-card__status--err", !!isError);
    }
    card.classList.toggle("model-card--error", !!isError);
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

  // Если значение пришло из старой привязки и его нет среди текущих опций,
  // добавляем его: иначе сохранение роли молча затрёт модель пустым значением.
  function ensureOption(id, value) {
    var select = byId(id);
    if (!select || !value) {
      return value;
    }
    var exists = Array.prototype.some.call(select.options, function (option) {
      return option.value === value;
    });
    if (!exists) {
      var option = document.createElement("option");
      option.value = value;
      option.textContent = value;
      select.appendChild(option);
    }
    return value;
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
    return putJSON("/api/settings/config", {
      host: fieldValue(IDS.settingsServerHost),
      port: parseInt(fieldValue(IDS.settingsServerPort), 10) || 0
    }).then(function () {
      return putJSON("/api/settings/keys", { keys: { external_vector_search: vectorSearch ? "true" : "false" } });
    }).then(loadSettingsData);
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
    });
  }

  function submitSettingsWorld() {
    return putJSON("/api/settings/config", {
      paganel_enabled: byId(IDS.settingsPaganelEnabled) ? byId(IDS.settingsPaganelEnabled).checked : false,
      scout_enabled: byId(IDS.settingsScoutEnabled) ? byId(IDS.settingsScoutEnabled).checked : false
    }).then(function () {
      var key = fieldValue(IDS.settingsPaganelKey);
      if (!key) {
        return loadSettingsData();
      }
      return putJSON("/api/settings/keys", { keys: { paganel: key } }).then(loadSettingsData);
    });
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
    return putJSON("/api/settings/channel", body).then(loadSettingsData);
  }

  function submitSettingsAdvanced() {
    return putJSON("/api/world/tunables", {
      world: activeSlugValue(),
      tunables: {
        globalLuck: parseFloat(fieldValue(IDS.settingsGlobalLuck)) || 0,
        maxMemoryEntries: parseInt(fieldValue(IDS.settingsMemoryMax), 10) || 0,
        similarityThreshold: parseFloat(fieldValue(IDS.settingsMemorySimilarity)) || 0,
        halfLifeDays: parseFloat(fieldValue(IDS.settingsMemoryHalflife)) || 0,
        dayNormKm: parseFloat(fieldValue(IDS.settingsDayNorm)) || 25
      }
    }).then(loadSettingsData);
  }

  function submitSettingsProvider() {
    var name = fieldValue(IDS.settingsProviderName);
    var key = fieldValue(IDS.settingsProviderKey);
    if (!name || !key) {
      throw new Error("Укажите имя провайдера и ключ");
    }
    var body = { keys: {} };
    body.keys[name] = key;
    return putJSON("/api/settings/keys", body).then(loadSettingsData);
  }

  function submitSettingsModel() {
    var role = fieldValue(IDS.settingsModelRole);
    var source = fieldValue(IDS.settingsModelSource);
    var ref = fieldValue(IDS.settingsModelRef);
    if (!role || !source || !ref) {
      throw new Error("Заполните роль, ссылку и название модели");
    }
    if (source.indexOf("https://") !== 0 && source.indexOf("http://") !== 0) {
      source = "https://" + source.replace(/^\/+/, "");
    }
    return postJSON("/api/settings/models/install", {
      world: activeSlugValue(),
      role: role,
      source: source,
      model_ref: ref
    }).then(function () {
      loadSettingsData();
    });
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
    });
  }

  function submitSettingsRoles() {
    var role = fieldValue(IDS.settingsRoleName);
    var provider = fieldValue(IDS.settingsRoleProvider);
    var model = fieldValue(IDS.settingsRoleModel);
    if (!role || !provider) {
      throw new Error("Укажите роль и провайдера");
    }
    return putJSON("/api/settings/bindings", {
      world: activeSlugValue(),
      bindings: [{
        role: role,
        model_ref: model,
        provider: provider,
        temperature: parseFloat(fieldValue(IDS.settingsRoleTemperature)) || 0,
        prompts: fieldValue(IDS.settingsRolePrompts),
        priority: parseInt(fieldValue(IDS.settingsRolePriority), 10) || 0
      }]
    }).then(loadSettingsData);
  }

  function settingsError(err) {
    toast((err && err.message) || String(err), false);
  }

  // Мастер первого запуска (T-12-20): шаги 0–4, состояние state/first-run.json,
  // идемпотентно — пройденные шаги при повторном запуске не переспрашиваются.
  var WIZARD_STEPS = ["0", "1", "2", "3", "4"];
  var FIRST_RUN_PATH = "/api/first-run";
  var FIRST_RUN_STEP_PATH = "/api/first-run/step";

  function checkFirstRun() {
    return apiJSON(FIRST_RUN_PATH).then(function (data) {
      if (data && data.complete === true) {
        showChat();
        return;
      }
      showWizard(data);
    }).catch(function () {
      // Узнать состояние не удалось — не блокируем игру, показываем чат.
      showChat();
    });
  }

  function showWizard(data) {
    var overlay = byId(IDS.wizardRoot);
    if (overlay) {
      overlay.hidden = false;
      renderWizard(data);
      return;
    }
    fetch("/wizard.html").then(function (res) {
      if (!res.ok) {
        throw new Error("wizard not found");
      }
      return res.text();
    }).then(function (html) {
      var root = document.createElement("div");
      root.id = IDS.wizardRoot;
      root.innerHTML = html;
      document.body.appendChild(root);
      translate(root);
      bindWizard();
      renderWizard(data);
    }).catch(function (err) {
      toast(err.message, false);
      showChat();
    });
  }

  function renderWizard(data) {
    var state = data || {};
    var steps = state.steps || {};
    renderWizardSteps(steps);
    renderWizardStation(state.station || {});
    renderWizardModels(state.station || {});
    loadWizardWorldOptions();
    showWizardStep(firstUndoneWizardStep(steps));
  }

  function firstUndoneWizardStep(steps) {
    for (var i = 0; i < WIZARD_STEPS.length; i++) {
      var s = WIZARD_STEPS[i];
      if (!steps[s] || !steps[s].done) {
        return s;
      }
    }
    return "4";
  }

  function renderWizardSteps(steps) {
    var root = byId(IDS.wizardRoot);
    if (!root) {
      return;
    }
    root.querySelectorAll("[data-wizard-step]").forEach(function (li) {
      var n = li.getAttribute("data-wizard-step");
      li.classList.toggle("is-done", !!(steps[n] && steps[n].done));
    });
  }

  function showWizardStep(n) {
    var root = byId(IDS.wizardRoot);
    if (!root) {
      return;
    }
    root.querySelectorAll("[data-wizard-step-panel]").forEach(function (panel) {
      panel.hidden = panel.getAttribute("data-wizard-step-panel") !== n;
    });
    root.querySelectorAll("[data-wizard-step]").forEach(function (li) {
      li.classList.toggle("is-current", li.getAttribute("data-wizard-step") === n);
    });
  }

  function renderWizardStation(station) {
    var box = byId(IDS.wizardStation);
    if (!box) {
      return;
    }
    var dict = window.I18N || {};
    var rows = [
      { ok: !!station.llama_server, text: dict["wizard.station.llama"] || "llama-server на месте" },
      { ok: station.gen_port_free !== false, text: dict["wizard.station.gen_port"] || "Порт генератора :5001 свободен" },
      { ok: station.embed_port_free !== false, text: dict["wizard.station.embed_port"] || "Порт эмбеддера :5002 свободен" },
      {
        ok: !!station.embed_model,
        text: (dict["wizard.station.embed_model"] || "Модель смысла установлена") + (station.embed_model ? ": " + station.embed_model : "")
      }
    ];
    box.innerHTML = rows.map(function (r) {
      return '<li class="wizard-check ' + (r.ok ? "wizard-check--ok" : "wizard-check--warn") + '">' +
        '<span class="wizard-check__dot"></span>' + escapeHTML(r.text) + "</li>";
    }).join("");
  }

  function renderWizardModels(station) {
    var grid = byId(IDS.wizardModelCards);
    if (!grid) {
      return;
    }
    var dict = window.I18N || {};
    var cards = MODEL_CATALOG.filter(function (m) { return m.role === "embed"; });
    var recommended = recommendedEmbedRef(station.ram_gb);
    grid.innerHTML = cards.map(function (m) {
      var badge = m.ref === recommended
        ? '<span class="chip chip--ok">' + (dict["settings.models.recommended"] || "рекомендуется") + "</span>"
        : "";
      return '<article class="model-card" data-catalog-role="' + escapeHTML(m.role) + '"' +
        ' data-catalog-source="' + escapeHTML(m.source) + '"' +
        ' data-catalog-ref="' + escapeHTML(m.ref) + '">' +
        '<div class="model-card__head"><span class="model-card__title">' + escapeHTML(m.label) + "</span>" +
        '<span class="chip">' + escapeHTML(m.size) + "</span></div>" +
        '<p class="model-card__meta">' + escapeHTML(m.note) + "</p>" +
        badge +
        '<div class="model-card__status" data-model-status>' + escapeHTML(dict["wizard.model.not_installed"] || "Не установлена") + "</div>" +
        '<div class="model-card__progress" hidden><span></span></div>' +
        '<div class="model-card__percent" hidden></div>' +
        '<div class="model-card__actions"><button type="button" data-model-install>' + escapeHTML(dict["settings.models.install"] || "Установить") + "</button></div>" +
        "</article>";
    }).join("");
  }

  function installWizardModel(card) {
    if (!card) {
      return;
    }
    var role = card.getAttribute("data-catalog-role") || "";
    var source = card.getAttribute("data-catalog-source") || "";
    var ref = card.getAttribute("data-catalog-ref") || "";
    if (!role || !source || !ref) {
      return;
    }
    var btn = card.querySelector("button[data-model-install]");
    var release = setBusy(btn);
    setCardProgress(card, 0, "Загрузка…");
    var stopPolling = startModelProgressPoll(role, card);
    postJSON("/api/settings/models/install", {
      world: "",
      role: role,
      source: source,
      model_ref: ref
    }).then(function () {
      setCardProgress(card, 1, "Установлена");
    }).catch(function (err) {
      setCardProgress(card, 0, (err && err.message) || String(err), true);
    }).then(function () {
      stopPolling();
      release();
    });
  }

  function loadWizardWorldOptions() {
    apiJSON("/api/atlas/birth").then(function (data) {
      fillBirthSelect(IDS.wizardWorldSocial, data.social_orders || [], "феодализм");
      fillBirthSelect(IDS.wizardWorldRating, data.ratings || [], "16+");
      var box = byId(IDS.wizardWorldGenres);
      if (box) {
        box.innerHTML = (data.genres || []).map(function (g) {
          return '<label class="choice"><input type="checkbox" value="' + escapeHTML(g) + '"' +
            (g === "фэнтези" ? " checked" : "") + "><span>" + escapeHTML(displayChoice(g)) + "</span></label>";
        }).join("");
      }
    }).catch(function () {
      // Перечни не загрузились — форма останется пустой, но мастера не ломает.
    });
  }

  function renderWizardLibrary(books) {
    var list = byId(IDS.wizardLibraryList);
    if (!list) {
      return;
    }
    var dict = window.I18N || {};
    if (!books || books.length === 0) {
      list.innerHTML = '<li class="wizard-empty">' + escapeHTML(dict["wizard.library.empty"] || "Полка пуста") + "</li>";
      return;
    }
    list.innerHTML = books.map(function (b) {
      return '<li><span>' + escapeHTML(b.name || b.id) + "</span>" +
        '<span class="chip">' + escapeHTML(b.status || "") + "</span></li>";
    }).join("");
  }

  function onWizardModelClick(event) {
    var btn = event.target.closest("button[data-model-install]");
    if (btn) {
      installWizardModel(btn.closest(".model-card"));
    }
  }

  function onWizardLibrarySubmit(event) {
    event.preventDefault();
    var fileInput = byId(IDS.wizardLibraryFile);
    var file = fileInput && fileInput.files && fileInput.files[0];
    if (!file) {
      toast(window.I18N["wizard.library.pick"] || "Выберите файл книги", false);
      return;
    }
    var form = new FormData();
    form.append("file", file);
    var release = setBusy(event.submitter);
    fetch("/api/library/import", {
      method: "POST",
      body: form,
      headers: { Accept: "application/json" }
    }).then(function (res) {
      return res.json().then(function (body) {
        if (!res.ok) {
          throw new Error((body && body.error && body.error.message) || res.statusText);
        }
        return body;
      });
    }).then(function (data) {
      renderWizardLibrary(data.books || []);
      formResult(byId(IDS.wizardLibraryForm), window.I18N["wizard.library.imported"] || "Книга на полке", true);
    }).catch(function (err) {
      formResult(byId(IDS.wizardLibraryForm), (err && err.message) || String(err), false);
    }).then(function () {
      release();
    });
  }

  function onWizardWorldSubmit(event) {
    event.preventDefault();
    var title = fieldValue(IDS.wizardWorldTitle);
    if (!title) {
      formResult(byId(IDS.wizardWorldForm), window.I18N["wizard.world.need_title"] || "Укажите название мира", false);
      return;
    }
    var genres = checkedBirthChoices(IDS.wizardWorldGenres);
    if (genres.length === 0) {
      genres = ["фэнтези"];
    }
    var body = {
      title: title,
      synopsis: fieldValue(IDS.wizardWorldSynopsis),
      lang: "ru",
      travel_mode: "вымышленная",
      genres: genres,
      social_order: fieldValue(IDS.wizardWorldSocial) || "феодализм",
      censorship: fieldValue(IDS.wizardWorldRating) || "16+"
    };
    var release = setBusy(event.submitter);
    postJSON("/api/atlas/worlds/birth", body).then(function (res) {
      var slug = res && res.world && res.world.slug;
      if (slug) {
        window.__activeSlug = slug;
        window.__activeWorldChecked = true;
      }
      formResult(byId(IDS.wizardWorldForm), window.I18N["wizard.world.created"] || "Мир создан", true);
      var nextBtn = byId(IDS.wizardStep3Next);
      if (nextBtn) {
        nextBtn.hidden = false;
      }
    }).catch(function (err) {
      formResult(byId(IDS.wizardWorldForm), (err && err.message) || String(err), false);
    }).then(function () {
      release();
    });
  }

  function wizardMarkStep(step, thenFn) {
    return putJSON(FIRST_RUN_STEP_PATH, { step: step }).then(function (data) {
      renderWizardSteps((data && data.steps) || {});
      if (thenFn) {
        thenFn(data);
      }
    }).catch(function (err) {
      toast((err && err.message) || String(err), false);
    });
  }

  function wizardNext(step, next) {
    wizardMarkStep(step, function () {
      showWizardStep(next);
    });
  }

  function wizardDone() {
    wizardMarkStep("4", function () {
      var root = byId(IDS.wizardRoot);
      if (root) {
        root.hidden = true;
      }
      showChat();
      if (window.Chat && typeof window.Chat.refresh === "function") {
        window.Chat.refresh();
      }
    });
  }

  function bindWizard() {
    var root = byId(IDS.wizardRoot);
    if (!root) {
      return;
    }
    var step0Next = byId(IDS.wizardStep0Next);
    if (step0Next) {
      step0Next.addEventListener("click", function () { wizardNext("0", "1"); });
    }
    var step1Next = byId(IDS.wizardStep1Next);
    if (step1Next) {
      step1Next.addEventListener("click", function () { wizardNext("1", "2"); });
    }
    var step2Skip = byId(IDS.wizardStep2Skip);
    if (step2Skip) {
      step2Skip.addEventListener("click", function () { wizardNext("2", "3"); });
    }
    var step2Next = byId(IDS.wizardStep2Next);
    if (step2Next) {
      step2Next.addEventListener("click", function () { wizardNext("2", "3"); });
    }
    var step3Next = byId(IDS.wizardStep3Next);
    if (step3Next) {
      step3Next.addEventListener("click", function () { wizardNext("3", "4"); });
    }
    var step4Done = byId(IDS.wizardStep4Done);
    if (step4Done) {
      step4Done.addEventListener("click", wizardDone);
    }
    var modelGrid = byId(IDS.wizardModelCards);
    if (modelGrid) {
      modelGrid.addEventListener("click", onWizardModelClick);
    }
    var libraryForm = byId(IDS.wizardLibraryForm);
    if (libraryForm) {
      libraryForm.addEventListener("submit", onWizardLibrarySubmit);
    }
    var worldForm = byId(IDS.wizardWorldForm);
    if (worldForm) {
      worldForm.addEventListener("submit", onWizardWorldSubmit);
    }
  }

  applyThemeClass(resolveTheme());
  translate(document);
  checkFirstRun();
  subscribeMcpWS();
  subscribeMonitorWS();
})();
