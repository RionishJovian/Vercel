import fs from "node:fs";
import vm from "node:vm";
import assert from "node:assert/strict";
const source = fs.readFileSync(new URL("../web/app.html", import.meta.url), "utf8");
const policy = source.match(/function canOpenView[\s\S]*?\n}/)[0];
const context = vm.createContext({});
vm.runInContext(policy, context);
for (const role of ["OWNER","ADMIN","VIEWER","DEVELOPER"]) {
  const me = {user:{user_role:role}};
  assert.equal(context.canOpenView("platform",me), role === "DEVELOPER");
  assert.equal(context.canOpenView("i18n",me), role === "DEVELOPER");
  assert.equal(context.canOpenView("team",me), role !== "VIEWER");
  assert.equal(context.canOpenView("media",me), role !== "VIEWER");
  for (const view of ["gateways","routers","sites","security","backups"]) assert.equal(context.canOpenView(view,me),true);
}
assert.ok(source.includes('name === "gateways"'));
for (const view of ["coins","unlockkeys"]) {
  assert.equal(context.canOpenView(view,{user:{user_role:"OWNER"},subscription:{mode:"subscription_only"}}),false);
  assert.equal(context.canOpenView(view,{user:{user_role:"OWNER"},subscription:{mode:"legacy"}}),true);
}
assert.ok(source.includes('router.deployment_layer === "MAIN_GATEWAY"'));
assert.ok(source.includes('history.pushState({}, "", "/app.html#" + next)'), 'refresh must stay in the console');
assert.ok(!source.includes('"/#overview"'), 'overview must not route to the marketing page');
console.log("Navigation checks passed: tenant/admin visibility, gateway route and role filtering.");
