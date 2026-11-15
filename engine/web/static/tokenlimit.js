// Счётчик токенов (IMP-7): та же целочисленная формула и те же константы, что
// в internal/textlimit — расхождения ловит js_parity_test.
// TOKENLIMIT-CONST {"cyrillic_weight":2,"other_weight":1,"units_per_token":4,"default_limit":2000}
(function (global) {
  "use strict";

  var CONST = {
    cyrillic_weight: 2,
    other_weight: 1,
    units_per_token: 4,
    default_limit: 2000
  };
  var KINDS = {
    playerMessage: "Реплика игрока",
    worldDescription: "Описание мира"
  };

  function weight(code) {
    return (code >= 0x0400 && code <= 0x04FF) ? CONST.cyrillic_weight : CONST.other_weight;
  }

  function estimate(text) {
    var s = String(text == null ? "" : text);
    var units = 0;
    for (var i = 0; i < s.length; i++) {
      var code = s.charCodeAt(i);
      if (code >= 0xD800 && code <= 0xDBFF && i + 1 < s.length) {
        // Суррогатная пара — одна руна вне кириллицы.
        units += CONST.other_weight;
        i += 1;
        continue;
      }
      units += weight(code);
    }
    return units === 0 ? 0 : Math.ceil(units / CONST.units_per_token);
  }

  function hint(kind, tokens, limit) {
    return kind + " длиннее " + limit + " токенов (≈" + tokens + "): сократите текст";
  }

  function check(kind, text, limit) {
    var lim = limit > 0 ? limit : CONST.default_limit;
    var tokens = estimate(text);
    if (tokens <= lim) {
      return null;
    }
    return { kind: kind, tokens: tokens, limit: lim, message: hint(kind, tokens, lim) };
  }

  var api = {
    estimate: estimate,
    hint: hint,
    check: check,
    KINDS: KINDS,
    CONST: CONST,
    DEFAULT_LIMIT: CONST.default_limit
  };
  global.TokenLimit = api;
  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }
})(typeof window !== "undefined" ? window : globalThis);
