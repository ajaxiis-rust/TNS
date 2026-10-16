/* Theme Builder — presets, color/font controls, live preview, export/import */
(function() {
  var CUSTOM_KEY = 'tns-theme-custom';
  var THEME_KEY = 'tns-theme';

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

  var PRESETS = {
    dark: { label: 'Dark', swatch: '#000000', vars: {
      black:'#000000',surface:'#080808','surface-raised':'#111111','surface-hover':'#161616',
      border:'#1A1A1A','border-visible':'#272727','border-strong':'#353535',
      'text-disabled':'#666666','text-tertiary':'#8E8E8E','text-secondary':'#BBBBBB','text-primary':'#E0E0E0','text-display':'#FFFFFF',
      accent:'#D71921',success:'#4A9E5C',warning:'#D4A843',interactive:'#5B9BF6'
    }},
    light: { label: 'Light', swatch: '#F5F5F5', vars: {
      black:'#FFFFFF',surface:'#F5F5F5','surface-raised':'#ECECEC','surface-hover':'#E0E0E0',
      border:'#E0E0E0','border-visible':'#CCCCCC','border-strong':'#AAAAAA',
      'text-disabled':'#999999','text-tertiary':'#666666','text-secondary':'#444444','text-primary':'#222222','text-display':'#000000',
      accent:'#D71921',success:'#2E7D32',warning:'#F9A825',interactive:'#1565C0'
    }},
    terminal: { label: 'Terminal', swatch: '#0A0F0A', vars: {
      black:'#0A0F0A',surface:'#0D120D','surface-raised':'#111811','surface-hover':'#151C15',
      border:'#1A2A1A','border-visible':'#2A3A2A','border-strong':'#3A4A3A',
      'text-disabled':'#336633','text-tertiary':'#448844','text-secondary':'#66BB66','text-primary':'#88FF88','text-display':'#AAFFAA',
      accent:'#FF3333',success:'#44FF44',warning:'#FFFF44',interactive:'#44FFFF'
    }},
    cyberpunk: { label: 'Cyber', swatch: '#0A0014', vars: {
      black:'#0A0014',surface:'#100020','surface-raised':'#180030','surface-hover':'#200040',
      border:'#300050','border-visible':'#4400AA','border-strong':'#6600CC',
      'text-disabled':'#6644AA','text-tertiary':'#8866CC','text-secondary':'#AA88EE','text-primary':'#CCAAFF','text-display':'#FFCCFF',
      accent:'#FF0066',success:'#00FF88',warning:'#FFAA00',interactive:'#00CCFF'
    }},
    dracula: { label: 'Dracula', swatch: '#282A36', vars: {
      black:'#282A36',surface:'#2D303E','surface-raised':'#343746','surface-hover':'#3C3F58',
      border:'#44475A','border-visible':'#6272A4','border-strong':'#7284A8',
      'text-disabled':'#6272A4','text-tertiary':'#818DA0','text-secondary':'#BFBFBF','text-primary':'#F8F8F2','text-display':'#FFFFFF',
      accent:'#FF5555',success:'#50FA7B',warning:'#F1FA8C',interactive:'#BD93F9'
    }},
    nord: { label: 'Nord', swatch: '#2E3440', vars: {
      black:'#2E3440',surface:'#3B4252','surface-raised':'#434C5E','surface-hover':'#4C566A',
      border:'#4C566A','border-visible':'#616E88','border-strong':'#7B88A1',
      'text-disabled':'#4C566A','text-tertiary':'#7B88A1','text-secondary':'#D8DEE9','text-primary':'#ECEFF4','text-display':'#FFFFFF',
      accent:'#BF616A',success:'#A3BE8C',warning:'#EBCB8B',interactive:'#88C0D0'
    }},
    monokai: { label: 'Monokai', swatch: '#272822', vars: {
      black:'#272822',surface:'#2D2E27','surface-raised':'#3E3D32','surface-hover':'#49483E',
      border:'#49483E','border-visible':'#75715E','border-strong':'#90908A',
      'text-disabled':'#75715E','text-tertiary':'#90908A','text-secondary':'#D6D6D6','text-primary':'#F8F8F2','text-display':'#FFFFFF',
      accent:'#F92672',success:'#A6E22E',warning:'#E6DB74',interactive:'#66D9EF'
    }},
    solarized: { label: 'Solarized', swatch: '#002B36', vars: {
      black:'#002B36',surface:'#073642','surface-raised':'#0A3D4C','surface-hover':'#0E4252',
      border:'#0E4252','border-visible':'#586E75','border-strong':'#657B83',
      'text-disabled':'#586E75','text-tertiary':'#657B83','text-secondary':'#93A1A1','text-primary':'#EEE8D5','text-display':'#FDF6E3',
      accent:'#DC322F',success:'#859900',warning:'#B58900',interactive:'#268BD2'
    }},
    gruvbox: { label: 'Gruvbox', swatch: '#282828', vars: {
      black:'#282828',surface:'#3C3836','surface-raised':'#504945','surface-hover':'#665C54',
      border:'#665C54','border-visible':'#7C6F64','border-strong':'#928374',
      'text-disabled':'#7C6F64','text-tertiary':'#928374','text-secondary':'#D5C4A1','text-primary':'#EBDBB2','text-display':'#FBF1C7',
      accent:'#FB4934',success:'#B8BB26',warning:'#FABD2F',interactive:'#83A598'
    }},
    tokyonight: { label: 'Tokyo Night', swatch: '#1A1B26', vars: {
      black:'#1A1B26',surface:'#24283B','surface-raised':'#292E42','surface-hover':'#33384A',
      border:'#33384A','border-visible':'#444B6A','border-strong':'#565F89',
      'text-disabled':'#444B6A','text-tertiary':'#565F89','text-secondary':'#A9B1D6','text-primary':'#C0CAF5','text-display':'#FFFFFF',
      accent:'#F7768E',success:'#9ECE6A',warning:'#E0AF68',interactive:'#7AA2F7'
    }},
    onedark: { label: 'One Dark', swatch: '#282C34', vars: {
      black:'#282C34',surface:'#2C313A','surface-raised':'#333842','surface-hover':'#3B4048',
      border:'#3B4048','border-visible':'#5C6370','border-strong':'#6B7280',
      'text-disabled':'#5C6370','text-tertiary':'#6B7280','text-secondary':'#ABB2BF','text-primary':'#E0E0E0','text-display':'#FFFFFF',
      accent:'#E06C75',success:'#98C379',warning:'#E5C07B',interactive:'#61AFEF'
    }},
    catppuccin: { label: 'Catppuccin', swatch: '#1E1E2E', vars: {
      black:'#1E1E2E',surface:'#313244','surface-raised':'#45475A','surface-hover':'#585B70',
      border:'#45475A','border-visible':'#6C7086','border-strong':'#7F849C',
      'text-disabled':'#6C7086','text-tertiary':'#7F849C','text-secondary':'#BAC2DE','text-primary':'#CDD6F4','text-display':'#FFFFFF',
      accent:'#F38BA8',success:'#A6E3A1',warning:'#F9E2AF',interactive:'#89B4FA'
    }},
    custom: { label: 'Custom', swatch: 'linear-gradient(135deg,#D71921,#5B9BF6)', vars: null }
  };

  var COLOR_GROUPS = {
    backgrounds: [
      { key: 'black', label: 'Background' },
      { key: 'surface', label: 'Surface' },
      { key: 'surface-raised', label: 'Raised' },
      { key: 'surface-hover', label: 'Hover' }
    ],
    borders: [
      { key: 'border', label: 'Border' },
      { key: 'border-visible', label: 'Visible' },
      { key: 'border-strong', label: 'Strong' }
    ],
    text: [
      { key: 'text-disabled', label: 'Disabled' },
      { key: 'text-tertiary', label: 'Tertiary' },
      { key: 'text-secondary', label: 'Secondary' },
      { key: 'text-primary', label: 'Primary' },
      { key: 'text-display', label: 'Display' }
    ],
    accents: [
      { key: 'accent', label: 'Accent' },
      { key: 'success', label: 'Success' },
      { key: 'warning', label: 'Warning' },
      { key: 'interactive', label: 'Interactive' }
    ]
  };

  var FONT_OPTIONS = [
    { key: 'font-mono', label: 'Mono', options: [
      ["'Space Mono',monospace", 'Space Mono'],
      ["'Fira Code',monospace", 'Fira Code'],
      ["'JetBrains Mono',monospace", 'JetBrains Mono'],
      ["'IBM Plex Mono',monospace", 'IBM Plex Mono'],
      ["monospace", 'System Mono']
    ]},
    { key: 'font-body', label: 'Body', options: [
      ["'Space Grotesk','DM Sans',system-ui,sans-serif", 'Space Grotesk'],
      ["'Inter',system-ui,sans-serif", 'Inter'],
      ["'Roboto',system-ui,sans-serif", 'Roboto'],
      ["'Nunito',system-ui,sans-serif", 'Nunito'],
      ["system-ui,sans-serif", 'System UI']
    ]},
    { key: 'font-display', label: 'Display', options: [
      ["'Doto','Space Mono',monospace", 'Doto'],
      ["'Orbitron',sans-serif", 'Orbitron'],
      ["'Rajdhani',sans-serif", 'Rajdhani'],
      ["'Space Mono',monospace", 'Space Mono']
    ]}
  ];

  var allVarKeys = [];
  Object.keys(COLOR_GROUPS).forEach(function(g) {
    COLOR_GROUPS[g].forEach(function(v) { allVarKeys.push(v.key); });
  });

  var currentVars = {};
  var activePreset = null;
  var previewStyleEl = null;

  /* ── State Management ── */

  function getVars() {
    var stored = {};
    try { stored = JSON.parse(localStorage.getItem(CUSTOM_KEY)) || {}; } catch(e) {}
    var vars = {};
    allVarKeys.forEach(function(k) {
      vars[k] = stored[k] || DEFAULTS[k] || '#000000';
    });
    FONT_OPTIONS.forEach(function(f) {
      vars[f.key] = stored[f.key] || FONTS[f.key] || '';
    });
    return vars;
  }

  function saveVars(vars) {
    try { localStorage.setItem(CUSTOM_KEY, JSON.stringify(vars)); } catch(e) {}
  }

  function applyVars(vars) {
    var css = '';
    for (var k in vars) {
      if (vars[k]) css += '--' + k + ':' + vars[k] + ';';
    }
    if (!previewStyleEl) {
      previewStyleEl = document.createElement('style');
      previewStyleEl.id = 'preview-override';
      document.head.appendChild(previewStyleEl);
    }
    previewStyleEl.textContent = '[data-theme="custom"]{' + css + '}';
  }

  function getComputed(varName) {
    return getComputedStyle(document.documentElement).getPropertyValue('--' + varName).trim();
  }

  /* ── Preset Grid Rendering ── */

  function renderPresets() {
    var el = document.getElementById('presets');
    if (!el) return;
    el.innerHTML = '';
    var keys = Object.keys(PRESETS);
    keys.forEach(function(key) {
      var p = PRESETS[key];
      var btn = document.createElement('button');
      btn.className = 'preset' + (key === activePreset ? ' preset--active' : '');
      btn.setAttribute('data-preset', key);

      var colors = document.createElement('div');
      colors.className = 'preset__colors';
      var swatchKeys = ['black', 'accent', 'success', 'warning', 'interactive', 'text-primary'];
      swatchKeys.forEach(function(sk) {
        var sw = document.createElement('div');
        sw.className = 'preset__swatch';
        var val = (p.vars && p.vars[sk]) || DEFAULTS[sk] || '#000';
        if (p.swatch && p.swatch.indexOf('gradient') !== -1 && sk === 'black') {
          sw.style.background = p.swatch;
        } else {
          sw.style.background = val;
        }
        colors.appendChild(sw);
      });

      var name = document.createElement('span');
      name.className = 'preset__name';
      name.textContent = p.label;

      btn.appendChild(colors);
      btn.appendChild(name);
      btn.onclick = function() { onPresetClick(key); };
      el.appendChild(btn);
    });
  }

  function onPresetClick(key) {
    var preset = PRESETS[key];
    if (!preset) return;

    if (preset.vars) {
      Object.keys(preset.vars).forEach(function(k) { currentVars[k] = preset.vars[k]; });
    }

    var builtinThemes = ['dark', 'light', 'terminal', 'cyberpunk'];
    if (builtinThemes.indexOf(key) !== -1) {
      if (typeof TNSTheme !== 'undefined') TNSTheme.set(key);
    } else {
      saveVars(currentVars);
      if (typeof TNSTheme !== 'undefined') TNSTheme.set('custom');
    }

    activePreset = key;
    saveVars(currentVars);
    updateColorInputs();
    updateFontInputs();
    renderPreview();
    renderPresets();
    toast('Preset: ' + preset.label, true);
  }

  function detectActivePreset() {
    var keys = Object.keys(PRESETS);
    for (var i = 0; i < keys.length; i++) {
      var p = PRESETS[keys[i]];
      if (!p.vars) continue;
      var match = true;
      for (var k in p.vars) {
        if ((currentVars[k] || '').toLowerCase() !== p.vars[k].toLowerCase()) { match = false; break; }
      }
      if (match) return keys[i];
    }
    return null;
  }

  /* ── Color Controls ── */

  function renderColorControls() {
    var containerIds = { backgrounds: 'colors-bg', borders: 'colors-border', text: 'colors-text', accents: 'colors-accent' };
    Object.keys(COLOR_GROUPS).forEach(function(group) {
      var container = document.getElementById(containerIds[group]);
      if (!container) return;
      container.innerHTML = '';
      COLOR_GROUPS[group].forEach(function(v) {
        var row = document.createElement('div');
        row.className = 'color-row';
        var swatch = document.createElement('div');
        swatch.className = 'color-row__swatch';
        swatch.style.background = currentVars[v.key] || '#000000';
        var colorInput = document.createElement('input');
        colorInput.type = 'color';
        colorInput.value = currentVars[v.key] || '#000000';
        colorInput.setAttribute('data-var', v.key);
        colorInput.oninput = function() { onColorInput(v.key, this.value, swatch, valSpan); };
        swatch.appendChild(colorInput);
        var label = document.createElement('span');
        label.className = 'color-row__label';
        label.textContent = v.label;
        var valSpan = document.createElement('span');
        valSpan.className = 'color-row__value';
        valSpan.textContent = currentVars[v.key] || '#000000';
        valSpan.onclick = function() { colorInput.click(); };
        row.appendChild(swatch);
        row.appendChild(label);
        row.appendChild(valSpan);
        container.appendChild(row);
      });
    });
  }

  function updateColorInputs() {
    var inputs = document.querySelectorAll('.color-row__swatch input[type="color"]');
    inputs.forEach(function(input) {
      var varName = input.getAttribute('data-var');
      if (currentVars[varName]) {
        input.value = currentVars[varName];
        input.parentElement.style.background = currentVars[varName];
        var valSpan = input.closest('.color-row').querySelector('.color-row__value');
        if (valSpan) valSpan.textContent = currentVars[varName];
      }
    });
  }

  function onColorInput(varName, value, swatchEl, valEl) {
    currentVars[varName] = value;
    if (swatchEl) swatchEl.style.background = value;
    if (valEl) valEl.textContent = value;
    activePreset = null;
    saveVars(currentVars);
    if (typeof TNSTheme !== 'undefined') TNSTheme.set('custom');
    renderPreview();
    renderPresets();
  }

  /* ── Font Controls ── */

  function renderFontControls() {
    var el = document.getElementById('font-selectors');
    if (!el) return;
    el.innerHTML = '';
    FONT_OPTIONS.forEach(function(f) {
      var row = document.createElement('div');
      row.className = 'font-row';
      var label = document.createElement('span');
      label.className = 'font-row__label';
      label.textContent = f.label;
      var sel = document.createElement('select');
      sel.className = 'font-row__select';
      sel.setAttribute('data-font', f.key);
      f.options.forEach(function(opt) {
        var option = document.createElement('option');
        option.value = opt[0];
        option.textContent = opt[1];
        if (currentVars[f.key] === opt[0]) option.selected = true;
        sel.appendChild(option);
      });
      sel.onchange = function() { onFontInput(f.key, this.value); };
      row.appendChild(label);
      row.appendChild(sel);
      el.appendChild(row);
    });
  }

  function updateFontInputs() {
    var selects = document.querySelectorAll('.font-row__select');
    selects.forEach(function(sel) {
      var fontKey = sel.getAttribute('data-font');
      if (currentVars[fontKey]) sel.value = currentVars[fontKey];
    });
  }

  function onFontInput(fontKey, value) {
    currentVars[fontKey] = value;
    activePreset = null;
    saveVars(currentVars);
    if (typeof TNSTheme !== 'undefined') TNSTheme.set('custom');
    renderPreview();
    renderPresets();
  }

  /* ── Live Preview ── */

  function renderPreview() {
    renderPalettePreview();
  }

  function renderPalettePreview() {
    var el = document.getElementById('preview-palette');
    if (!el) return;
    el.innerHTML = '';
    var allVars = [].concat(
      COLOR_GROUPS.backgrounds, COLOR_GROUPS.borders, COLOR_GROUPS.text, COLOR_GROUPS.accents
    );
    allVars.forEach(function(v) {
      var swatch = document.createElement('div');
      swatch.className = 'preview-swatch';
      swatch.style.background = currentVars[v.key] || '#000000';
      var nameEl = document.createElement('div');
      nameEl.className = 'preview-swatch__name';
      nameEl.textContent = v.label;
      nameEl.style.color = v.key.indexOf('text') === 0 ? currentVars['text-display'] || '#fff' : '#fff';
      var hexEl = document.createElement('div');
      hexEl.className = 'preview-swatch__hex';
      hexEl.textContent = currentVars[v.key] || '#000000';
      hexEl.style.color = v.key.indexOf('text') === 0 ? currentVars['text-display'] || '#fff' : '#fff';
      swatch.appendChild(nameEl);
      swatch.appendChild(hexEl);
      el.appendChild(swatch);
    });
  }

  /* ── Export / Import ── */

  function exportTheme() {
    var data = {
      name: activePreset ? PRESETS[activePreset].label : 'Custom',
      version: 1,
      vars: currentVars,
      timestamp: new Date().toISOString()
    };
    var blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
    var url = URL.createObjectURL(blob);
    var a = document.createElement('a');
    a.href = url;
    a.download = 'tns-theme-' + Date.now() + '.json';
    a.click();
    URL.revokeObjectURL(url);
    toast('Theme exported', true);
  }

  function importTheme() {
    document.getElementById('importInput').click();
  }

  function onImportFile(e) {
    var file = e.target.files[0];
    if (!file) return;
    var reader = new FileReader();
    reader.onload = function(ev) {
      try {
        var data = JSON.parse(ev.target.result);
        var vars = data.vars || data.colors || {};
        Object.keys(vars).forEach(function(k) { currentVars[k] = vars[k]; });
        activePreset = detectActivePreset();
        saveVars(currentVars);
        if (typeof TNSTheme !== 'undefined') TNSTheme.set('custom');
        updateColorInputs();
        updateFontInputs();
        renderPreview();
        renderPresets();
        toast('Theme imported', true);
      } catch(err) {
        toast('Import failed: ' + err.message, false);
      }
    };
    reader.readAsText(file);
    e.target.value = '';
  }

  /* ── Actions ── */

  function applyTheme() {
    saveVars(currentVars);
    if (typeof TNSTheme !== 'undefined') TNSTheme.set('custom');
    toast('Theme applied', true);
  }

  function resetColors() {
    var preset = PRESETS.dark;
    Object.keys(preset.vars).forEach(function(k) { currentVars[k] = preset.vars[k]; });
    FONT_OPTIONS.forEach(function(f) { currentVars[f.key] = f.options[0][0]; });
    activePreset = 'dark';
    saveVars(currentVars);
    if (typeof TNSTheme !== 'undefined') TNSTheme.set('dark');
    updateColorInputs();
    updateFontInputs();
    renderPreview();
    renderPresets();
    toast('Reset to dark defaults', true);
  }

  function clearColors() {
    try { localStorage.removeItem(CUSTOM_KEY); } catch(e) {}
    if (typeof TNSTheme !== 'undefined') TNSTheme.set('dark');
    currentVars = getVars();
    activePreset = detectActivePreset();
    updateColorInputs();
    updateFontInputs();
    applyVars(currentVars);
    renderPreview();
    renderPresets();
    toast('Cleared custom theme', true);
  }

  /* ── Toast ── */

  function toast(msg, ok) {
    var el = document.createElement('div');
    el.className = 'toast ' + (ok ? 'toast--ok' : 'toast--err');
    el.textContent = msg;
    document.body.appendChild(el);
    setTimeout(function() { el.remove(); }, 2500);
  }

  /* ── Init ── */

  function init() {
    currentVars = getVars();
    activePreset = detectActivePreset();
    renderPresets();
    renderColorControls();
    renderFontControls();
    applyVars(currentVars);
    renderPreview();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }

  window.importTheme = importTheme;
  window.exportTheme = exportTheme;
  window.applyTheme = applyTheme;
  window.resetColors = resetColors;
  window.clearColors = clearColors;
  window.onImportFile = onImportFile;
})();
