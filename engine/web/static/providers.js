"use strict";

// Каталог провайдеров (T-11-01): выбор в списке «Быстрое добавление» подставляет
// имя провайдера в поле формы и показывает его base URL в строке подсказки.
// Только клиентская подстановка: новых запросов к станку нет.
(function () {
  if (window.Providers && window.Providers.bound) {
    return;
  }

  var CATALOG_ID = "settings-provider-catalog";
  var HINT_ID = "settings-provider-catalog-hint";
  var NAME_ID = "settings-provider-name";

  function byId(id) {
    return document.getElementById(id);
  }

  function dict() {
    return window.I18N || {};
  }

  function key(name, fallback) {
    var value = dict()[name];
    return typeof value === "string" && value ? value : fallback;
  }

  // Имя провайдера — часть подписи до разделителя «—».
  function providerName(label) {
    var text = String(label || "").split("\u2014")[0].trim();
    return text || String(label || "").trim();
  }

  function idleHint() {
    return key("settings.providers.catalog.hint", "Выберите провайдера — имя подставится в форму, base URL появится здесь.");
  }

  function baseURLHint(baseURL) {
    return key("settings.providers.catalog.base_url", "Base URL") + ": " + baseURL;
  }

  function selectedOption(select) {
    if (!select || select.selectedIndex < 0) {
      return null;
    }
    return select.options[select.selectedIndex] || null;
  }

  function applySelection(select) {
    var option = selectedOption(select);
    var hint = byId(HINT_ID);
    var nameField = byId(NAME_ID);
    var baseURL = option ? option.getAttribute("data-base-url") || "" : "";
    var known = !!(option && option.value);
    if (nameField && known) {
      nameField.value = providerName(option.textContent);
    }
    if (hint) {
      hint.textContent = known && baseURL ? baseURLHint(baseURL) : idleHint();
    }
    return known;
  }

  function onDocumentChange(event) {
    var target = event.target;
    if (!target || target.id !== CATALOG_ID) {
      return;
    }
    applySelection(target);
  }

  document.addEventListener("change", onDocumentChange);

  window.Providers = {
    bound: true,
    applySelection: function () {
      return applySelection(byId(CATALOG_ID));
    }
  };

  // Разметка настроек может быть уже на странице (прямой заход на /settings.html).
  if (byId(CATALOG_ID)) {
    applySelection(byId(CATALOG_ID));
  }
})();
