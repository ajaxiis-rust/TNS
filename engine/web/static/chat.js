"use strict";

// Чат (T-08-15): реплика уходит в конвейер POST /api/turn, ответ мира приходит
// порциями по WS-топику turn.stream, панель мир/герой/система читается из
// GET /api/state, сигналы сцены — лайк/дизлайк. Кнопку BOOK создаёт app.js
// (T-07-10); сообщения вставляются перед нижней строкой действий листа.
(function () {
  var TURN_TOPIC = "turn.stream";
  var THUMB_UP = "M7 10v10H4V10h3Zm0 0 4-7c1.4 0 2.2 1 2 2.4L12.5 9H19a2 2 0 0 1 2 2.3l-1 6A2 2 0 0 1 18 19H7";
  var THUMB_DOWN = "M17 14V4h3v10h-3Zm0 0-4 7c-1.4 0-2.2-1-2-2.4l.5-3.6H5a2 2 0 0 1-2-2.3l1-6A2 2 0 0 1 6 5h11";

  function byId(id) {
    return document.getElementById(id);
  }

  var chatLog = byId("chat-log");
  var chatForm = byId("chat-form");
  var chatInput = byId("chat-input");
  var chatSend = byId("chat-send");
  var pending = false;
  var pageActions = null;

  // turn_id ходов, уже показанных из состояния (канонический рендер).
  var committedTurns = {};

  function activeSlug() {
    return window.__activeSlug || "";
  }

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

  function pageActionsEl() {
    if (!pageActions || !pageActions.parentNode) {
      pageActions = document.createElement("div");
      pageActions.className = "page-actions";
      pageActions.hidden = true;
      chatLog.appendChild(pageActions);
    }
    return pageActions;
  }

  function insertIntoLog(node) {
    var actions = pageActionsEl();
    if (actions && actions.parentNode === chatLog) {
      chatLog.insertBefore(node, actions);
    } else {
      chatLog.appendChild(node);
    }
    chatLog.scrollTop = chatLog.scrollHeight;
  }

  function signalButton(sceneID, kind, label) {
    var btn = document.createElement("button");
    btn.type = "button";
    btn.innerHTML = '<svg class="ic" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="' +
      (kind === "like" ? THUMB_UP : THUMB_DOWN) + '"/></svg>';
    btn.appendChild(document.createTextNode(label));
    btn.addEventListener("click", function () {
      postJSON("/api/scene/" + sceneID + "/signal", { kind: kind }).catch(function () {});
    });
    return btn;
  }

  function renderPageActions(sceneID) {
    var box = pageActionsEl();
    box.textContent = "";
    if (!sceneID) {
      box.hidden = true;
      return;
    }
    var dict = window.I18N || {};
    box.appendChild(signalButton(sceneID, "like", dict["chat.like"] || "Понравилось"));
    box.appendChild(signalButton(sceneID, "dislike", dict["chat.dislike"] || "Мимо"));
    box.hidden = false;
    chatLog.scrollTop = chatLog.scrollHeight;
  }

  function appendMessage(role, text) {
    var dict = window.I18N || {};
    var msg = document.createElement("div");
    msg.className = "chat-msg chat-msg--" + role;
    var body = document.createElement("div");
    body.className = "chat-msg__text";
    body.textContent = role === "player" ? (text || dict["chat.you"] || "Ты") : text;
    msg.appendChild(body);
    insertIntoLog(msg);
  }

  function renderReturn(summary) {
    var box = byId("chat-return");
    if (!summary) {
      if (box) {
        box.hidden = true;
      }
      return;
    }
    if (!box) {
      box = document.createElement("div");
      box.className = "chat-return";
      box.id = "chat-return";
      chatLog.insertBefore(box, chatLog.firstChild);
    }
    box.textContent = summary;
    box.hidden = false;
  }

  function renderState(data) {
    var dict = window.I18N || {};

    var world = byId("world-panel");
    if (world) {
      var ws = data.world_state || {};
      var label = ws.title || data.world || "";
      if (ws.travel_mode) {
        label = label + " · " + ws.travel_mode;
      }
      world.textContent = label;
    }

    var hero = byId("hero-panel");
    if (hero) {
      var h = data.hero || {};
      var parts = [];
      if (h.place_id) {
        parts.push((dict["chat.place"] || "место") + " " + h.place_id);
      }
      if (h.mood) {
        parts.push(h.mood);
      }
      if (h.status) {
        parts.push(h.status);
      }
      hero.textContent = parts.join(" · ");
    }

    var system = byId("system-panel");
    if (system) {
      var s = data.system || {};
      var text = "";
      if (s.session_id) {
        text = (dict["chat.session"] || "сессия") + " " + s.session_id;
      }
      system.textContent = text;
    }

    rebuildLog(data);
  }

  // Канонический рендер журнала: состояние перечитывается после хода и при
  // возврате; сводка рисуется до первой реплики, действия сцены — внизу листа.
  function rebuildLog(data) {
    committedTurns = {};
    var nodes = chatLog.querySelectorAll(".chat-msg, .chat-return");
    nodes.forEach(function (node) {
      node.parentNode.removeChild(node);
    });

    renderReturn(data.return_summary || "");

    var lastSceneID = "";
    var turns = data.turns || [];
    turns.forEach(function (t) {
      committedTurns[t.turn_id] = true;
      appendMessage("player", t.player_text);
      appendMessage("world", t.reply_text);
      if (t.scene_id) {
        lastSceneID = t.scene_id;
      }
    });

    renderPageActions(lastSceneID);
  }

  // Живая доставка: порции WS дорисовывают пузырь ответа, пока ход ещё идёт.
  function onChunk(payload) {
    var turnID = payload.turn_id;
    if (committedTurns[turnID]) {
      return;
    }
    var msg = chatLog.querySelector('[data-turn-id="' + turnID + '"]');
    if (!msg) {
      msg = document.createElement("div");
      msg.className = "chat-msg chat-msg--world";
      msg.setAttribute("data-turn-id", String(turnID));
      var body = document.createElement("div");
      body.className = "chat-msg__text";
      msg.appendChild(body);
      insertIntoLog(msg);
    }
    var text = msg.querySelector(".chat-msg__text");
    text.textContent += (payload.text || "");
    chatLog.scrollTop = chatLog.scrollHeight;
  }

  function subscribeStream() {
    if (!window.WebSocket) {
      return;
    }
    var proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    var ws = new WebSocket(proto + "//" + window.location.host + "/ws");
    ws.onopen = function () {
      ws.send(JSON.stringify({ type: "subscribe", topics: [TURN_TOPIC] }));
    };
    ws.onmessage = function (event) {
      var msg = null;
      try {
        msg = JSON.parse(event.data);
      } catch (err) {
        return;
      }
      if (msg && msg.topic === TURN_TOPIC && msg.event_type === "chunk" && msg.payload) {
        onChunk(msg.payload);
      }
    };
    ws.onclose = function () {
      // Обрыв WS не теряет ход: переподключение подтянет состояние из /api/state.
      setTimeout(subscribeStream, 1000);
    };
  }

  function setPending(value) {
    pending = value;
    if (chatInput) {
      chatInput.disabled = value;
    }
    if (chatSend) {
      chatSend.disabled = value;
    }
  }

  function submitTurn(event) {
    event.preventDefault();
    if (pending) {
      return;
    }
    var text = chatInput ? chatInput.value.trim() : "";
    if (!text) {
      return;
    }
    if (chatInput) {
      chatInput.value = "";
    }
    setPending(true);
    appendMessage("player", text);
    postJSON("/api/turn", { world: activeSlug(), text: text }).then(function () {
      setPending(false);
      refresh();
    }).catch(function (err) {
      setPending(false);
      appendMessage("system", err.message);
    });
  }

  function refresh() {
    var slug = activeSlug();
    if (!slug) {
      return;
    }
    apiJSON("/api/state?world=" + encodeURIComponent(slug)).then(renderState).catch(function () {});
  }

  function init() {
    var dict = window.I18N || {};
    if (chatInput) {
      chatInput.placeholder = dict["chat.placeholder"] || "Твои слова — часть истории";
    }
    if (chatSend) {
      chatSend.setAttribute("aria-label", dict["chat.send"] || "Отправить");
    }
    if (chatForm) {
      chatForm.addEventListener("submit", submitTurn);
    }
    subscribeStream();
    refresh();
  }

  window.Chat = { refresh: refresh };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
