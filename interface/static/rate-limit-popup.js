/**
 * Rate Limit Popup — WebSocket notification handler for rate limit events.
 * Shows a popup when a provider hits its rate limit and falls back to local model.
 */

(function() {
  'use strict';

  const POPUP_KEY = 'tns_rate_limit_dismissed';
  const POPUP_DURATION = 10000; // Auto-dismiss after 10 seconds

  let ws = null;
  let popupContainer = null;
  let popupTimeout = null;

  function escapeHtml(str) {
    if (typeof str !== 'string') return '';
    return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  function init() {
    // Create popup container
    popupContainer = document.createElement('div');
    popupContainer.id = 'rate-limit-popup-container';
    document.body.appendChild(popupContainer);

    // Load CSS
    if (!document.querySelector('link[href*="rate-limit-popup.css"]')) {
      const link = document.createElement('link');
      link.rel = 'stylesheet';
      link.href = '/static/rate-limit-popup.css';
      document.head.appendChild(link);
    }

    // Connect to WebSocket
    connectWebSocket();
  }

  function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}`;

    try {
      ws = tnsStubWebSocket();

      ws.onopen = () => {
        console.log('[RateLimit] WebSocket connected');
      };

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          if (data.type === 'rate_limit_notification') {
            handleRateLimitNotification(data);
          }
        } catch (err) {
          console.error('[RateLimit] Failed to parse message:', err);
        }
      };

      ws.onclose = () => {
        console.log('[RateLimit] WebSocket disconnected, reconnecting in 5s...');
        setTimeout(connectWebSocket, 5000);
      };

      ws.onerror = (err) => {
        console.error('[RateLimit] WebSocket error:', err);
      };
    } catch (err) {
      console.error('[RateLimit] Failed to connect WebSocket:', err);
      setTimeout(connectWebSocket, 5000);
    }
  }

  function handleRateLimitNotification(data) {
    // Check if user dismissed notifications
    if (localStorage.getItem(POPUP_KEY) === 'true') {
      console.log('[RateLimit] Notifications dismissed by user');
      return;
    }

    showPopup(data);
  }

  function showPopup(data) {
    // Clear existing popup
    clearPopup();

    const popup = document.createElement('div');
    popup.className = 'rate-limit-popup';
    popup.innerHTML = `
      <div class="rate-limit-popup-header">
        <span class="rate-limit-popup-icon">⚠️</span>
        <span class="rate-limit-popup-title">${escapeHtml(data.providerId)} rate limit</span>
        <button class="rate-limit-popup-close" onclick="window.RateLimitPopup.close()">×</button>
      </div>
      <div class="rate-limit-popup-body">
        <div class="rate-limit-popup-message">
          Ключ ${escapeHtml(data.key)} достиг лимита запросов.
        </div>
        <div class="rate-limit-popup-details">
          <div class="rate-limit-popup-detail-row">
            <span class="rate-limit-popup-detail-label">Провайдер:</span>
            <span class="rate-limit-popup-detail-value">${escapeHtml(data.providerId)}</span>
          </div>
          <div class="rate-limit-popup-detail-row">
            <span class="rate-limit-popup-detail-label">Ключ:</span>
            <span class="rate-limit-popup-detail-value">${escapeHtml(data.key)}</span>
          </div>
          <div class="rate-limit-popup-detail-row">
            <span class="rate-limit-popup-detail-label">Fallback:</span>
            <span class="rate-limit-popup-detail-value">${escapeHtml(data.fallbackProvider)}/${escapeHtml(data.fallbackModel)}</span>
          </div>
        </div>
        <div class="rate-limit-popup-switch">
          <label class="rate-limit-popup-switch-label">Переключить модель:</label>
          <select class="rate-limit-popup-switch-select" id="rate-limit-model-select">
            <option value="">Выберите модель...</option>
          </select>
        </div>
        <div class="rate-limit-popup-dismiss">
          <input type="checkbox" id="rate-limit-dismiss-checkbox">
          <label for="rate-limit-dismiss-checkbox">Отключить уведомления</label>
        </div>
      </div>
      <div class="rate-limit-popup-footer">
        <button class="rate-limit-popup-btn rate-limit-popup-btn-primary" onclick="window.RateLimitPopup.switchModel()">
          Переключить
        </button>
        <button class="rate-limit-popup-btn rate-limit-popup-btn-secondary" onclick="window.RateLimitPopup.close()">
          Закрыть
        </button>
      </div>
    `;

    popupContainer.appendChild(popup);

    // Load available models
    loadAvailableModels(data.providerId);

    // Auto-dismiss
    popupTimeout = setTimeout(() => {
      closePopup();
    }, POPUP_DURATION);
  }

  function loadAvailableModels(currentProvider) {
    tnsStubFetch()
      .then(res => res.json())
      .then(data => {
        const select = document.getElementById('rate-limit-model-select');
        if (!select) return;

        select.innerHTML = '<option value="">Выберите модель...</option>';

        for (const provider of data.providers || []) {
          const group = document.createElement('optgroup');
          group.label = provider.name || provider.id;

          // We need to fetch models for each provider
          tnsStubFetch()
            .then(res => res.json())
            .then(providerData => {
              for (const model of providerData.models || []) {
                const option = document.createElement('option');
                option.value = `${provider.id}:${model}`;
                option.textContent = `${provider.name || provider.id} / ${model}`;
                if (provider.id === currentProvider) {
                  option.selected = true;
                }
                group.appendChild(option);
              }
              select.appendChild(group);
            })
            .catch(() => {});
        }
      })
      .catch(err => {
        console.error('[RateLimit] Failed to load providers:', err);
      });
  }

  function clearPopup() {
    if (popupTimeout) {
      clearTimeout(popupTimeout);
      popupTimeout = null;
    }
    if (popupContainer) {
      popupContainer.innerHTML = '';
    }
  }

  function closePopup() {
    // Check if user wants to dismiss notifications
    const checkbox = document.getElementById('rate-limit-dismiss-checkbox');
    if (checkbox && checkbox.checked) {
      localStorage.setItem(POPUP_KEY, 'true');
    }

    clearPopup();
  }

  function switchModel() {
    const select = document.getElementById('rate-limit-model-select');
    if (!select || !select.value) {
      alert('Выберите модель');
      return;
    }

    const [providerId, modelId] = select.value.split(':');

    // Find the agent that was using this provider
    // For now, we'll switch the director agent as an example
    tnsStubFetch()
    .then(res => res.json())
    .then(data => {
      if (data.status === 'switched') {
        console.log('[RateLimit] Model switched:', data);
        closePopup();
      } else {
        alert('Ошибка переключения: ' + (data.error || 'Unknown error'));
      }
    })
    .catch(err => {
      console.error('[RateLimit] Failed to switch model:', err);
      alert('Ошибка переключения модели');
    });
  }

  function resetDismissed() {
    localStorage.removeItem(POPUP_KEY);
  }

  // Public API
  window.RateLimitPopup = {
    init,
    close: closePopup,
    switchModel,
    resetDismissed,
  };

  // Auto-init when DOM is ready
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
