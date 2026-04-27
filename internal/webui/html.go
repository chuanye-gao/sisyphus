package webui

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Sisyphus</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #fbf7eb;
      --surface: #fffdf7;
      --surface-soft: #f4ecdc;
      --ink: #121923;
      --muted: #70695f;
      --line: rgba(18, 25, 35, 0.11);
      --gold: #c8902e;
      --gold-soft: #efd8a3;
      --sand: #d6c1a1;
      --bronze: #9a6b37;
      --navy: #132333;
      --navy-soft: #203448;
      --danger: #a33a2f;
      --shadow: 0 18px 50px rgba(19, 35, 51, 0.12);
    }

    * {
      box-sizing: border-box;
    }

    html,
    body {
      height: 100%;
    }

    body {
      margin: 0;
      background:
        radial-gradient(circle at top left, rgba(200, 144, 46, 0.11), transparent 30%),
        linear-gradient(180deg, #fffdf7 0%, var(--bg) 100%);
      color: var(--ink);
      font: 14px/1.5 ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }

    button,
    textarea {
      font: inherit;
    }

    button {
      border: 0;
      cursor: pointer;
    }

    button:disabled {
      cursor: default;
      opacity: 0.52;
    }

    .app {
      height: 100%;
      display: grid;
      grid-template-columns: 268px minmax(0, 1fr);
      background: rgba(255, 253, 247, 0.7);
    }

    .sidebar {
      min-height: 0;
      display: flex;
      flex-direction: column;
      border-right: 1px solid var(--line);
      background:
        linear-gradient(180deg, rgba(19, 35, 51, 0.97), rgba(24, 38, 51, 0.94)),
        var(--navy);
      color: #fffaf0;
      padding: 18px;
    }

    .brand {
      display: flex;
      align-items: center;
      gap: 12px;
      min-width: 0;
      margin-bottom: 22px;
    }

    .emblem {
      width: 42px;
      height: 42px;
      flex: 0 0 auto;
      border-radius: 50%;
      background: #fffaf0;
      box-shadow: 0 8px 20px rgba(0, 0, 0, 0.28);
      overflow: hidden;
    }

    .emblem svg {
      display: block;
      width: 100%;
      height: 100%;
    }

    img.emblem {
      object-fit: cover;
    }

    .brand-text {
      min-width: 0;
    }

    .brand-name {
      font-size: 17px;
      font-weight: 750;
      letter-spacing: 0;
    }

    .brand-model {
      color: rgba(255, 250, 240, 0.68);
      font-size: 12px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .new-chat {
      height: 42px;
      border-radius: 12px;
      background: rgba(255, 250, 240, 0.08);
      color: #fffaf0;
      border: 1px solid rgba(255, 250, 240, 0.14);
      text-align: left;
      padding: 0 14px;
    }

    .new-chat:hover {
      background: rgba(255, 250, 240, 0.13);
    }

    .session-panel {
      min-height: 0;
      flex: 1;
      display: grid;
      grid-template-rows: auto minmax(0, 1fr);
      gap: 10px;
      margin-top: 18px;
    }

    .side-title {
      margin: 0;
      color: rgba(255, 250, 240, 0.52);
      font-size: 11px;
      font-weight: 800;
      text-transform: uppercase;
    }

    .session-list {
      min-height: 0;
      overflow: auto;
      display: grid;
      align-content: start;
      gap: 6px;
      padding-right: 4px;
      scrollbar-gutter: stable;
      scrollbar-width: none;
    }

    .session-list:hover,
    .session-list:focus-within {
      scrollbar-width: thin;
      scrollbar-color: rgba(200, 144, 46, 0.55) rgba(255, 250, 240, 0.05);
    }

    .session-list::-webkit-scrollbar {
      width: 0;
    }

    .session-list:hover::-webkit-scrollbar,
    .session-list:focus-within::-webkit-scrollbar {
      width: 8px;
    }

    .session-list::-webkit-scrollbar-track {
      background: rgba(255, 250, 240, 0.04);
      border-radius: 999px;
    }

    .session-list::-webkit-scrollbar-thumb {
      background: rgba(200, 144, 46, 0.48);
      border-radius: 999px;
      border: 2px solid rgba(19, 35, 51, 0.94);
    }

    .session-item {
      width: 100%;
      min-height: 56px;
      display: grid;
      grid-template-rows: auto auto;
      gap: 4px;
      padding: 10px 12px;
      border-radius: 10px;
      color: rgba(255, 250, 240, 0.82);
      background: transparent;
      text-align: left;
    }

    .session-item:hover {
      background: rgba(255, 250, 240, 0.09);
    }

    .session-item.current {
      background: rgba(200, 144, 46, 0.22);
      color: #fffaf0;
    }

    .session-title {
      overflow: hidden;
      display: -webkit-box;
      -webkit-line-clamp: 2;
      -webkit-box-orient: vertical;
      word-break: break-word;
      font-size: 13px;
      font-weight: 650;
      line-height: 1.35;
    }

    .session-meta {
      color: rgba(255, 250, 240, 0.5);
      font-size: 11px;
      line-height: 1.2;
    }

    .session-empty {
      color: rgba(255, 250, 240, 0.48);
      font-size: 12px;
      padding: 8px 2px;
    }

    .side-spacer {
      height: 14px;
      flex: 0 0 auto;
    }

    .side-meta {
      display: grid;
      gap: 8px;
      color: rgba(255, 250, 240, 0.62);
      font-size: 12px;
    }

    .side-meta span {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .main {
      min-width: 0;
      min-height: 0;
      display: grid;
      grid-template-rows: 58px minmax(0, 1fr) auto;
      position: relative;
    }

    .topbar {
      display: flex;
      align-items: center;
      padding: 0 24px;
      border-bottom: 1px solid var(--line);
      background: rgba(255, 253, 247, 0.76);
      backdrop-filter: blur(18px);
    }

    .top-title {
      font-weight: 700;
      color: var(--navy);
    }

    .status {
      margin-left: auto;
      display: flex;
      align-items: center;
      gap: 8px;
      color: var(--muted);
      font-size: 13px;
    }

    .status::before {
      content: "";
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: var(--gold);
      box-shadow: 0 0 0 5px rgba(200, 144, 46, 0.14);
    }

    .conversation {
      min-height: 0;
      overflow: auto;
      padding: 30px 24px 26px;
      scrollbar-width: thin;
    }

    .conversation-inner {
      width: min(860px, 100%);
      margin: 0 auto;
    }

    .welcome {
      min-height: 46vh;
      display: grid;
      place-items: center;
      text-align: center;
      color: var(--navy);
    }

    .welcome-card {
      display: grid;
      justify-items: center;
      gap: 14px;
    }

    .welcome-logo {
      width: 96px;
      height: 96px;
      box-shadow: 0 16px 46px rgba(19, 35, 51, 0.18);
    }

    .welcome h1 {
      margin: 0;
      font-size: 30px;
      line-height: 1.15;
      letter-spacing: 0;
    }

    .welcome p {
      max-width: 520px;
      margin: 0;
      color: var(--muted);
      font-size: 15px;
    }

    .message {
      display: grid;
      grid-template-columns: 34px minmax(0, 1fr);
      gap: 13px;
      margin-bottom: 24px;
    }

    .avatar {
      width: 34px;
      height: 34px;
      border-radius: 50%;
      display: grid;
      place-items: center;
      font-size: 12px;
      font-weight: 800;
      color: #fffaf0;
      background: var(--navy);
      overflow: hidden;
    }

    .message.user .avatar {
      background: var(--bronze);
    }

    .avatar .emblem {
      width: 100%;
      height: 100%;
      box-shadow: none;
    }

    .bubble {
      min-width: 0;
      max-width: 100%;
      padding: 2px 0 0;
    }

    .label {
      margin-bottom: 5px;
      color: var(--navy);
      font-weight: 700;
      font-size: 13px;
    }

    .message.user .content {
      display: inline-block;
      max-width: min(690px, 100%);
      padding: 12px 15px;
      border-radius: 18px;
      background: #f2e5cd;
      border: 1px solid rgba(154, 107, 55, 0.18);
    }

    .content {
      white-space: pre-wrap;
      word-break: break-word;
      color: var(--ink);
      font-size: 15px;
      line-height: 1.64;
    }

    .trace {
      width: fit-content;
      max-width: 100%;
      margin: -8px 0 16px 47px;
      padding: 7px 11px;
      border-radius: 999px;
      background: rgba(239, 216, 163, 0.28);
      border: 1px solid rgba(200, 144, 46, 0.18);
      color: var(--muted);
      font-size: 12px;
    }

    .trace.warn {
      color: var(--bronze);
    }

    .trace.error {
      color: var(--danger);
    }

    details.trace {
      border-radius: 12px;
      padding: 8px 11px;
    }

    details.trace pre {
      max-height: 190px;
      overflow: auto;
      margin: 8px 0 0;
      white-space: pre-wrap;
      word-break: break-word;
      color: var(--ink);
      font: 12px/1.5 ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace;
    }

    .composer-wrap {
      padding: 12px 24px 22px;
      background: linear-gradient(180deg, rgba(251, 247, 235, 0), rgba(251, 247, 235, 0.96) 28%, rgba(251, 247, 235, 1));
    }

    .composer {
      width: min(860px, 100%);
      margin: 0 auto;
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      gap: 10px;
      align-items: end;
      padding: 10px 10px 10px 16px;
      border: 1px solid rgba(19, 35, 51, 0.14);
      border-radius: 24px;
      background: rgba(255, 253, 247, 0.94);
      box-shadow: var(--shadow);
    }

    textarea {
      width: 100%;
      min-height: 28px;
      max-height: 220px;
      resize: none;
      border: 0;
      outline: 0;
      padding: 8px 0;
      background: transparent;
      color: var(--ink);
      line-height: 1.5;
    }

    textarea::placeholder {
      color: rgba(112, 105, 95, 0.78);
    }

    .actions {
      display: flex;
      align-items: center;
      gap: 8px;
    }

    .icon-button,
    .send-button {
      width: 38px;
      height: 38px;
      border-radius: 50%;
      display: grid;
      place-items: center;
      font-weight: 800;
    }

    .icon-button {
      background: rgba(19, 35, 51, 0.07);
      color: var(--navy);
    }

    .icon-button:hover {
      background: rgba(19, 35, 51, 0.11);
    }

    .send-button {
      background: var(--navy);
      color: #fffaf0;
      box-shadow: 0 10px 22px rgba(19, 35, 51, 0.18);
    }

    .send-button:hover {
      background: var(--navy-soft);
    }

    .hidden-state {
      display: none;
    }

    @media (max-width: 860px) {
      .app {
        grid-template-columns: 1fr;
      }

      .sidebar {
        display: none;
      }

      .topbar {
        padding: 0 16px;
      }

      .conversation {
        padding: 22px 14px;
      }

      .composer-wrap {
        padding: 10px 12px 16px;
      }

      .welcome h1 {
        font-size: 25px;
      }
    }
  </style>
</head>
<body>
  <div class="app">
    <aside class="sidebar">
      <div class="brand">
        <img class="emblem" src="/assets/logo.png" alt="">
        <div class="brand-text">
          <div class="brand-name">Sisyphus</div>
          <div class="brand-model">{{.Model}}</div>
        </div>
      </div>
      <button class="new-chat" id="clear" type="button">New chat</button>
      <section class="session-panel">
        <h2 class="side-title">Sessions</h2>
        <div class="session-list" id="sessions"></div>
      </section>
      <div class="side-spacer"></div>
      <div class="side-meta">
        <span id="session-id">{{.SessionID}}</span>
        <span><span id="messages">-</span> messages</span>
        <span><span id="tokens">-</span> tokens</span>
      </div>
    </aside>

    <main class="main">
      <header class="topbar">
        <div class="top-title">Sisyphus</div>
        <div class="status" id="status">Ready</div>
      </header>

      <section class="conversation" id="conversation" aria-live="polite">
        <div class="conversation-inner" id="conversation-inner">
          <div class="welcome" id="welcome">
            <div class="welcome-card">
              <img class="emblem welcome-logo" src="/assets/logo.png" alt="">
              <h1>What shall we push forward?</h1>
              <p>Give Sisyphus a task, a code question, or a long piece of context.</p>
            </div>
          </div>
        </div>
      </section>

      <div class="composer-wrap">
        <form class="composer" id="composer">
          <textarea id="input" rows="1" placeholder="Message Sisyphus"></textarea>
          <div class="actions">
            <button class="icon-button" id="cancel" type="button" disabled title="Stop">x</button>
            <button class="send-button" id="send" type="submit" title="Send">&gt;</button>
          </div>
        </form>
      </div>
    </main>
  </div>

  <script>
    const conversation = document.getElementById("conversation");
    const conversationInner = document.getElementById("conversation-inner");
    let welcome = document.getElementById("welcome");
    const form = document.getElementById("composer");
    const input = document.getElementById("input");
    const sendButton = document.getElementById("send");
    const cancelButton = document.getElementById("cancel");
    const clearButton = document.getElementById("clear");
    const sessionsEl = document.getElementById("sessions");
    const statusEl = document.getElementById("status");
    const sessionEl = document.getElementById("session-id");
    const messagesEl = document.getElementById("messages");
    const tokensEl = document.getElementById("tokens");

    let busy = false;
    let assistantContent = null;
    let currentSessionId = sessionEl.textContent;

    function setBusy(value) {
      busy = value;
      sendButton.disabled = value;
      cancelButton.disabled = !value;
      clearButton.disabled = value;
      input.disabled = value;
    }

    function setStatus(text) {
      statusEl.textContent = text;
    }

    function scrollToBottom() {
      conversation.scrollTop = conversation.scrollHeight;
    }

    function hideWelcome() {
      if (welcome && welcome.parentNode) {
        welcome.parentNode.removeChild(welcome);
      }
    }

    function showWelcome() {
      conversationInner.innerHTML = "";
      const wrapper = document.createElement("div");
      wrapper.className = "welcome";
      wrapper.id = "welcome";
      wrapper.innerHTML = '<div class="welcome-card"><img class="emblem welcome-logo" src="/assets/logo.png" alt=""><h1>What shall we push forward?</h1><p>Give Sisyphus a task, a code question, or a long piece of context.</p></div>';
      conversationInner.appendChild(wrapper);
      welcome = wrapper;
    }

    function autoSizeInput() {
      input.style.height = "auto";
      input.style.height = Math.min(input.scrollHeight, 220) + "px";
    }

    function addTrace(text, className) {
      hideWelcome();
      const line = document.createElement("div");
      line.className = "trace" + (className ? " " + className : "");
      line.textContent = text;
      conversationInner.appendChild(line);
      scrollToBottom();
      return line;
    }

    function addToolResult(name, result, elapsed) {
      hideWelcome();
      const details = document.createElement("details");
      details.className = "trace";
      const summary = document.createElement("summary");
      summary.textContent = name + " completed in " + elapsed + " ms";
      const pre = document.createElement("pre");
      pre.textContent = result || "";
      details.appendChild(summary);
      details.appendChild(pre);
      conversationInner.appendChild(details);
      scrollToBottom();
    }

    function addMessage(role, text) {
      hideWelcome();
      const item = document.createElement("article");
      item.className = "message " + role;

      const avatar = document.createElement("div");
      avatar.className = "avatar";
      if (role === "assistant") {
        const img = document.createElement("img");
        img.className = "emblem";
        img.src = "/assets/logo.png";
        img.alt = "";
        avatar.appendChild(img);
      } else {
        avatar.textContent = "U";
      }

      const bubble = document.createElement("div");
      bubble.className = "bubble";
      const label = document.createElement("div");
      label.className = "label";
      label.textContent = role === "assistant" ? "Sisyphus" : "You";
      const content = document.createElement("div");
      content.className = "content";
      content.textContent = text || "";
      bubble.appendChild(label);
      bubble.appendChild(content);
      item.appendChild(avatar);
      item.appendChild(bubble);
      conversationInner.appendChild(item);
      scrollToBottom();
      return content;
    }

    function renderChatMessages(messages) {
      conversationInner.innerHTML = "";
      assistantContent = null;
      if (!messages || messages.length === 0) {
        showWelcome();
        return;
      }
      welcome = null;
      messages.forEach((message) => {
        addMessage(message.role, message.content);
      });
      scrollToBottom();
    }

    function applyState(body) {
      currentSessionId = body.session_id;
      sessionEl.textContent = body.session_id;
      messagesEl.textContent = body.memory.messages;
      tokensEl.textContent = body.memory.tokens;
      setBusy(Boolean(body.busy));
      if (Array.isArray(body.chat_messages)) {
        renderChatMessages(body.chat_messages);
      }
    }

    function ensureAssistant() {
      if (!assistantContent) {
        assistantContent = addMessage("assistant", "");
      }
      return assistantContent;
    }

    function parseEvent(raw) {
      let event = "message";
      const data = [];
      raw.replace(/\r/g, "").split("\n").forEach((line) => {
        if (line.startsWith("event:")) {
          event = line.slice(6).trim();
        } else if (line.startsWith("data:")) {
          data.push(line.slice(5).trimStart());
        }
      });
      let payload = {};
      if (data.length > 0) {
        try {
          payload = JSON.parse(data.join("\n"));
        } catch (err) {
          payload = { message: data.join("\n") };
        }
      }
      handleEvent(event, payload);
    }

    function handleEvent(event, payload) {
      if (event === "session") {
        sessionEl.textContent = payload.session_id || sessionEl.textContent;
      } else if (event === "turn_start") {
        setStatus("Reading");
      } else if (event === "llm_request") {
        setStatus("Thinking");
      } else if (event === "thinking") {
        setStatus("Thinking");
      } else if (event === "content") {
        ensureAssistant().textContent += payload.delta || "";
        scrollToBottom();
      } else if (event === "tool_call") {
        assistantContent = null;
        setStatus("Using " + payload.name);
        addTrace("Using " + payload.name);
      } else if (event === "tool_result") {
        addToolResult(payload.name, payload.result, payload.elapsed_ms);
      } else if (event === "llm_response") {
        setStatus("Answering");
      } else if (event === "checkpoint") {
        if (payload.tokens_total) {
          tokensEl.textContent = payload.tokens_total;
        }
      } else if (event === "done") {
        setStatus("Ready");
        if (payload.session_id) {
          currentSessionId = payload.session_id;
          sessionEl.textContent = payload.session_id;
        }
        if (payload.memory) {
          messagesEl.textContent = payload.memory.messages;
          tokensEl.textContent = payload.memory.tokens;
        }
        loadSessions();
      } else if (event === "cancelled") {
        addTrace(payload.message || "cancelled", "warn");
        setStatus("Ready");
      } else if (event === "error") {
        addTrace(payload.message || "error", "error");
        setStatus("Error");
      }
    }

    async function sendTurn(text) {
      setBusy(true);
      assistantContent = null;
      addMessage("user", text);
      setStatus("Sending");

      const response = await fetch("/api/turn", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ input: text })
      });

      if (!response.ok) {
        const body = await response.json().catch(() => ({ error: response.statusText }));
        throw new Error(body.error || response.statusText);
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const result = await reader.read();
        if (result.done) {
          break;
        }
        buffer += decoder.decode(result.value, { stream: true });
        let index = buffer.indexOf("\n\n");
        while (index >= 0) {
          const raw = buffer.slice(0, index);
          buffer = buffer.slice(index + 2);
          if (raw.trim()) {
            parseEvent(raw);
          }
          index = buffer.indexOf("\n\n");
        }
      }

      if (buffer.trim()) {
        parseEvent(buffer);
      }
    }

    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      const text = input.value.trim();
      if (!text || busy) {
        return;
      }
      input.value = "";
      autoSizeInput();
      try {
        await sendTurn(text);
      } catch (err) {
        addTrace(err.message, "error");
        setStatus("Error");
      } finally {
        setBusy(false);
        loadSessions();
        input.focus();
      }
    });

    input.addEventListener("input", autoSizeInput);

    input.addEventListener("keydown", (event) => {
      if (event.key === "Enter" && !event.shiftKey) {
        event.preventDefault();
        form.requestSubmit();
      }
    });

    cancelButton.addEventListener("click", async () => {
      await fetch("/api/cancel", { method: "POST" });
      setStatus("Stopping");
    });

    clearButton.addEventListener("click", async () => {
      const response = await fetch("/api/clear", { method: "POST" });
      const body = await response.json();
      if (!response.ok) {
        addTrace(body.error || "clear failed", "error");
        return;
      }
      applyState(body);
      setStatus("Ready");
      loadSessions();
    });

    async function loadState() {
      const response = await fetch("/api/state");
      const body = await response.json();
      applyState(body);
    }

    function renderSessions(sessions) {
      sessionsEl.innerHTML = "";
      if (!sessions || sessions.length === 0) {
        const empty = document.createElement("div");
        empty.className = "session-empty";
        empty.textContent = "No saved sessions";
        sessionsEl.appendChild(empty);
        return;
      }
      sessions.forEach((session) => {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "session-item" + (session.current ? " current" : "");
        button.disabled = busy;

        const title = document.createElement("div");
        title.className = "session-title";
        title.textContent = session.title || session.id;

        const meta = document.createElement("div");
        meta.className = "session-meta";
        const count = session.message_count || 0;
        meta.textContent = count + (count === 1 ? " message" : " messages");

        button.appendChild(title);
        button.appendChild(meta);
        button.addEventListener("click", () => loadSession(session.id));
        sessionsEl.appendChild(button);
      });
    }

    async function loadSessions() {
      const response = await fetch("/api/sessions");
      const body = await response.json();
      if (!response.ok) {
        return;
      }
      renderSessions(body.sessions);
    }

    async function loadSession(id) {
      if (busy || id === currentSessionId) {
        return;
      }
      setStatus("Loading");
      const response = await fetch("/api/sessions/load", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id })
      });
      const body = await response.json();
      if (!response.ok) {
        addTrace(body.error || "load failed", "error");
        setStatus("Error");
        return;
      }
      applyState(body);
      setStatus("Ready");
      loadSessions();
    }

    autoSizeInput();
    loadState();
    loadSessions();
    input.focus();
  </script>
</body>
</html>`
