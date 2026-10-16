/**
 * Heartbeat UI Component
 *
 * Displays real-time progress updates from the server.
 * Shows a progress bar and status message during processing.
 */

class HeartbeatUI {
  constructor() {
    this.bar = null;
    this.messageEl = null;
    this.progressEl = null;
    this.isVisible = false;
    this.init();
  }

  init() {
    // Create heartbeat bar element if it doesn't exist
    this.bar = document.getElementById('heartbeat-bar');
    if (!this.bar) {
      this.bar = document.createElement('div');
      this.bar.id = 'heartbeat-bar';
      this.bar.className = 'heartbeat-bar hidden';

      this.progressEl = document.createElement('div');
      this.progressEl.className = 'heartbeat-progress';

      this.messageEl = document.createElement('span');
      this.messageEl.className = 'heartbeat-message';

      this.bar.appendChild(this.progressEl);
      this.bar.appendChild(this.messageEl);

      // Insert before chat messages
      const chatContainer = document.getElementById('chat-messages') || document.querySelector('.chat-container');
      if (chatContainer) {
        chatContainer.parentNode?.insertBefore(this.bar, chatContainer);
      } else {
        document.body.appendChild(this.bar);
      }
    } else {
      this.progressEl = this.bar.querySelector('.heartbeat-progress');
      this.messageEl = this.bar.querySelector('.heartbeat-message');
    }

    // Add styles if not present
    this.addStyles();
  }

  addStyles() {
    if (document.getElementById('heartbeat-styles')) return;

    const style = document.createElement('style');
    style.id = 'heartbeat-styles';
    style.textContent = `
      .heartbeat-bar {
        position: fixed;
        top: 0;
        left: 0;
        right: 0;
        height: 4px;
        background: rgba(0, 0, 0, 0.1);
        z-index: 1000;
        transition: opacity 0.3s ease;
      }

      .heartbeat-bar.hidden {
        opacity: 0;
        pointer-events: none;
      }

      .heartbeat-bar.visible {
        opacity: 1;
      }

      .heartbeat-progress {
        height: 100%;
        background: linear-gradient(90deg, #4a9eff, #00ff88);
        transition: width 0.3s ease;
        width: 0%;
        box-shadow: 0 0 10px rgba(74, 158, 255, 0.5);
      }

      .heartbeat-message {
        position: absolute;
        top: 8px;
        right: 16px;
        font-size: 12px;
        color: #666;
        font-family: 'JetBrains Mono', monospace;
      }

      /* Dark theme support */
      @media (prefers-color-scheme: dark) {
        .heartbeat-bar {
          background: rgba(255, 255, 255, 0.1);
        }

        .heartbeat-message {
          color: #999;
        }
      }
    `;
    document.head.appendChild(style);
  }

  /**
   * Show the heartbeat bar with a message.
   */
  show(message, progress = 0) {
    if (!this.bar) return;

    this.bar.classList.remove('hidden');
    this.bar.classList.add('visible');
    this.isVisible = true;

    if (this.messageEl) {
      this.messageEl.textContent = message;
    }
    if (this.progressEl) {
      this.progressEl.style.width = `${Math.min(100, Math.max(0, progress * 100))}%`;
    }
  }

  /**
   * Update the heartbeat bar.
   */
  update(message, progress) {
    if (!this.isVisible) {
      this.show(message, progress);
      return;
    }

    if (this.messageEl) {
      this.messageEl.textContent = message;
    }
    if (this.progressEl) {
      this.progressEl.style.width = `${Math.min(100, Math.max(0, progress * 100))}%`;
    }
  }

  /**
   * Hide the heartbeat bar.
   */
  hide() {
    if (!this.bar) return;

    this.bar.classList.remove('visible');
    this.bar.classList.add('hidden');
    this.isVisible = false;

    // Reset progress
    if (this.progressEl) {
      this.progressEl.style.width = '0%';
    }
  }

  /**
   * Handle a heartbeat message from WebSocket.
   */
  handleMessage(data) {
    if (data.type === 'heartbeat') {
      this.update(data.message, data.progress);
    } else if (data.type === 'narrative' || data.type === 'done') {
      // Processing complete
      setTimeout(() => this.hide(), 500);
    }
  }
}

// Export for use in main script
window.HeartbeatUI = HeartbeatUI;
