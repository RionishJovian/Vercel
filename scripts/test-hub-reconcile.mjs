import test from "node:test";
import assert from "node:assert/strict";
import { parseLive, parseSaved, planReconcile, reconcileHub } from "./hub-reconcile.mjs";

const addressesOf = (p) => [p.tunnelAddress];
const peer = (key, ip, state = "active", extra = {}) => ({ routerPublicKey: key, tunnelAddress: `10.77.0.${ip}/32`, state, ...extra });

test("reads live and saved peers", () => {
  assert.deepEqual([...parseLive("A\t10.77.0.2/32\nB\t(none)\n")], [["A", ["10.77.0.2/32"]], ["B", []]]);
  const conf = "[Interface]\nPrivateKey = hidden\n\n[Peer]\nPublicKey = A\nAllowedIPs = 10.77.0.2/32, 10.77.0.3/32\n\n[Peer]\nPublicKey = B\nAllowedIPs = 10.77.0.4/32\n";
  assert.deepEqual([...parseSaved(conf)], [["A", ["10.77.0.2/32", "10.77.0.3/32"]], ["B", ["10.77.0.4/32"]]]);
});

test("restores an active peer missing after a crash", () => {
  const plan = planReconcile({ peers: [peer("A", 2), peer("B", 3)], live: parseLive("A\t10.77.0.2/32\n"), addressesOf });
  assert.deepEqual(plan.add, [{ key: "B", addresses: ["10.77.0.3/32"] }]);
  assert.deepEqual(plan.remove, []);
});

test("corrects a peer on the wrong address", () => {
  const plan = planReconcile({ peers: [peer("A", 2)], live: parseLive("A\t10.77.0.9/32\n"), addressesOf });
  assert.deepEqual(plan.add, [{ key: "A", addresses: ["10.77.0.2/32"] }]);
});

test("removes a live peer the store marks removed or disabled", () => {
  const plan = planReconcile({ peers: [peer("A", 2, "removed"), peer("B", 3, "disabled")], live: parseLive("A\t10.77.0.2/32\nB\t10.77.0.3/32\n"), addressesOf });
  assert.deepEqual(plan.remove, ["A", "B"]);
});

test("keeps a key that an active peer also uses", () => {
  const plan = planReconcile({ peers: [peer("A", 2, "removed"), peer("A", 3)], live: parseLive("A\t10.77.0.3/32\n"), addressesOf });
  assert.deepEqual(plan.remove, []);
  assert.deepEqual(plan.add, []);
});

test("never removes a live peer the store does not know", () => {
  const plan = planReconcile({ peers: [peer("A", 2)], live: parseLive("A\t10.77.0.2/32\nZ\t10.77.0.50/32\n"), addressesOf });
  assert.deepEqual(plan.remove, []);
  assert.deepEqual(plan.unknown, ["Z"]);
});

test("leaves a peer in the middle of a migration alone", () => {
  const migrating = peer("A", 2, "active", { managementMigration: { phase: "staged" } });
  assert.deepEqual(planReconcile({ peers: [migrating], live: parseLive(""), addressesOf }).add, []);
  const finished = peer("A", 2, "active", { managementMigration: { phase: "completed" } });
  assert.equal(planReconcile({ peers: [finished], live: parseLive(""), addressesOf }).add.length, 1);
});

test("a peer still being enrolled is not touched", () => {
  const plan = planReconcile({ peers: [peer("A", 2, "provisioning")], live: parseLive(""), addressesOf });
  assert.deepEqual(plan, { add: [], remove: [], unknown: [] });
});

test("entries sharing a key become one peer holding all their addresses", () => {
  const plan = planReconcile({ peers: [peer("A", 2), peer("A", 3), peer("A", 4)], live: parseLive("A\t10.77.0.4/32\n"), addressesOf });
  assert.deepEqual(plan.add, [{ key: "A", addresses: ["10.77.0.2/32", "10.77.0.3/32", "10.77.0.4/32"] }]);
});

test("a shared key already holding all its addresses is left alone", () => {
  const plan = planReconcile({ peers: [peer("A", 2), peer("A", 3)], live: parseLive("A\t10.77.0.3/32 10.77.0.2/32\n"), addressesOf });
  assert.deepEqual(plan.add, []);
});

test("a shared key is left alone while any of its entries is migrating", () => {
  const migrating = peer("A", 2, "active", { managementMigration: { phase: "staged" } });
  assert.deepEqual(planReconcile({ peers: [migrating, peer("A", 3)], live: parseLive("A\t10.77.0.2/32\n"), addressesOf }).add, []);
});

function fakeHub({ peers, live, saved }) {
  const state = { live: new Map(live), saved, saves: 0 };
  const render = (map) => [...map].map(([k, a]) => `${k}\t${a.join(" ")}`).join("\n");
  const conf = (map) => "[Interface]\n" + [...map].map(([k, a]) => `[Peer]\nPublicKey = ${k}\nAllowedIPs = ${a.join(", ")}\n`).join("\n");
  return {
    state,
    args: {
      load: () => peers, addressesOf,
      readLive: () => render(state.live), readSaved: () => state.saved,
      setPeer: (k, a) => state.live.set(k, a), removePeer: (k) => state.live.delete(k),
      saveConfig: () => { state.saved = conf(state.live); state.saves++; },
    },
  };
}

test("after a crash: restores the missing peer and saves the file", () => {
  const hub = fakeHub({ peers: [peer("A", 2), peer("B", 3)], live: [["A", ["10.77.0.2/32"]]], saved: "[Interface]\n[Peer]\nPublicKey = A\nAllowedIPs = 10.77.0.2/32\n" });
  const result = reconcileHub(hub.args);
  assert.deepEqual(result, { added: 1, removed: 0, unknown: 0, saved: true });
  assert.ok(hub.state.saved.includes("PublicKey = B"));
});

test("saves a peer that is live but missing from the file", () => {
  const hub = fakeHub({ peers: [peer("A", 2)], live: [["A", ["10.77.0.2/32"]]], saved: "[Interface]\n" });
  assert.equal(reconcileHub(hub.args).saved, true);
  assert.equal(hub.state.saves, 1);
});

test("a shared key settles after one pass instead of changing every time", () => {
  const hub = fakeHub({ peers: [peer("A", 2), peer("A", 3), peer("A", 4), peer("B", 5)], live: [["A", ["10.77.0.4/32"]], ["B", ["10.77.0.5/32"]]], saved: "[Interface]\n[Peer]\nPublicKey = A\nAllowedIPs = 10.77.0.4/32\n\n[Peer]\nPublicKey = B\nAllowedIPs = 10.77.0.5/32\n" });
  assert.deepEqual(reconcileHub(hub.args), { added: 1, removed: 0, unknown: 0, saved: true });
  assert.deepEqual(hub.state.live.get("B"), ["10.77.0.5/32"]);
  assert.deepEqual(reconcileHub(hub.args), { added: 0, removed: 0, unknown: 0, saved: false });
  assert.equal(hub.state.saves, 1);
});

test("does nothing when everything already matches", () => {
  const hub = fakeHub({ peers: [peer("A", 2)], live: [["A", ["10.77.0.2/32"]]], saved: "[Interface]\n[Peer]\nPublicKey = A\nAllowedIPs = 10.77.0.2/32\n" });
  assert.deepEqual(reconcileHub(hub.args), { added: 0, removed: 0, unknown: 0, saved: false });
  assert.equal(hub.state.saves, 0);
});
