// MegaMagic "Signal Gem": a brilliant-cut emerald seen from above with a golden Wi-Fi beacon in its table.
const fs = require("fs");
const f = (n) => +n.toFixed(2);
const C = [256, 262], R = 160, TABLE = 0.5, STAR = 0.71;
const deg = Math.PI / 180;
const at = (r, a) => [C[0] + r * Math.cos(a * deg), C[1] + r * Math.sin(a * deg)];
const N = 8, step = 360 / N, rot = -90 + step / 2; // flat top
const G = [], T = [], S = [], M = [];
for (let i = 0; i < N; i++) {
  const a = rot + i * step;
  G.push(at(R, a)); T.push(at(R * TABLE, a));
  S.push(at(R * STAR, a + step / 2)); M.push(at(R * Math.cos((step / 2) * deg), a + step / 2));
}
const poly = (pts) => "M" + pts.map((p) => `${f(p[0])} ${f(p[1])}`).join("L") + "Z";
const centroid = (pts) => [pts.reduce((s, p) => s + p[0], 0) / pts.length, pts.reduce((s, p) => s + p[1], 0) / pts.length];

// Light comes from the upper left. Facets facing it are bright mint; facets facing away are deep emerald.
const lightDir = Math.atan2(-0.75, -0.66);
const mix = (a, b, t) => a.map((v, i) => Math.round(v + (b[i] - v) * t));
const hex = (c) => "#" + c.map((v) => v.toString(16).padStart(2, "0")).join("");
const DARK = [1, 52, 33], MID = [8, 150, 98], LIGHT = [118, 248, 196], WHITE = [236, 255, 247];
let seed = 7; const rnd = () => ((seed = (seed * 16807) % 2147483647) / 2147483647);
function shade(pts, tilt, extra = 0) {
  const c = centroid(pts);
  const a = Math.atan2(c[1] - C[1], c[0] - C[0]);
  const facing = Math.cos(a - lightDir);              // -1 .. 1
  let t = 0.48 + 0.4 * facing * tilt + (rnd() - 0.5) * 0.2 + extra;
  t = Math.max(0.04, Math.min(0.9, t));
  const col = t < 0.5 ? mix(DARK, MID, t / 0.5) : t < 0.92 ? mix(MID, LIGHT, (t - 0.5) / 0.42) : mix(LIGHT, WHITE, (t - 0.92) / 0.08);
  return hex(col);
}
let facets = "";
for (let i = 0; i < N; i++) {
  const j = (i + 1) % N, h = (i + N - 1) % N;
  const kite = [S[h], T[i], S[i], G[i]];
  facets += `<path d="${poly(kite)}" fill="${shade(kite, 1.0)}"/>`;
  const star = [T[i], T[j], S[i]];
  facets += `<path d="${poly(star)}" fill="${shade(star, 0.6, 0.06)}"/>`;
  const ug1 = [S[i], G[i], M[i]], ug2 = [S[i], M[i], G[j]];
  facets += `<path d="${poly(ug1)}" fill="${shade(ug1, 1.2, -0.06)}"/>`;
  facets += `<path d="${poly(ug2)}" fill="${shade(ug2, 1.2, -0.06)}"/>`;
}
// Facet edges: thin bright lines, like light catching each cut.
let edges = "";
for (let i = 0; i < N; i++) {
  const j = (i + 1) % N;
  edges += `M${f(T[i][0])} ${f(T[i][1])}L${f(S[i][0])} ${f(S[i][1])}L${f(T[j][0])} ${f(T[j][1])}`;
  edges += `M${f(S[i][0])} ${f(S[i][1])}L${f(G[i][0])} ${f(G[i][1])}M${f(S[i][0])} ${f(S[i][1])}L${f(G[j][0])} ${f(G[j][1])}M${f(S[i][0])} ${f(S[i][1])}L${f(M[i][0])} ${f(M[i][1])}`;
}
const tablePath = poly(T), girdle = poly(G);
let pavilion = "";
for (let i = 0; i < N; i++) {
  const j = (i + 1) % N;
  const mid = [(T[i][0] + T[j][0]) / 2, (T[i][1] + T[j][1]) / 2];
  pavilion += `<path d="${poly([C, T[i], mid])}" fill="#ffffff" fill-opacity="${i % 2 ? 0.07 : 0.02}"/>`;
  pavilion += `<path d="${poly([C, mid, T[j]])}" fill="#001a10" fill-opacity="${i % 2 ? 0.12 : 0.2}"/>`;
}

// Wi-Fi beacon inside the table
const B = [C[0], C[1] + 34];
const arc = (r) => { const a0 = -135, a1 = -45; const p0 = [B[0] + r * Math.cos(a0 * deg), B[1] + r * Math.sin(a0 * deg)], p1 = [B[0] + r * Math.cos(a1 * deg), B[1] + r * Math.sin(a1 * deg)]; return `M${f(p0[0])} ${f(p0[1])}A${r} ${r} 0 0 1 ${f(p1[0])} ${f(p1[1])}`; };
const beacon = `${arc(28)}${arc(52)}${arc(76)}`;

const sparkle = (cx, cy, R, r) => { const p = []; for (let i = 0; i < 8; i++) { const a = i * 45 - 90, rr = i % 2 ? r : R; p.push([cx + rr * Math.cos(a * deg), cy + rr * Math.sin(a * deg)]); } return poly(p); };

const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512">
<title>MegaMagic</title>
<defs>
  <radialGradient id="bg" cx="50%" cy="38%" r="78%">
    <stop offset="0" stop-color="#0f5240"/><stop offset=".55" stop-color="#062a20"/><stop offset="1" stop-color="#021510"/>
  </radialGradient>
  <linearGradient id="rim" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0" stop-color="#bfffe3" stop-opacity=".45"/><stop offset=".4" stop-color="#bfffe3" stop-opacity=".06"/><stop offset="1" stop-color="#000" stop-opacity=".4"/>
  </linearGradient>
  <radialGradient id="spill" cx="50%" cy="50%" r="50%">
    <stop offset="0" stop-color="#36e3a2" stop-opacity=".45"/><stop offset="1" stop-color="#36e3a2" stop-opacity="0"/>
  </radialGradient>
  <radialGradient id="table" cx="42%" cy="34%" r="80%">
    <stop offset="0" stop-color="#43e0a5"/><stop offset=".55" stop-color="#0b9865"/><stop offset="1" stop-color="#045c3d"/>
  </radialGradient>
  <radialGradient id="core" cx="50%" cy="62%" r="55%">
    <stop offset="0" stop-color="#ffe7a1" stop-opacity=".55"/><stop offset="1" stop-color="#ffe7a1" stop-opacity="0"/>
  </radialGradient>
  <linearGradient id="gold" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0" stop-color="#fff4cf"/><stop offset=".5" stop-color="#ffd773"/><stop offset="1" stop-color="#f1b23a"/>
  </linearGradient>
  <linearGradient id="sheen" x1="0" y1="0" x2="1" y2="1">
    <stop offset="0" stop-color="#ffffff" stop-opacity=".5"/><stop offset=".45" stop-color="#ffffff" stop-opacity="0"/>
  </linearGradient>
  <radialGradient id="glint"><stop offset="0" stop-color="#fff" stop-opacity=".95"/><stop offset=".3" stop-color="#fff6d8" stop-opacity=".35"/><stop offset="1" stop-color="#fff6d8" stop-opacity="0"/></radialGradient>
  <linearGradient id="depth" x1="0" y1="0" x2="1" y2="1">
    <stop offset="0" stop-color="#ffffff" stop-opacity=".14"/><stop offset=".45" stop-color="#ffffff" stop-opacity="0"/><stop offset="1" stop-color="#012a1b" stop-opacity=".38"/>
  </linearGradient>
  <filter id="blur2" x="-10%" y="-10%" width="120%" height="120%"><feGaussianBlur stdDeviation="2.5"/></filter>
  <filter id="blur8" x="-30%" y="-30%" width="160%" height="160%"><feGaussianBlur stdDeviation="8"/></filter>
  <filter id="blur20" x="-40%" y="-40%" width="180%" height="180%"><feGaussianBlur stdDeviation="20"/></filter>
  <filter id="glow" x="-40%" y="-40%" width="180%" height="180%"><feGaussianBlur stdDeviation="5" result="b"/><feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge></filter>
  <clipPath id="tileClip"><rect x="24" y="24" width="464" height="464" rx="116"/></clipPath>
  <clipPath id="gemClip"><path d="${girdle}"/></clipPath>
  <clipPath id="tableClip"><path d="${tablePath}"/></clipPath>
</defs>

<rect x="24" y="24" width="464" height="464" rx="116" fill="url(#bg)"/>
<g clip-path="url(#tileClip)">
  <ellipse cx="256" cy="430" rx="190" ry="60" fill="url(#spill)" filter="url(#blur20)"/>
  <ellipse cx="264" cy="288" rx="160" ry="160" fill="#000" opacity=".5" filter="url(#blur20)"/>
</g>
<rect x="25.5" y="25.5" width="461" height="461" rx="114.5" fill="none" stroke="url(#rim)" stroke-width="3"/>

<!-- the gem -->
<g>
  ${facets}
  <path d="${tablePath}" fill="url(#table)"/>
  <g clip-path="url(#tableClip)">
    ${pavilion}
    <circle cx="${B[0]}" cy="${B[1] - 20}" r="90" fill="url(#core)"/>
  </g>
  <path d="${girdle}" fill="url(#depth)"/>
  <g clip-path="url(#gemClip)"><path d="M${f(C[0] - 150)} ${f(C[1] - 40)}L${f(C[0] - 40)} ${f(C[1] - 150)}L${f(C[0] - 28)} ${f(C[1] - 138)}L${f(C[0] - 138)} ${f(C[1] - 28)}Z" fill="#ffffff" opacity=".22" filter="url(#blur2)"/></g>
  <path d="${edges}" fill="none" stroke="#d9fff0" stroke-opacity=".28" stroke-width="1.3" stroke-linejoin="round"/>
  <path d="${tablePath}" fill="none" stroke="#e8fff6" stroke-opacity=".55" stroke-width="1.6" stroke-linejoin="round"/>
  <path d="${girdle}" fill="none" stroke="#c9ffe8" stroke-opacity=".6" stroke-width="2.2" stroke-linejoin="round"/>
  <g clip-path="url(#gemClip)"><path d="M${f(G[6][0])} ${f(G[6][1])}L${f(G[7][0])} ${f(G[7][1])}L${f(G[0][0])} ${f(G[0][1])}L256 262Z" fill="url(#sheen)" opacity=".35"/></g>
</g>

<!-- beacon -->
<g filter="url(#glow)" fill="none" stroke="url(#gold)" stroke-linecap="round" stroke-width="13">
  <path d="${beacon}"/>
</g>
<circle cx="${B[0]}" cy="${B[1]}" r="11" fill="url(#gold)" filter="url(#glow)"/>

<!-- sparkles -->
<circle cx="372" cy="128" r="46" fill="url(#glint)"/>
<path d="${sparkle(372, 128, 34, 6)}" fill="#fffdf2"/>
<circle cx="140" cy="380" r="24" fill="url(#glint)" opacity=".8"/>
<path d="${sparkle(140, 380, 15, 3)}" fill="#e9fff5"/>
<circle cx="412" cy="196" r="5" fill="#fff" opacity=".85"/>
</svg>
`;
fs.writeFileSync(__dirname + "/megamagic-gem.svg", svg);
console.log("wrote", svg.length);
