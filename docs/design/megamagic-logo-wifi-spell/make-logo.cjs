// MegaMagic "Wi-Fi Spell": a wand whose glowing star tip casts the Wi-Fi arcs as spell light.
const fs = require("fs");
const f = (n) => +n.toFixed(2);
const deg = Math.PI / 180;
const B = [200, 316];                  // wand tip = the Wi-Fi "dot"
const DIR = -45;                       // arcs open toward the upper right
const RADII = [78, 140, 202];
const at = (r, a) => [B[0] + r * Math.cos(a * deg), B[1] + r * Math.sin(a * deg)];
const arc = (r, span = 84) => { const a0 = DIR - span / 2, a1 = DIR + span / 2; const p = at(r, a0), q = at(r, a1); return `M${f(p[0])} ${f(p[1])}A${r} ${r} 0 0 1 ${f(q[0])} ${f(q[1])}`; };
const star = (cx, cy, R, r) => { const p = []; for (let i = 0; i < 8; i++) { const a = i * 45 - 90, rr = i % 2 ? r : R; p.push([cx + rr * Math.cos(a * deg), cy + rr * Math.sin(a * deg)]); } return "M" + p.map((x) => `${f(x[0])} ${f(x[1])}`).join("L") + "Z"; };

// magic dust riding the arcs (fixed positions: deterministic)
const dust = [[78, -12, 5.5], [140, -70, 7], [140, -24, 4.5], [202, -8, 9], [202, -60, 5], [202, -32, 3.5], [112, -40, 3], [172, -84, 3.5], [234, -36, 4.5], [234, -72, 3]];
let dustSvg = "";
for (const [r, a, s] of dust) {
  const [x, y] = at(r, a);
  dustSvg += s > 5 ? `<circle cx="${f(x)}" cy="${f(y)}" r="${f(s * 2.6)}" fill="url(#spark)"/><path d="${star(x, y, s * 1.7, s * 0.32)}" fill="#fffdf0"/>` : `<circle cx="${f(x)}" cy="${f(y)}" r="${f(s * 2)}" fill="url(#spark)" opacity=".8"/><circle cx="${f(x)}" cy="${f(y)}" r="${f(s * 0.55)}" fill="#ffffff"/>`;
}

const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512">
<title>MegaMagic</title>
<defs>
  <radialGradient id="tile" cx="72%" cy="22%" r="100%">
    <stop offset="0" stop-color="#1fae80"/><stop offset=".4" stop-color="#0b7a57"/><stop offset=".78" stop-color="#05452f"/><stop offset="1" stop-color="#022a1e"/>
  </radialGradient>
  <linearGradient id="rim" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0" stop-color="#fff" stop-opacity=".5"/><stop offset=".35" stop-color="#fff" stop-opacity=".07"/><stop offset=".7" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".35"/>
  </linearGradient>
  <linearGradient id="gloss" gradientUnits="userSpaceOnUse" x1="0" y1="24" x2="0" y2="200">
    <stop offset="0" stop-color="#fff" stop-opacity=".2"/><stop offset="1" stop-color="#fff" stop-opacity="0"/>
  </linearGradient>
  <radialGradient id="aura" cx="${f(B[0])}" cy="${f(B[1])}" r="260" gradientUnits="userSpaceOnUse">
    <stop offset="0" stop-color="#ffe9a6" stop-opacity=".42"/><stop offset=".35" stop-color="#7ff5c9" stop-opacity=".16"/><stop offset="1" stop-color="#7ff5c9" stop-opacity="0"/>
  </radialGradient>
  <linearGradient id="a1" x1="0" y1="1" x2="1" y2="0"><stop offset="0" stop-color="#ffe6a0"/><stop offset="1" stop-color="#ffc94d"/></linearGradient>
  <linearGradient id="a2" x1="0" y1="1" x2="1" y2="0"><stop offset="0" stop-color="#ffd772"/><stop offset=".5" stop-color="#d6f59a"/><stop offset="1" stop-color="#8ff0c4"/></linearGradient>
  <linearGradient id="a3" x1="0" y1="1" x2="1" y2="0"><stop offset="0" stop-color="#a6f7d3"/><stop offset="1" stop-color="#5fe8b4"/></linearGradient>
  <linearGradient id="shaft" gradientUnits="userSpaceOnUse" x1="0" y1="-17" x2="0" y2="17">
    <stop offset="0" stop-color="#a9c7ba"/><stop offset=".18" stop-color="#fff"/><stop offset=".34" stop-color="#fbf8ef"/><stop offset=".7" stop-color="#d9e6df"/><stop offset="1" stop-color="#7f9c90"/>
  </linearGradient>
  <linearGradient id="cap" gradientUnits="userSpaceOnUse" x1="0" y1="-14" x2="0" y2="14">
    <stop offset="0" stop-color="#1e4a3c"/><stop offset=".22" stop-color="#7cc7a8"/><stop offset=".36" stop-color="#1a5240"/><stop offset=".75" stop-color="#03211a"/><stop offset="1" stop-color="#010f0b"/>
  </linearGradient>
  <linearGradient id="band" gradientUnits="userSpaceOnUse" x1="0" y1="-17" x2="0" y2="17">
    <stop offset="0" stop-color="#b9cfc5"/><stop offset=".2" stop-color="#fff"/><stop offset=".55" stop-color="#c7d8d0"/><stop offset="1" stop-color="#6c877b"/>
  </linearGradient>
  <radialGradient id="spark"><stop offset="0" stop-color="#fff" stop-opacity=".95"/><stop offset=".35" stop-color="#fff3c7" stop-opacity=".4"/><stop offset="1" stop-color="#fff3c7" stop-opacity="0"/></radialGradient>
  <radialGradient id="tipglow"><stop offset="0" stop-color="#fffbe6"/><stop offset=".25" stop-color="#ffe089" stop-opacity=".85"/><stop offset=".6" stop-color="#ffd25e" stop-opacity=".25"/><stop offset="1" stop-color="#ffd25e" stop-opacity="0"/></radialGradient>
  <filter id="glow" filterUnits="userSpaceOnUse" x="0" y="0" width="512" height="512"><feGaussianBlur stdDeviation="9"/></filter>
  <filter id="soft" x="-20%" y="-20%" width="140%" height="140%"><feGaussianBlur stdDeviation="7"/></filter>
  <clipPath id="tileClip"><rect x="24" y="24" width="464" height="464" rx="116"/></clipPath>
</defs>

<rect x="24" y="24" width="464" height="464" rx="116" fill="url(#tile)"/>
<g clip-path="url(#tileClip)">
  <rect x="24" y="24" width="464" height="464" fill="url(#aura)"/>
  <path d="M24 24H488V206C380 240 236 244 24 198Z" fill="url(#gloss)"/>
  <g transform="translate(${f(B[0] + 10)} ${f(B[1] + 14)}) rotate(135)" filter="url(#soft)" opacity=".4">
    <path d="M20 -12L150 -16A16 16 0 0 1 150 16L20 12Z" fill="#011a12"/>
  </g>
</g>
<rect x="25.5" y="25.5" width="461" height="461" rx="114.5" fill="none" stroke="url(#rim)" stroke-width="3"/>

<!-- Wi-Fi arcs as spell light: soft glow, solid body, bright core -->
<g fill="none" stroke-linecap="round">
  <path d="${arc(RADII[0])}" stroke="#ffd25e" stroke-width="34" opacity=".55" filter="url(#glow)"/>
  <path d="${arc(RADII[1])}" stroke="#b8f0a0" stroke-width="34" opacity=".5" filter="url(#glow)"/>
  <path d="${arc(RADII[2])}" stroke="#6ff2c0" stroke-width="34" opacity=".5" filter="url(#glow)"/>
  <path d="${arc(RADII[0])}" stroke="url(#a1)" stroke-width="22"/>
  <path d="${arc(RADII[1])}" stroke="url(#a2)" stroke-width="22"/>
  <path d="${arc(RADII[2])}" stroke="url(#a3)" stroke-width="22"/>
  <path d="${arc(RADII[0] - 3, 70)}" stroke="#fffbea" stroke-opacity=".85" stroke-width="5"/>
  <path d="${arc(RADII[1] - 3, 74)}" stroke="#fffdf2" stroke-opacity=".75" stroke-width="5"/>
  <path d="${arc(RADII[2] - 3, 76)}" stroke="#f2fff9" stroke-opacity=".7" stroke-width="5"/>
</g>
${dustSvg}

<!-- wand: pearl shaft pointing down-left from the tip -->
<g transform="translate(${f(B[0])} ${f(B[1])}) rotate(135)">
  <path d="M22 -13L150 -17A17 17 0 0 1 150 17L22 13A13 13 0 0 1 22 -13Z" fill="url(#shaft)"/>
  <path d="M22 -13.1L58 -13.9V13.9L22 13.1A13 13 0 0 1 22 -13.1Z" fill="url(#cap)"/>
  <rect x="56" y="-15.6" width="8" height="31.2" rx="3" fill="url(#band)"/>
  <rect x="126" y="-17.6" width="7" height="35.2" rx="2.5" fill="url(#band)"/>
  <rect x="136" y="-17.6" width="3.5" height="35.2" rx="1.4" fill="url(#band)" opacity=".85"/>
  <path d="M68 7.2L160 9" stroke="#fff" stroke-opacity=".9" stroke-width="3" stroke-linecap="round"/>
</g>

<!-- the star at the tip is the Wi-Fi dot -->
<circle cx="${f(B[0])}" cy="${f(B[1])}" r="58" fill="url(#tipglow)"/>
<path d="${star(B[0], B[1], 40, 9)}" fill="#ffe7a0"/>
<path d="${star(B[0], B[1], 40, 9)}" fill="none" stroke="#fff6d6" stroke-width="2"/>
<path d="${star(B[0], B[1], 24, 6)}" fill="#fffdf2"/>
<circle cx="${f(B[0])}" cy="${f(B[1])}" r="5" fill="#ffffff"/>
</svg>
`;
fs.writeFileSync(__dirname + "/megamagic-spell.svg", svg);
console.log("wrote", svg.length);
