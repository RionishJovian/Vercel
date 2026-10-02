import fs from 'node:fs';
import vm from 'node:vm';
import assert from 'node:assert/strict';
const html = fs.readFileSync(new URL('../web/app.html', import.meta.url), 'utf8');
const code = html.slice(html.indexOf('let magicDudeTurns ='), html.indexOf('async function renderAIUsage('));
const elements = new Map();
const frames = new Map();
let frameID = 0, posts = 0, resolveReply, resize;
function element(id) {
  return {id, hidden: false, dataset: {}, value: '', disabled: false, scrollTop: 0, scrollHeight: 500,
    handlers: {}, children: [], innerHTML: '', focusCalls: [],
    addEventListener(event, fn) { this.handlers[event] = fn; },
    focus(options) { this.focusCalls.push(options); },
    appendChild(child) { this.children.push(child); }, get firstChild() { return null; }};
}
for (const id of ['magic-dude-popup','md-messages','md-form','md-question','md-send','md-note','md-close']) elements.set(id, element(id));
const context = vm.createContext({
  document: {getElementById: id => elements.get(id), querySelectorAll: () => [], addEventListener() {}, createElement: () => element('stack')},
  requestAnimationFrame: fn => { frames.set(++frameID, fn); return frameID; }, cancelAnimationFrame: id => frames.delete(id),
  ResizeObserver: class { constructor(fn) { resize = fn; } disconnect() {} observe() {} },
  esc: value => String(value).replace(/</g,'&lt;').replace(/>/g,'&gt;'),
  MM: { post: () => { posts++; return new Promise(resolve => { resolveReply = resolve; }); } }
});
vm.runInContext(code, context);
function flush() { const pending = [...frames.values()]; frames.clear(); pending.forEach(fn => fn()); }
context.initMagicDudePopup(); flush();
const list = elements.get('md-messages'), popup = elements.get('magic-dude-popup'), input = elements.get('md-question');
assert.equal(list.scrollTop, 500);
list.scrollHeight = 900; resize(); flush(); assert.equal(list.scrollTop, 900, 'late layout follows latest content');
popup.hidden = true; list.scrollHeight = 1200; resize(); flush(); assert.equal(list.scrollTop,900);
context.setMagicDudeOpen(true); flush(); assert.equal(list.scrollTop,1200, 'reopen scrolls after layout');
input.value = 'library ဘယ်မှာလဲ';
const submit = elements.get('md-form').handlers.submit;
const first = submit({preventDefault(){}});
input.value = 'duplicate'; await submit({preventDefault(){}});
assert.equal(posts,1, 'one in-flight request');
popup.hidden = true;
const focusCount = input.focusCalls.length;
resolveReply({answer: '<script>unsafe</script>'}); await first; flush();
assert.equal(input.focusCalls.length,focusCount, 'closed popup never steals focus');
assert.ok(list.innerHTML.includes('&lt;script&gt;'));
assert.ok(!list.innerHTML.includes('<script>'));
context.setMagicDudeOpen(true); flush();
assert.ok(input.focusCalls.every(options => options.preventScroll), 'focus never scrolls the page');
let submitted = false; elements.get('md-form').requestSubmit = () => { submitted = true; };
input.handlers.keydown({key:'Enter',isComposing:true,preventDefault(){}});
assert.equal(submitted,false, 'IME Enter does not send');
assert.ok(html.includes('lang="my"'));
assert.ok(!code.includes('apiErrorText(err'), 'provider errors do not leak English/raw details');
console.log('Magic Dude tests passed: deferred/resize/reopen scroll, duplicate guard, escaping, hidden focus, IME and Burmese UI.');
