/* Theme manager — reads/writes localStorage('tns-theme') + custom theme vars */
(function() {
  var KEY = 'tns-theme';
  var CUSTOM_KEY = 'tns-theme-custom';
  var html = document.documentElement;
  var styleEl = null;

  var DEFAULTS = {
    'black': '#000000',
    'surface': '#080808',
    'surface-raised': '#111111',
    'surface-hover': '#161616',
    'border': '#1A1A1A',
    'border-visible': '#272727',
    'border-strong': '#353535',
    'text-disabled': '#666666',
    'text-tertiary': '#8E8E8E',
    'text-secondary': '#BBBBBB',
    'text-primary': '#E0E0E0',
    'text-display': '#FFFFFF',
    'accent': '#D71921',
    'success': '#4A9E5C',
    'warning': '#D4A843',
    'interactive': '#5B9BF6'
  };

  var FONTS = {
    'font-mono': "'Space Mono', monospace",
    'font-body': "'Space Grotesk', 'DM Sans', system-ui, sans-serif",
    'font-display': "'Doto', 'Space Mono', monospace"
  };

  function getTheme() {
    try { return localStorage.getItem(KEY) || 'dark'; } catch(e) { return 'dark'; }
  }

  function getCustomVars() {
    try { return JSON.parse(localStorage.getItem(CUSTOM_KEY)) || {}; } catch(e) { return {}; }
  }

  function saveCustomVars(vars) {
    try { localStorage.setItem(CUSTOM_KEY, JSON.stringify(vars)); } catch(e) {}
  }

  function applyCustomVars() {
    var vars = getCustomVars();
    var css = '';
    for (var k in vars) {
      if (vars[k]) css += '--' + k + ':' + vars[k] + ';';
    }
    if (!styleEl) {
      styleEl = document.createElement('style');
      styleEl.id = 'tns-custom-theme';
      document.head.appendChild(styleEl);
    }
    styleEl.textContent = css ? '[data-theme="custom"]{' + css + '}' : '';
  }

  function setTheme(t) {
    html.setAttribute('data-theme', t);
    try { localStorage.setItem(KEY, t); } catch(e) {}
    if (t === 'custom') applyCustomVars();
    updateSelectors(t);
    updateCustomPanel(t);
  }

  function updateSelectors(t) {
    var btns = document.querySelectorAll('.theme-selector__btn');
    for (var i = 0; i < btns.length; i++) {
      var isActive = btns[i].getAttribute('data-theme-value') === t;
      btns[i].classList.toggle('theme-selector__btn--active', isActive);
    }
  }

  function updateCustomPanel(t) {
    var panel = document.getElementById('customThemePanel');
    if (panel) panel.style.display = t === 'custom' ? 'block' : 'none';
  }

  function toggle() {
    setTheme(getTheme() === 'dark' ? 'light' : 'dark');
  }

  /* ── Custom theme constructor ── */

  function initCustomPanel() {
    var panel = document.getElementById('customThemePanel');
    if (!panel) return;

    var vars = getCustomVars();
    var inputs = panel.querySelectorAll('[data-var]');
    for (var i = 0; i < inputs.length; i++) {
      var el = inputs[i];
      var varName = el.getAttribute('data-var');
      var val = vars[varName] || DEFAULTS[varName] || '';
      if (el.type === 'color') {
        el.value = val;
      } else {
        el.value = val;
      }
      el.addEventListener('input', onCustomInput);
    }

    var fontSelects = panel.querySelectorAll('[data-font]');
    for (var j = 0; j < fontSelects.length; j++) {
      var fs = fontSelects[j];
      var fontKey = fs.getAttribute('data-font');
      var fval = vars[fontKey] || FONTS[fontKey] || '';
      fs.value = fval;
      fs.addEventListener('change', onCustomInput);
    }

    var resetBtn = document.getElementById('customThemeReset');
    if (resetBtn) resetBtn.addEventListener('click', function() {
      saveCustomVars({});
      var inputs2 = panel.querySelectorAll('[data-var]');
      for (var k = 0; k < inputs2.length; k++) {
        var v = inputs2[k].getAttribute('data-var');
        if (inputs2[k].type === 'color') inputs2[k].value = DEFAULTS[v] || '#000000';
        else inputs2[k].value = DEFAULTS[v] || '';
      }
      var fselects = panel.querySelectorAll('[data-font]');
      for (var m = 0; m < fselects.length; m++) {
        var fk = fselects[m].getAttribute('data-font');
        fselects[m].value = FONTS[fk] || '';
      }
      applyCustomVars();
    });

    updateCustomPanel(getTheme());
  }

  function onCustomInput() {
    var panel = document.getElementById('customThemePanel');
    if (!panel) return;
    var vars = {};
    var inputs = panel.querySelectorAll('[data-var]');
    for (var i = 0; i < inputs.length; i++) {
      var k = inputs[i].getAttribute('data-var');
      vars[k] = inputs[i].value;
    }
    var fontSelects = panel.querySelectorAll('[data-font]');
    for (var j = 0; j < fontSelects.length; j++) {
      var fk = fontSelects[j].getAttribute('data-font');
      vars[fk] = fontSelects[j].value;
    }
    saveCustomVars(vars);
    applyCustomVars();
  }

  /* ── Init ── */

  setTheme(getTheme());

  function wire() {
    var btns = document.querySelectorAll('.theme-selector__btn');
    for (var i = 0; i < btns.length; i++) {
      btns[i].addEventListener('click', function() {
        var t = this.getAttribute('data-theme-value');
        if (t) setTheme(t);
      });
    }
    updateSelectors(getTheme());
    initCustomPanel();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', wire);
  } else {
    wire();
  }

  window.TNSTheme = { get: getTheme, set: setTheme, toggle: toggle, getCustomVars: getCustomVars, saveCustomVars: saveCustomVars };
})();
