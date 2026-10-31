// Click feel: a native-feeling haptic tick on press.
// iPhone (iOS 18+ Safari): toggling a hidden <input switch> fires the system haptic.
// Android: navigator.vibrate. Nothing plays if the device has no haptics.
const KEY = "mm2_click";
let label = null;

export function clickOn() {
  try { return localStorage.getItem(KEY) !== "off"; } catch (_) { return true; }
}
export function setClick(on) {
  try { localStorage.setItem(KEY, on ? "on" : "off"); } catch (_) { /* private mode */ }
}

function ensureSwitch() {
  if (label) return label;
  label = document.createElement("label");
  label.setAttribute("aria-hidden", "true");
  label.style.cssText = "position:fixed;left:0;top:0;width:1px;height:1px;opacity:0;pointer-events:none;overflow:hidden";
  const input = document.createElement("input");
  input.type = "checkbox";
  input.setAttribute("switch", "");
  input.tabIndex = -1;
  label.append(input);
  document.body.append(label);
  return label;
}

function tick() {
  if (navigator.vibrate) { navigator.vibrate(8); return; }
  try { ensureSwitch().click(); } catch (_) { /* no haptics available */ }
}

export function installClickFeel() {
  document.addEventListener("pointerdown", (e) => {
    if (!clickOn() || e.button > 0) return;
    const el = e.target.closest && e.target.closest("button,.pill,.btn,.tab,.lang button,a.pill");
    if (!el || el.disabled || el.getAttribute("aria-disabled") === "true") return;
    tick();
  }, { passive: true });
}
