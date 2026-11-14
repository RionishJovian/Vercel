// Console contrast is checked separately from the sign-in's scoped palette.
// Include nested surfaces and translucent status fills, not just bare token pairs.
import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {runInNewContext} from 'node:vm';

const read = (file) => readFileSync(new URL('../app/' + file, import.meta.url), 'utf8');
const css = read('src/styles/tokens.css');
const color = (value) => value.startsWith('#')
  ? [1, 3, 5].map((i) => parseInt(value.slice(i, i + 2), 16)).concat(1)
  : value.match(/[\d.]+/g).map(Number);
const over = (fg, bg) => fg.slice(0, 3).map((v, i) => v * fg[3] + bg[i] * (1 - fg[3])).concat(1);
const luminance = (rgb) => rgb.slice(0, 3).map((v) => v / 255)
  .map((v) => v <= .04045 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4)
  .reduce((sum, v, i) => sum + v * [.2126, .7152, .0722][i], 0);
const ratio = (fg, bg) => {
  const [a, b] = [luminance(fg), luminance(bg)].sort((a, b) => b - a);
  return (a + .05) / (b + .05);
};

for (const theme of ['night', 'day']) {
  const block = css.match(new RegExp(`:root\\[data-theme="${theme}"\\] \\{([^}]+)\\}`))?.[1];
  assert.ok(block, `missing ${theme} console palette`);
  const t = Object.fromEntries([...block.matchAll(/--([a-z0-9-]+):\s*(#[0-9a-f]{6}|rgba\([^;]+\));/g)]
    .map((m) => [m[1], color(m[2])]));
  const surfaces = ['bg-0', 'bg-2', 'glass-solid', 'glass-strong', 'surface-sunken'];
  const check = (fg, bg, minimum, label) => assert.ok(ratio(fg, bg) >= minimum,
    `${theme} ${label}: ${ratio(fg, bg).toFixed(2)} < ${minimum}`);

  test(`console ${theme}: text and data reach 4.5:1 on every solid surface`, () => {
    for (const bg of surfaces) for (const fg of ['ink', 'ink-soft', 'ink-mute', 'jade', 'ok', 'warn', 'bad', 'idle']) {
      check(t[fg], t[bg], 4.5, `${fg} on ${bg}`);
    }
  });
  test(`console ${theme}: the teal atmosphere wash keeps text at 4.5:1 and edges at 3:1`, () => {
    const wash = over(t['atmo-core'], t['bg-0']);
    for (const fg of ['ink', 'ink-soft', 'ink-mute', 'jade', 'ok', 'warn', 'bad', 'idle']) check(t[fg], wash, 4.5, `${fg} on wash`);
    for (const fg of ['edge', 'jade']) check(t[fg], wash, 3, `${fg} on wash`);
  });
  test(`console ${theme}: status chips and notes reach 4.5:1 on composited fills`, () => {
    for (const bg of surfaces) for (const status of ['ok', 'warn', 'bad', 'idle']) {
      check(t[status], over(t[status + '-soft'], t[bg]), 4.5, `${status} on ${status}-soft over ${bg}`);
    }
  });
  test(`console ${theme}: action ink reaches 4.5:1 across the cyan gradient`, () => {
    for (const bg of ['jade', 'jade-deep']) check(t['on-jade'], t[bg], 4.5, `on-jade on ${bg}`);
  });
  test(`console ${theme}: control boundaries and focus borders reach 3:1`, () => {
    for (const bg of surfaces) for (const fg of ['edge', 'jade']) check(t[fg], t[bg], 3, `${fg} on ${bg}`);
  });
}

test('console canvas carries the radial teal wash and a 3% contour while cards stay opaque', () => {
  const base = read('src/styles/base.css');
  assert.match(base, /radial-gradient\(ellipse 85% 65% at 25% 15%, var\(--atmo-core\), transparent 70%\)/);
  assert.match(base, /body::after \{[^}]*opacity: \.03;/);
  assert.equal(readFileSync(new URL('../app/public/contour-topo.svg', import.meta.url), 'utf8').startsWith('<svg'), true);
});

test('console Burmese body and headings keep the owner-specified leading', () => {
  const base = read('src/styles/base.css');
  assert.match(base, /:lang\(my\) body, :lang\(my\) \{ line-height: 1\.75;/);
  assert.match(base, /:lang\(my\) h1, :lang\(my\) h2, :lang\(my\) h3 \{ line-height: 1\.6;/);
});

test('the loading shell shows only the selected language before React loads', () => {
  const html = read('index.html');
  assert.match(html, /html\[lang="en"\] \.boot-my, html\[lang="my"\] \.boot-en \{ display: none; \}/);
  assert.doesNotMatch(html, /Loading MegaMagic · MegaMagic/);
});

// Run the pre-bundle script itself so persisted choices apply before the loader is read.
for (const lang of ['en', 'my']) test(`loading shell restores ${lang} before React mounts`, () => {
  const root = {dataset: {}, lang: 'my'};
  runInNewContext(read('public/theme-init.js'), {
    localStorage: {getItem: (key) => key === 'mm2_lang' ? lang : 'day'},
    document: {documentElement: root},
  });
  assert.equal(root.lang, lang);
  assert.equal(root.dataset.theme, 'day');
});
