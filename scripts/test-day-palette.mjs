import fs from 'node:fs';
import assert from 'node:assert/strict';
const css=fs.readFileSync('web/assets/css/redesign.css','utf8');
const day=css.split(':root[data-theme="day"] {')[1].split(':root[data-theme="night"]')[0];
const color=name=>day.match(new RegExp(`--${name}: (#[a-f0-9]+);`))[1];
const luminance=hex=>{const c=hex.slice(1).match(/../g).map(s=>parseInt(s,16)/255).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4);return c[0]*.2126+c[1]*.7152+c[2]*.0722};
const contrast=(a,b)=>{const x=luminance(a),y=luminance(b);return (Math.max(x,y)+.05)/(Math.min(x,y)+.05)};
for(const [fg,bg] of [['foreground','background'],['foreground','surface'],['foreground-muted','surface'],['sidebar-foreground','sidebar'],['sidebar-foreground','sidebar-selected'],['on-primary','primary'],['on-primary','primary-hover']]){
 const ratio=contrast(color(fg),color(bg));assert.ok(ratio>=4.5,`${fg}/${bg}: ${ratio.toFixed(2)} must meet normal-text contrast`);
}
for(const shade of ['#0a3b25','#2a6151','#b2b7aa','#fcf7f0','#d8c2a4'])assert.ok(day.includes(shade));
console.log('Day palette passed: all five reference shades and normal-text contrast.');
