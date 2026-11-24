
// === Full Gameplay Mouse & Keyboard Controller (5-Byte DataChannel) ===
(() => {
  const videoEl = document.getElementById("streamVideo");
  if (!videoEl || typeof pc === "undefined") return;

  videoEl.tabIndex = 0;

  const inputChannel =
    window.__cgInputChannel ||
    pc.createDataChannel("input", { ordered: false, maxRetransmits: 0 });
  inputChannel.binaryType = "arraybuffer";
  window.__cgInputChannel = inputChannel;

  const CODE_TO_VK = {
    KeyA: 0x41, KeyB: 0x42, KeyC: 0x43, KeyD: 0x44, KeyE: 0x45, KeyF: 0x46,
    KeyG: 0x47, KeyH: 0x48, KeyI: 0x49, KeyJ: 0x4a, KeyK: 0x4b, KeyL: 0x4c,
    KeyM: 0x4d, KeyN: 0x4e, KeyO: 0x4f, KeyP: 0x50, KeyQ: 0x51, KeyR: 0x52,
    KeyS: 0x53, KeyT: 0x54, KeyU: 0x55, KeyV: 0x56, KeyW: 0x57, KeyX: 0x58,
    KeyY: 0x59, KeyZ: 0x5a,
    Digit0: 0x30, Digit1: 0x31, Digit2: 0x32, Digit3: 0x33, Digit4: 0x34,
    Digit5: 0x35, Digit6: 0x36, Digit7: 0x37, Digit8: 0x38, Digit9: 0x39,
    Space: 0x20, Enter: 0x0d, NumpadEnter: 0x0d, Backspace: 0x08, Tab: 0x09,
    Escape: 0x1b, ShiftLeft: 0xa0, ShiftRight: 0xa1, ControlLeft: 0xa2,
    ControlRight: 0xa3, AltLeft: 0xa4, AltRight: 0xa5, CapsLock: 0x14,
    ArrowLeft: 0x25, ArrowUp: 0x26, ArrowRight: 0x27, ArrowDown: 0x28,
    Insert: 0x2d, Delete: 0x2e, Home: 0x24, End: 0x23,
    PageUp: 0x21, PageDown: 0x22,
    F1: 0x70, F2: 0x71, F3: 0x72, F4: 0x73, F5: 0x74, F6: 0x75,
    F7: 0x76, F8: 0x77, F9: 0x78, F10: 0x79, F11: 0x7a, F12: 0x7b,
    Minus: 0xbd, Equal: 0xbb, BracketLeft: 0xdb, BracketRight: 0xdd,
    Backslash: 0xdc, Semicolon: 0xba, Quote: 0xde, Comma: 0xbc,
    Period: 0xbe, Slash: 0xbf, Backquote: 0xc0,
  };

  function clampInt8(v) {
    return Math.max(-127, Math.min(127, Math.round(v || 0)));
  }

  function sendInputPacket(type, code, flags, dx = 0, dy = 0) {
    if (!inputChannel || inputChannel.readyState !== "open") return;
    const buf = new ArrayBuffer(5);
    const view = new DataView(buf);
    view.setUint8(0, type & 0xff);
    view.setUint8(1, code & 0xff);
    view.setUint8(2, flags & 0xff);
    view.setInt8(3, clampInt8(dx));
    view.setInt8(4, clampInt8(dy));
    inputChannel.send(buf);
  }

  function lockPointerIfNeeded() {
    if (document.pointerLockElement !== videoEl && videoEl.requestPointerLock) {
      try {
        videoEl.requestPointerLock();
      } catch (_) {}
    }
    videoEl.focus({ preventScroll: true });
  }

  document.addEventListener("fullscreenchange", () => {
    if (document.fullscreenElement) lockPointerIfNeeded();
  });

  videoEl.addEventListener("pointerdown", () => {
    lockPointerIfNeeded();
  });

  document.addEventListener("mousemove", (e) => {
    if (document.pointerLockElement !== videoEl && !document.fullscreenElement) return;
    let remX = e.movementX || 0;
    let remY = e.movementY || 0;
    while (remX !== 0 || remY !== 0) {
      const stepX = clampInt8(remX);
      const stepY = clampInt8(remY);
      sendInputPacket(1, 0, 0, stepX, stepY);
      remX -= stepX;
      remY -= stepY;
    }
  });

  document.addEventListener("mousedown", (e) => {
    if (document.pointerLockElement !== videoEl && document.activeElement !== videoEl) return;
    const btn = e.button === 0 ? 1 : e.button === 2 ? 2 : e.button === 1 ? 3 : 0;
    if (!btn) return;
    e.preventDefault();
    sendInputPacket(2, btn, 1, 0, 0);
  });

  document.addEventListener("mouseup", (e) => {
    const btn = e.button === 0 ? 1 : e.button === 2 ? 2 : e.button === 1 ? 3 : 0;
    if (!btn) return;
    e.preventDefault();
    sendInputPacket(2, btn, 0, 0, 0);
  });

  document.addEventListener("wheel", (e) => {
    if (document.pointerLockElement !== videoEl && document.activeElement !== videoEl) return;
    e.preventDefault();
    const scrollStep = clampInt8(-(e.deltaY || 0) / 100);
    if (scrollStep !== 0) sendInputPacket(4, 0, 0, 0, scrollStep);
  }, { passive: false });

  document.addEventListener("contextmenu", (e) => {
    if (document.pointerLockElement === videoEl || document.activeElement === videoEl) {
      e.preventDefault();
    }
  });

  window.addEventListener("keydown", (e) => {
    if (document.pointerLockElement !== videoEl && document.activeElement !== videoEl) return;
    const vk = CODE_TO_VK[e.code] ?? e.keyCode;
    if (!vk) return;
    e.preventDefault();
    sendInputPacket(3, vk, 1, 0, 0);
  }, { passive: false });

  window.addEventListener("keyup", (e) => {
    const vk = CODE_TO_VK[e.code] ?? e.keyCode;
    if (!vk) return;
    e.preventDefault();
    sendInputPacket(3, vk, 0, 0, 0);
  }, { passive: false });

  window.addEventListener("blur", () => {
    if (!inputChannel || inputChannel.readyState !== "open") return;
    for (const vk of Object.values(CODE_TO_VK)) {
      sendInputPacket(3, vk, 0, 0, 0);
    }
  });
})();
