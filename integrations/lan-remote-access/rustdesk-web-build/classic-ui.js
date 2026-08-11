import './style.css';
import * as globals from './globals';

const app = document.querySelector('#app');

if (app) {
  app.innerHTML = `
    <section id="status" class="center-card" aria-live="polite">
      <h1>正在连接电脑</h1>
      <p id="status-text">正在建立加密视频通道…</p>
      <div class="actions">
        <a class="button" href="/rdp/">返回入口</a>
        <button id="retry" type="button" hidden>重新连接</button>
      </div>
    </section>

    <section id="password-panel" class="center-card" hidden>
      <h1>需要 RustDesk 密码</h1>
      <p>请输入被控电脑上设置的固定密码。密码只用于当前加密会话。</p>
      <form id="password-form" class="password-form">
        <label for="password">RustDesk 密码</label>
        <input id="password" name="password" type="password" autocomplete="current-password" required />
        <div class="actions">
          <a class="button" href="/rdp/">取消</a>
          <button class="primary" type="submit">连接</button>
        </div>
      </form>
    </section>

    <section id="viewer" class="viewer" hidden>
      <canvas id="player" tabindex="0" aria-label="远程桌面画面"></canvas>
      <nav class="toolbar" aria-label="远程控制工具栏">
        <span id="session-state" class="session-state">加密中继</span>
        <button id="fullscreen" type="button">全屏</button>
        <button id="send-text" type="button">发送文本</button>
        <button id="ctrl-alt-del" type="button">Ctrl+Alt+Del</button>
        <a class="button" href="/files/" target="_blank" rel="noopener">传输文件</a>
        <button id="disconnect" type="button">断开</button>
      </nav>
    </section>
  `;

  const status = document.querySelector('#status');
  const statusText = document.querySelector('#status-text');
  const retry = document.querySelector('#retry');
  const passwordPanel = document.querySelector('#password-panel');
  const passwordForm = document.querySelector('#password-form');
  const passwordInput = document.querySelector('#password');
  const viewer = document.querySelector('#viewer');
  const canvas = document.querySelector('#player');
  const sessionState = document.querySelector('#session-state');
  const peerId = new URLSearchParams(location.search).get('id') || app.dataset.peerId;
  let connection;
  let player;
  let pendingMove;
  let moveFrame;
  let firstFrameTimer;

  const prepareVideoDecoder = async (candidate) => {
    const loader = window.OGVLoader?.loadClass?.bind(window.OGVLoader);
    const loadDecoder = candidate.loadVideoDecoder?.bind(candidate);
    if (!loader || !loadDecoder) throw new Error('VP9 解码器不可用。');

    // Scramjet 内的 Worker 无法稳定完成 OGV 初始化，保留同一 WASM 解码器的单线程路径。
    window.OGVLoader.loadClass = (name, callback, options) =>
      loader(
        name,
        callback,
        name === 'OGVDecoderVideoVP9W' ? { ...options, worker: false, threading: false } : options,
      );

    loadDecoder();
    const deadline = performance.now() + 15000;
    while (typeof candidate._videoDecoder?.processFrame !== 'function') {
      if (performance.now() >= deadline) throw new Error('VP9 解码器初始化超时。');
      await new Promise((resolve) => setTimeout(resolve, 25));
    }

    let keepPreparedDecoder = true;
    candidate.loadVideoDecoder = () => {
      if (keepPreparedDecoder) {
        keepPreparedDecoder = false;
        return;
      }
      return loadDecoder();
    };
  };

  localStorage.setItem('key', app.dataset.serverKey);
  localStorage.setItem('rendezvous-server', location.hostname);
  localStorage.setItem('custom-rendezvous-server', location.hostname);
  localStorage.removeItem('access_token');

  window.onGlobalEvent = (raw) => {
    try {
      const event = JSON.parse(raw);
      if (event.name === 'connection_ready') {
        sessionState.textContent = event.secure === 'true' ? '已加密 · 中继' : '中继';
      }
    } catch (error) {
      console.error('RustDesk event decode failed', error);
    }
  };

  const showOnly = (element) => {
    for (const section of [status, passwordPanel, viewer]) section.hidden = section !== element;
  };

  const showStatus = (message, isError = false) => {
    showOnly(status);
    statusText.textContent = message;
    statusText.classList.toggle('error-text', isError);
    retry.hidden = !isError;
  };

  const showPassword = () => {
    showOnly(passwordPanel);
    passwordInput.value = '';
    passwordInput.focus();
  };

  const showViewer = () => {
    showOnly(viewer);
    canvas.focus();
  };

  const messageBox = (type, _title, text) => {
    if (type === 'input-password') {
      clearTimeout(firstFrameTimer);
      return showPassword();
    }
    if (!type) return;
    if (type === 'error') {
      clearTimeout(firstFrameTimer);
      return showStatus(text || '连接失败，请重试。', true);
    }
    if (type === 'connecting') return showStatus('正在验证并载入远程桌面…');
    if (type === 'success') {
      showStatus('连接成功，正在等待第一帧画面…');
      clearTimeout(firstFrameTimer);
      firstFrameTimer = setTimeout(() => {
        showStatus('连接已建立，但 20 秒内未能解码 VP9 首帧。请重新连接后再试。', true);
      }, 20000);
    }
  };

  const connect = async () => {
    if (!peerId) return showStatus('入口没有提供有效的 RustDesk ID。', true);
    clearTimeout(firstFrameTimer);
    showStatus('正在建立加密视频通道…');
    connection = globals.newConn();
    connection.setMsgbox(messageBox);
    const renderFrame = (frame) => {
      clearTimeout(firstFrameTimer);
      const width = frame.format.displayWidth;
      const height = frame.format.displayHeight;
      if (canvas.width !== width || canvas.height !== height) {
        canvas.width = width;
        canvas.height = height;
        canvas.style.aspectRatio = `${width} / ${height}`;
      }
      player.drawFrame(frame);
      sessionState.textContent = `${width} × ${height} · 已加密`;
      showViewer();
    };
    connection.setDraw(renderFrame);
    // 经典 UI 直接消费 YUV 帧，避免再进入仅供 Flutter 使用的 onRgba 桥。
    connection.draw = renderFrame;
    await prepareVideoDecoder(connection);
    await connection.start(peerId.replaceAll(' ', ''));
  };

  const modifiers = (event) => [event.altKey, event.ctrlKey, event.shiftKey, event.metaKey];
  const remotePoint = (event) => {
    const rect = canvas.getBoundingClientRect();
    return [
      Math.max(
        0,
        Math.min(
          canvas.width - 1,
          Math.round(((event.clientX - rect.left) * canvas.width) / rect.width),
        ),
      ),
      Math.max(
        0,
        Math.min(
          canvas.height - 1,
          Math.round(((event.clientY - rect.top) * canvas.height) / rect.height),
        ),
      ),
    ];
  };
  const buttonBits = (button) => ({ 0: 1, 2: 2, 1: 4 })[button] || 0;
  const sendPointer = (type, event) => {
    if (!connection || canvas.width === 0) return;
    const [x, y] = remotePoint(event);
    connection.inputMouse(type | (buttonBits(event.button) << 3), x, y, ...modifiers(event));
  };

  canvas.addEventListener('pointermove', (event) => {
    pendingMove = event;
    if (moveFrame) return;
    moveFrame = requestAnimationFrame(() => {
      const current = pendingMove;
      pendingMove = undefined;
      moveFrame = undefined;
      if (!current || !connection) return;
      const [x, y] = remotePoint(current);
      connection.inputMouse(0, x, y, ...modifiers(current));
    });
  });
  canvas.addEventListener('pointerdown', (event) => {
    event.preventDefault();
    canvas.focus();
    canvas.setPointerCapture(event.pointerId);
    sendPointer(1, event);
  });
  canvas.addEventListener('pointerup', (event) => {
    event.preventDefault();
    sendPointer(2, event);
  });
  canvas.addEventListener('contextmenu', (event) => event.preventDefault());
  canvas.addEventListener(
    'wheel',
    (event) => {
      event.preventDefault();
      connection?.inputMouse(
        3,
        Math.round(event.deltaX),
        Math.round(event.deltaY),
        ...modifiers(event),
      );
    },
    { passive: false },
  );

  const keyNames = {
    Enter: 'VK_RETURN',
    NumpadEnter: 'VK_ENTER',
    Backspace: 'VK_BACK',
    Tab: 'VK_TAB',
    Escape: 'VK_ESCAPE',
    Space: 'VK_SPACE',
    PageUp: 'VK_PRIOR',
    PageDown: 'VK_NEXT',
    End: 'VK_END',
    Home: 'VK_HOME',
    ArrowLeft: 'VK_LEFT',
    ArrowUp: 'VK_UP',
    ArrowRight: 'VK_RIGHT',
    ArrowDown: 'VK_DOWN',
    Insert: 'VK_INSERT',
    Delete: 'VK_DELETE',
    ShiftLeft: 'VK_SHIFT',
    ShiftRight: 'RShift',
    ControlLeft: 'VK_CONTROL',
    ControlRight: 'RControl',
    AltLeft: 'VK_MENU',
    AltRight: 'RAlt',
    MetaLeft: 'Meta',
    MetaRight: 'RWin',
    Comma: 'VK_COMMA',
    Slash: 'VK_SLASH',
    Semicolon: 'VK_SEMICOLON',
    Quote: 'VK_QUOTE',
    BracketLeft: 'VK_LBRACKET',
    BracketRight: 'VK_RBRACKET',
    Backslash: 'VK_BACKSLASH',
    Minus: 'VK_MINUS',
    Equal: 'VK_PLUS',
  };
  const remoteKey = (event) => {
    if (/^Key[A-Z]$/.test(event.code)) return `VK_${event.code.slice(3)}`;
    if (/^Digit[0-9]$/.test(event.code)) return `VK_${event.code.slice(5)}`;
    if (/^F(?:[1-9]|1[0-2])$/.test(event.code)) return `VK_${event.code}`;
    if (/^Numpad[0-9]$/.test(event.code)) return `VK_${event.code.toUpperCase()}`;
    return keyNames[event.code];
  };
  const sendKey = (event, down) => {
    const name = remoteKey(event);
    if (!name || !connection) return;
    event.preventDefault();
    connection.inputKey(name, down, event.repeat, ...modifiers(event));
  };
  canvas.addEventListener('keydown', (event) => sendKey(event, true));
  canvas.addEventListener('keyup', (event) => sendKey(event, false));

  passwordForm.addEventListener('submit', (event) => {
    event.preventDefault();
    const password = passwordInput.value;
    passwordInput.value = '';
    if (password) connection?.login(password);
  });
  retry.addEventListener('click', () => location.reload());
  document.querySelector('#disconnect').addEventListener('click', () => {
    clearTimeout(firstFrameTimer);
    globals.close();
    location.href = '/rdp/';
  });
  document.querySelector('#fullscreen').addEventListener('click', async () => {
    if (document.fullscreenElement) await document.exitFullscreen();
    else await viewer.requestFullscreen();
    canvas.focus();
  });
  document.querySelector('#ctrl-alt-del').addEventListener('click', () => {
    connection?.ctrlAltDel();
    canvas.focus();
  });
  document.querySelector('#send-text').addEventListener('click', () => {
    const text = prompt('输入要发送到远程电脑的文本：');
    if (text) connection?.inputString(text);
    canvas.focus();
  });

  player = YUVCanvas.attach(canvas);
  window
    .init()
    .then(connect)
    .catch((error) => showStatus(String(error), true));
}
