import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

// Go supplies the actual function output to this runner. The ordinary critical-UI
// command reads the same raw HTML template without requiring a Go build.
const source = process.argv.includes('--html-stdin') ? '' : readFileSync(new URL('../internal/provisioner/routeros/portal_deploy.go', import.meta.url), 'utf8');
const html = process.argv.includes('--html-stdin') ? readFileSync(0, 'utf8') : source.match(/func portalLoginHTML\(\) string \{\s*return `([\s\S]*?)`\s*\}/)?.[1];
assert.ok(html, 'actual router login HTML must be available');
const attributes = text => Object.fromEntries([...text.matchAll(/([\w-]+)="([^"]*)"/g)].map(match => [match[1], match[2]]));
const formMarkup = id => {
  const found = [...html.matchAll(/<form\b([^>]*)>([\s\S]*?)<\/form>/g)].find(match => attributes(match[1]).id === id);
  assert.ok(found, `form ${id} must exist in the generated HTML`);
  return { attrs: attributes(found[1]), body: found[2] };
};
const visibleForm = formMarkup('voucher-login');
const wireForm = formMarkup('sendin');
const md5 = text => createHash('md5').update(Buffer.from(text, 'latin1')).digest('hex');
const binaryID = '\xf9';
const binaryChallenge = Buffer.from('ef0dd80b119c65a5c3ab62a65b7dfdce', 'hex').toString('latin1');
const octal = text => [...text].map(char => '\\' + char.charCodeAt(0).toString(8).padStart(3, '0')).join('');

function fixture({ chap = true, helper = md5, submitError = false, code = '  abcd-12_34  ', preview = false, consent } = {}) {
  const elements = new Map();
  for (const match of html.matchAll(/<(?:form|input|button|p)\b([^>]*)>/g)) {
    const attr = attributes(match[1]);
    if (attr.id) elements.set(attr.id, { value: '', textContent: '', hidden: /\bhidden\b/.test(match[1]), disabled: false, focus() { this.focused = true; } });
  }
  let submissions = 0;
  const payloads = [];
  const fields = {};
  for (const match of wireForm.body.matchAll(/<input\b([^>]*)>/g)) {
    const attr = attributes(match[1]);
    if (attr.name) fields[attr.name] = { value: attr.value || '' };
  }
  elements.get('voucher').value = code;
  // Other login tests represent a guest who accepted any rendered terms;
  // the dedicated consent test below supplies an explicit refusal instead.
  if (elements.has('voucher-consent')) elements.get('voucher-consent').checked = true;
  if (consent !== undefined) elements.set('voucher-consent', { checked: consent });
  elements.get('sendin').elements = fields;
  elements.get('sendin').submit = () => {
    if (submitError) throw new Error('navigation unavailable');
    submissions++;
    payloads.push(Object.fromEntries(Object.entries(fields).map(([name, field]) => [name, field.value])));
  };
  const timers = new Map();
  const listeners = new Map();
  let nextTimer = 0;
  const context = vm.createContext({
    document: { body: { getAttribute: name => name === 'data-preview' && preview ? 'true' : null }, getElementById: id => elements.get(id) },
    setTimeout(callback, delay) { timers.set(++nextTimer, { callback, delay }); return nextTimer; },
    clearTimeout(id) { timers.delete(id); },
    addEventListener(event, callback) { listeners.set(event, callback); },
    ...(helper ? { hexMD5: helper } : {}),
  });
  context.window = context;
  const rendered = html.replaceAll('$(chap-id)', chap ? octal(binaryID) : '').replaceAll('$(chap-challenge)', chap ? octal(binaryChallenge) : '');
  for (const match of rendered.matchAll(/<script\b([^>]*)>([\s\S]*?)<\/script>/g)) {
    if (!attributes(match[1]).src) vm.runInContext(match[2], context, { filename: 'router-login.html' });
  }
  return {
    context, elements, timers, listeners, payloads,
    get submissions() { return submissions; },
    // This is the exact onsubmit attribute used by both Enter and the button.
    submit() { return vm.runInContext(`(function(){${visibleForm.attrs.onsubmit}})()`, context); },
    expire() { for (const { callback } of [...timers.values()]) callback(); },
  };
}

test('generated page submits a real form and loads the cloned router-local CHAP helper', () => {
  assert.equal(wireForm.attrs.action, '$(link-login-only)');
  assert.equal(wireForm.attrs.method, 'post');
  assert.equal(visibleForm.attrs.action, '$(link-login-only)');
  assert.equal(visibleForm.attrs.method, 'post');
  assert.match(visibleForm.body, /<button\b[^>]*type="submit"/);
  assert.match(visibleForm.body, /<input\b[^>]*id="voucher"/);
  const sources = [...html.matchAll(/<script\b([^>]*)>/g)].map(match => attributes(match[1]).src).filter(Boolean);
  assert.deepEqual(sources, ['md5.js']);
  assert.equal(new URL(sources[0], 'http://10.5.50.1/login').pathname, '/md5.js');
  assert.match(html, /\$\(if chap-id\)<script src="md5\.js"><\/script>\$\(endif\)/);
});

test('actual CHAP script hashes raw ID + normalized code + raw challenge exactly once', () => {
  const inputs = [];
  const f = fixture({ helper: value => { inputs.push(value); return md5(value); } });
  assert.equal(f.submit(), false);
  assert.equal(f.submissions, 1);
  assert.deepEqual(inputs, [binaryID + 'ABCD-12_34' + binaryChallenge]);
  assert.equal(f.payloads[0].username, 'ABCD-12_34');
  assert.equal(f.payloads[0].password, md5(binaryID + 'ABCD-12_34' + binaryChallenge));
  assert.notEqual(f.payloads[0].password, md5(binaryID + md5(binaryChallenge) + 'ABCD-12_34'));
  assert.equal(f.payloads[0].dst, '$(link-orig)');
  assert.equal(f.payloads[0].popup, 'true');
});

test('actual PAP script submits the normalized code without requiring the CHAP helper', () => {
  const f = fixture({ chap: false, helper: null });
  assert.equal(f.submit(), false);
  assert.equal(f.submissions, 1);
  assert.equal(f.payloads[0].username, 'ABCD-12_34');
  assert.equal(f.payloads[0].password, 'ABCD-12_34');
});

test('repeated Enter/click submits once while waiting, then permits retry after no response', () => {
  const f = fixture();
  f.submit();
  f.submit();
  f.submit();
  assert.equal(f.submissions, 1);
  assert.equal(f.elements.get('voucher').disabled, true);
  assert.equal(f.elements.get('connect').disabled, true);
  assert.equal([...f.timers.values()][0].delay, 15000);
  f.expire();
  assert.equal(f.elements.get('voucher').disabled, false);
  assert.equal(f.elements.get('connect').disabled, false);
  assert.match(f.elements.get('login-error').textContent, /No login response/);
  assert.equal(f.elements.get('login-error').hidden, false);
  f.submit();
  assert.equal(f.submissions, 2);
  assert.equal(f.payloads[1].username, f.payloads[0].username);
});

test('invalid input does not authenticate or consume a pending attempt', () => {
  for (const code of ['', 'abc', 'ABCD?1234', 'ABCD 1234', 'A'.repeat(33)]) {
    const f = fixture({ code });
    f.submit();
    assert.equal(f.submissions, 0, `invalid code ${JSON.stringify(code)} must not submit`);
    assert.equal(f.timers.size, 0);
    assert.equal(f.elements.get('voucher').focused, true);
    assert.equal(f.elements.get('connect').disabled, false);
    f.elements.get('voucher').value = 'abcd1234';
    f.submit();
    assert.equal(f.submissions, 1);
  }
});

test('CHAP helper failure never downgrades to plaintext and permits retry after recovery', () => {
  for (const helper of [null, () => { throw new Error('broken helper'); }]) {
    const f = fixture({ helper });
    f.submit();
    assert.equal(f.submissions, 0);
    assert.equal(f.elements.get('connect').disabled, false);
    assert.match(f.elements.get('login-error').textContent, /Reload this page/);
    f.context.hexMD5 = md5;
    f.submit();
    assert.equal(f.submissions, 1);
    assert.notEqual(f.payloads[0].password, 'ABCD-12_34');
  }
});

test('synchronous submit failure and restored pages release the duplicate guard', () => {
  const failed = fixture({ submitError: true });
  failed.submit();
  assert.equal(failed.submissions, 0);
  assert.equal(failed.timers.size, 0);
  assert.equal(failed.elements.get('connect').disabled, false);
  assert.match(failed.elements.get('login-error').textContent, /Could not send your code/);
  const restored = fixture();
  restored.submit();
  restored.listeners.get('pageshow')();
  assert.equal(restored.timers.size, 0);
  assert.equal(restored.elements.get('connect').disabled, false);
  restored.submit();
  assert.equal(restored.submissions, 2);
});

test('pasted maximum-length code can be trimmed before enforcing the code length', () => {
  // Browser maxlength would truncate the end of a pasted code before trim runs.
  assert.doesNotMatch(visibleForm.body, /<input\b[^>]*id="voucher"[^>]*maxlength=/);
  const f = fixture({ code: '  ' + 'a'.repeat(32) + '  ' });
  f.submit();
  assert.equal(f.submissions, 1);
  assert.equal(f.payloads[0].username, 'A'.repeat(32));
});


test('router design preview cannot consume a code or submit a login', () => {
 const f=fixture({preview:true});f.submit();assert.equal(f.submissions,0);assert.equal(f.timers.size,0);assert.match(f.elements.get('login-error').textContent,/Preview only/);
});

test('required consent prevents router submission until accepted', () => {
 const f=fixture({consent:false});f.submit();assert.equal(f.submissions,0);assert.match(f.elements.get('login-error').textContent,/terms/);f.elements.get('voucher-consent').checked=true;f.submit();assert.equal(f.submissions,1);
});
