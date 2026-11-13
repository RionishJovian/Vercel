// MegaMagic "Wi-Fi Spell — Glass": vector rebuild of the owner's Gemini render.
// Clear dark glass tile, chrome wand casting tube-lit Wi-Fi arcs, gold dust, faint circuit traces. Transparent background.
const fs = require("fs");
const f = (n) => +n.toFixed(2);
const deg = Math.PI / 180;
const B = [190, 330];                 // star tip = the Wi-Fi dot
const DIR = -45, RADII = [80, 142, 204];
const at = (r, a, c = B) => [c[0] + r * Math.cos(a * deg), c[1] + r * Math.sin(a * deg)];
const arc = (r, span = 84) => { const p = at(r, DIR - span / 2), q = at(r, DIR + span / 2); return `M${f(p[0])} ${f(p[1])}A${r} ${r} 0 0 1 ${f(q[0])} ${f(q[1])}`; };
const star = (cx, cy, R, r) => { const p = []; for (let i = 0; i < 8; i++) { const a = i * 45 - 90, rr = i % 2 ? r : R; p.push([cx + rr * Math.cos(a * deg), cy + rr * Math.sin(a * deg)]); } return "M" + p.map((x) => `${f(x[0])} ${f(x[1])}`).join("L") + "Z"; };

// Gold dust: deterministic pseudo-random cloud fanning from the star
let seed = 20261113; const rnd = () => ((seed = (seed * 48271) % 2147483647) / 2147483647);
let dust = "";
for (let i = 0; i < 720; i++) {
  const spread = rnd() < 0.78;
  const a = spread ? -110 + rnd() * 125 : 100 + rnd() * 120;          // main fan up-right, a little spill down-left
  const r = spread ? 14 + Math.pow(rnd(), 2.1) * 260 : 12 + Math.pow(rnd(), 1.5) * 80;
  const [x, y] = at(r, a);
  if (x < 52 || x > 460 || y < 52 || y > 460) continue;
  const s = 0.45 + Math.pow(rnd(), 4) * 2.3;
  const o = (0.45 + rnd() * 0.55) * (1 - r / 430);
  const col = rnd() < 0.72 ? "#ffe2a0" : "#ffffff";
  dust += `<circle cx="${f(x)}" cy="${f(y)}" r="${f(s)}" fill="${col}" fill-opacity="${f(Math.max(0.15, o))}"/>`;
}
// Faint circuit traces inside the glass (right edge and lower left)
const traces = [
  "M452 150H424L410 164V214", "M452 176H432L420 188V250", "M452 300H430L416 314V352",
  "M150 452V430L136 416H96", "M230 452V436L244 422H286", "M310 452V440",
];
const pads = [[410, 214], [420, 250], [416, 352], [96, 416], [286, 422], [310, 440]];

const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512">
<title>MegaMagic</title>
<defs>
  <linearGradient id="glass" x1="0" y1="0" x2="1" y2="1">
    <stop offset="0" stop-color="#16504e"/><stop offset=".45" stop-color="#0c3537"/><stop offset="1" stop-color="#071f22"/>
  </linearGradient>
  <linearGradient id="slab" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0" stop-color="#0e3a39"/><stop offset="1" stop-color="#04191a"/>
  </linearGradient>
  <linearGradient id="bevel" x1="0" y1="0" x2="1" y2="1">
    <stop offset="0" stop-color="#c8fff2" stop-opacity=".85"/><stop offset=".35" stop-color="#7fe6d0" stop-opacity=".35"/>
    <stop offset=".7" stop-color="#3fb9a0" stop-opacity=".25"/><stop offset="1" stop-color="#9ff7dc" stop-opacity=".6"/>
  </linearGradient>
  <linearGradient id="sheen" gradientUnits="userSpaceOnUse" x1="40" y1="40" x2="300" y2="300">
    <stop offset="0" stop-color="#ffffff" stop-opacity=".2"/><stop offset=".42" stop-color="#ffffff" stop-opacity="0"/>
  </linearGradient>
  <radialGradient id="aura" cx="${B[0]}" cy="${B[1]}" r="280" gradientUnits="userSpaceOnUse">
    <stop offset="0" stop-color="#ffd978" stop-opacity=".38"/><stop offset=".3" stop-color="#5ff0bd" stop-opacity=".14"/><stop offset="1" stop-color="#5ff0bd" stop-opacity="0"/>
  </radialGradient>
  <radialGradient id="floorGlow" cx="50%" cy="100%" r="60%">
    <stop offset="0" stop-color="#ffcf5a" stop-opacity=".7"/><stop offset=".45" stop-color="#5ff0bd" stop-opacity=".32"/><stop offset="1" stop-color="#5ff0bd" stop-opacity="0"/>
  </radialGradient>
  <linearGradient id="a1" x1="0" y1="1" x2="1" y2="0"><stop offset="0" stop-color="#ffd26a"/><stop offset="1" stop-color="#ffbe45"/></linearGradient>
  <linearGradient id="a2" x1="0" y1="1" x2="1" y2="0"><stop offset="0" stop-color="#d9f27a"/><stop offset="1" stop-color="#b5ec6e"/></linearGradient>
  <linearGradient id="a3" x1="0" y1="1" x2="1" y2="0"><stop offset="0" stop-color="#7ef0b8"/><stop offset="1" stop-color="#52e3a9"/></linearGradient>
  <linearGradient id="chrome" gradientUnits="userSpaceOnUse" x1="0" y1="-15" x2="0" y2="15">
    <stop offset="0" stop-color="#2b3336"/><stop offset=".14" stop-color="#e9eef0"/><stop offset=".26" stop-color="#ffffff"/>
    <stop offset=".42" stop-color="#8b9599"/><stop offset=".58" stop-color="#1a2124"/><stop offset=".78" stop-color="#b9c2c5"/><stop offset="1" stop-color="#4a5357"/>
  </linearGradient>
  <linearGradient id="collar" gradientUnits="userSpaceOnUse" x1="0" y1="-13" x2="0" y2="13">
    <stop offset="0" stop-color="#0b0f10"/><stop offset=".25" stop-color="#5c676b"/><stop offset=".45" stop-color="#0d1213"/><stop offset="1" stop-color="#000"/>
  </linearGradient>
  <radialGradient id="tip"><stop offset="0" stop-color="#fffef6"/><stop offset=".2" stop-color="#ffeaa6" stop-opacity=".95"/><stop offset=".5" stop-color="#ffc94d" stop-opacity=".35"/><stop offset="1" stop-color="#ffc94d" stop-opacity="0"/></radialGradient>
  <radialGradient id="spark"><stop offset="0" stop-color="#fff" stop-opacity=".95"/><stop offset=".35" stop-color="#fff3c7" stop-opacity=".4"/><stop offset="1" stop-color="#fff3c7" stop-opacity="0"/></radialGradient>
  <filter id="glow" filterUnits="userSpaceOnUse" x="0" y="0" width="512" height="512"><feGaussianBlur stdDeviation="10"/></filter>
  <filter id="soft" filterUnits="userSpaceOnUse" x="0" y="0" width="512" height="512"><feGaussianBlur stdDeviation="6"/></filter>
  <clipPath id="tileClip"><rect x="40" y="40" width="432" height="432" rx="100"/></clipPath>
</defs>

<!-- glass slab: thickness below, then the face -->
<rect x="40" y="52" width="432" height="432" rx="100" fill="url(#slab)"/>
<rect x="40" y="40" width="432" height="432" rx="100" fill="url(#glass)"/>
<g clip-path="url(#tileClip)">
  <rect x="40" y="40" width="432" height="432" fill="url(#aura)"/>
  <ellipse cx="256" cy="476" rx="230" ry="70" fill="url(#floorGlow)"/>
  <g fill="none" stroke="#6ff2c8" stroke-opacity=".16" stroke-width="2" stroke-linejoin="round">${traces.map((d) => `<path d="${d}"/>`).join("")}</g>
  ${pads.map(([x, y]) => `<circle cx="${x}" cy="${y}" r="3.2" fill="none" stroke="#6ff2c8" stroke-opacity=".2" stroke-width="1.6"/>`).join("")}
  <path d="M40 40H360C300 120 160 210 40 250Z" fill="url(#sheen)"/>
</g>

<!-- Wi-Fi arcs as glowing tubes: glow, shadow edge, body, top light, hot core -->
<g fill="none" stroke-linecap="round">
  <path d="${arc(RADII[0])}" stroke="#ffc94d" stroke-width="40" opacity=".5" filter="url(#glow)"/>
  <path d="${arc(RADII[1])}" stroke="#c6f06a" stroke-width="40" opacity=".42" filter="url(#glow)"/>
  <path d="${arc(RADII[2])}" stroke="#4fe6a8" stroke-width="40" opacity=".45" filter="url(#glow)"/>
  <path d="${arc(RADII[0] + 2)}" stroke="#a96d12" stroke-opacity=".55" stroke-width="26"/>
  <path d="${arc(RADII[1] + 2)}" stroke="#5e8a1d" stroke-opacity=".5" stroke-width="26"/>
  <path d="${arc(RADII[2] + 2)}" stroke="#137a56" stroke-opacity=".55" stroke-width="26"/>
  <path d="${arc(RADII[0])}" stroke="url(#a1)" stroke-width="24"/>
  <path d="${arc(RADII[1])}" stroke="url(#a2)" stroke-width="24"/>
  <path d="${arc(RADII[2])}" stroke="url(#a3)" stroke-width="24"/>
  <path d="${arc(RADII[0] - 5, 72)}" stroke="#fff6d8" stroke-opacity=".9" stroke-width="6"/>
  <path d="${arc(RADII[1] - 5, 76)}" stroke="#fbffe6" stroke-opacity=".85" stroke-width="6"/>
  <path d="${arc(RADII[2] - 5, 78)}" stroke="#ecfff6" stroke-opacity=".85" stroke-width="6"/>
</g>

<!-- gold dust -->
<g>${dust}</g>
${[[RADII[0], -12, 7], [RADII[1], -66, 8], [RADII[2], -6, 10]].map(([r, a, s]) => { const [x, y] = at(r, a); return `<circle cx="${f(x)}" cy="${f(y)}" r="${s * 2.6}" fill="url(#spark)"/><path d="${star(x, y, s * 1.8, s * 0.3)}" fill="#fffdf2"/>`; }).join("")}

<!-- glass bevel on top of everything inside the tile -->
<rect x="44" y="44" width="424" height="424" rx="96" fill="none" stroke="url(#bevel)" stroke-width="8"/>
<rect x="40.8" y="40.8" width="430.4" height="430.4" rx="99.2" fill="none" stroke="#e6fff8" stroke-opacity=".55" stroke-width="1.6"/>
<rect x="52" y="52" width="408" height="408" rx="88" fill="none" stroke="#ffffff" stroke-opacity=".08" stroke-width="2"/>

<!-- chrome wand, reaching past the tile's lower-left corner -->
<g transform="translate(${B[0] + 8} ${B[1] + 14}) rotate(135)" filter="url(#soft)" opacity=".5">
  <path d="M22 -11L196 -14A14 14 0 0 1 196 14L22 11Z" fill="#000"/>
</g>
<g transform="translate(${B[0]} ${B[1]}) rotate(135)">
  <path d="M20 -11L196 -15A15 15 0 0 1 196 15L20 11A11 11 0 0 1 20 -11Z" fill="url(#chrome)"/>
  <path d="M20 -11.2L62 -12.2V12.2L20 11.2A11 11 0 0 1 20 -11.2Z" fill="url(#collar)"/>
  <rect x="60" y="-13.4" width="6" height="26.8" rx="2" fill="url(#chrome)"/>
  <rect x="172" y="-15.6" width="5" height="31.2" rx="2" fill="#20282b" opacity=".55"/>
  <rect x="180" y="-15.6" width="3" height="31.2" rx="1.2" fill="#20282b" opacity=".45"/>
  <path d="M70 -7L198 -9" stroke="#ffffff" stroke-opacity=".9" stroke-width="2.4" stroke-linecap="round"/>
  <path d="M26 -6L56 -6.6" stroke="#ffe6a0" stroke-opacity=".7" stroke-width="2" stroke-linecap="round"/>
</g>

<!-- star at the tip = the Wi-Fi dot -->
<circle cx="${B[0]}" cy="${B[1]}" r="74" fill="url(#tip)"/>
<path d="${star(B[0], B[1], 46, 8)}" fill="#ffe8a6"/>
<path d="${star(B[0], B[1], 28, 5)}" fill="#fffef4"/>
<path d="M${B[0] - 70} ${B[1]}H${B[0] + 70}M${B[0]} ${B[1] - 64}V${B[1] + 64}" stroke="#fff4cf" stroke-opacity=".45" stroke-width="1.6" stroke-linecap="round"/>
<circle cx="${B[0]}" cy="${B[1]}" r="6" fill="#ffffff"/>
</svg>
`;
fs.writeFileSync(__dirname + "/megamagic-glass.svg", svg);
console.log("wrote", svg.length);
