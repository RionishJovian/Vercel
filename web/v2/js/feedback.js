// Click feel: a short mechanical "thock" plus a tiny haptic tick on press.
// Sound is synthesized (no audio files) and can be switched off in More.
const KEY = "mm2_click";
let ctx = null;

export function clickOn() {
  try { return localStorage.getItem(KEY) !== "off"; } catch (_) { return true; }
}
export function setClick(on) {
  try { localStorage.setItem(KEY, on ? "on" : "off"); } catch (_) { /* private mode */ }
}

function thock() {
  try {
    const AC = window.AudioContext || window.webkitAudioContext;
    if (!AC) return;
    ctx = ctx || new AC();
    if (ctx.state === "suspended") ctx.resume();
    const t = ctx.currentTime;
    // Low body: quick pitch drop, like a cushioned key bottoming out.
    const o = ctx.createOscillator(), g = ctx.createGain();
    o.type = "sine"; o.frequency.setValueAtTime(190, t); o.frequency.exponentialRampToValueAtTime(80, t + 0.05);
    g.gain.setValueAtTime(0.0001, t); g.gain.exponentialRampToValueAtTime(0.16, t + 0.004); g.gain.exponentialRampToValueAtTime(0.0001, t + 0.07);
    o.connect(g).connect(ctx.destination); o.start(t); o.stop(t + 0.08);
    // Soft click on top: a few ms of filtered noise.
    const n = ctx.createBuffer(1, Math.floor(ctx.sampleRate * 0.02), ctx.sampleRate);
    const d = n.getChannelData(0);
    for (let i = 0; i < d.length; i++) d[i] = (Math.random() * 2 - 1) * (1 - i / d.length);
    const s = ctx.createBufferSource(), f = ctx.createBiquadFilter(), ng = ctx.createGain();
    s.buffer = n; f.type = "bandpass"; f.frequency.value = 2600; f.Q.value = 0.8; ng.gain.value = 0.09;
    s.connect(f).connect(ng).connect(ctx.destination); s.start(t);
  } catch (_) { /* audio blocked: stay silent */ }
}

export function installClickFeel() {
  document.addEventListener("pointerdown", (e) => {
    if (!clickOn() || e.button > 0) return;
    const el = e.target.closest && e.target.closest("button,.pill,.btn,.tab,.lang button,a.pill");
    if (!el || el.disabled || el.getAttribute("aria-disabled") === "true") return;
    thock();
    if (navigator.vibrate) navigator.vibrate(6);
  }, { passive: true });
}
