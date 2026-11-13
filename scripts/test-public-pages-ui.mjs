// Contrast and language guards for the public pages (landing, sign-in, pricing).
// Text pairs must reach 4.5:1 in both themes; input borders and the focus ring 3:1.
// Each text appears once per language and the other is hidden, so English and Burmese are never shown side by side.
import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

const css = readFileSync(new URL('../web/assets/css/landing.css', import.meta.url), 'utf8');

function tokens(selector) {
  const out = {};
  const start = css.indexOf(selector + ' {');
  assert.ok(start >= 0, `missing token block ${selector}`);
  const body = css.slice(css.indexOf('{', start) + 1, css.indexOf('}', start));
  for (const m of body.matchAll(/--([a-z-]+):\s*(#[0-9a-f]{6})\b/g)) out[m[1]] = m[2];
  return out;
}
const lum = (hex) => {
  const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
};
const ratio = (a, b) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + 0.05) / (y + 0.05); };

const themes = {
  night: tokens(':root, :root[data-theme="night"]'),
  day: tokens(':root[data-theme="day"]'),
};
const textPairs = [
  ['ink', 'bg'], ['ink', 'surface'], ['ink-soft', 'surface'], ['ink-soft', 'sunken'], ['ink-mute', 'bg'], ['ink-mute', 'surface'], ['ink-mute', 'sunken'],
  ['on-accent', 'accent'], ['on-accent', 'accent-hover'], ['accent-text', 'surface'], ['accent-text', 'bg'],
  ['ok', 'surface'], ['warn', 'surface'], ['bad', 'surface'], ['ok', 'bg'],
];
const nonTextPairs = [['control-edge', 'sunken'], ['control-edge', 'surface'], ['accent', 'bg'], ['accent', 'surface']];

for (const [name, t] of Object.entries(themes)) {
  test(`${name}: text colour pairs reach 4.5:1`, () => {
    for (const [fg, bg] of textPairs) assert.ok(ratio(t[fg], t[bg]) >= 4.5, `${fg} ${t[fg]} on ${bg} ${t[bg]} is ${ratio(t[fg], t[bg]).toFixed(2)}`);
  });
  test(`${name}: input borders and focus ring reach 3:1`, () => {
    for (const [fg, bg] of nonTextPairs) assert.ok(ratio(t[fg], t[bg]) >= 3, `${fg} on ${bg} is ${ratio(t[fg], t[bg]).toFixed(2)}`);
  });
}

test('Burmese keeps 1.75 line height and the inactive language is hidden', () => {
  assert.match(css, /--lh-my:\s*1\.75/);
  assert.match(css, /\[data-lang="my"\] body[^{]*\{\s*line-height:\s*var\(--lh-my\)/);
  assert.match(css, /\[data-lang="en"\] \[lang="my"\],\s*\[data-lang="my"\] \[lang="en"\]\s*\{\s*display:\s*none/);
});

test('no page puts both languages in one text node', () => {
  for (const f of ['index.html', 'signin.html', 'pricing.html']) {
    const html = readFileSync(new URL('../web/' + f, import.meta.url), 'utf8');
    assert.doesNotMatch(html, /\p{Script=Latin}{2,}\p{Script=Myanmar}/u, `${f}: Latin text runs straight into Burmese`);
  }
});
