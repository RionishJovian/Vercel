import { initialValues, listFor, type Inventory } from "./schema";

const inv: Inventory = { objects: {
  filters: [{ id: "a", values: { comment: "mm-protection-invalid-1", "in-interface": "ether1" } }, { id: "b", values: { comment: "custom rule" } }],
  queues: [{ id: "q1", values: { name: "mm-fair-guests", comment: "mm-fairshare-v1:x" } }, { id: "q2", values: { name: "vip" } }],
  leases: [{ id: "l1", values: { address: "1.1.1.1", dynamic: "yes" } }, { id: "l2", values: { address: "1.1.1.2", dynamic: "no" } }],
  bindings: [{ id: "b1", values: { type: "bypassed", disabled: "no", comment: "guest tv" } }, { id: "b2", values: { type: "bypassed", disabled: "no", comment: "mm-grace" } }, { id: "b3", values: { type: "regular" } }],
} };

describe("network task records", () => {
  it("offers only the app's own protection rules", () => { expect(listFor(inv, "firewall").map((o) => o.id)).toEqual(["a"]); });
  it("keeps fairshare and plain queues apart", () => { expect(listFor(inv, "fairshare").map((o) => o.id)).toEqual(["q1"]); expect(listFor(inv, "queue").map((o) => o.id)).toEqual(["q2"]); });
  it("hides dynamic DHCP leases", () => { expect(listFor(inv, "lease").map((o) => o.id)).toEqual(["l2"]); });
  it("only lists unmanaged bypasses for the shield", () => { expect(listFor(inv, "shield").map((o) => o.id)).toEqual(["b1"]); });
});

describe("network starting values", () => {
  it("suggests defaults for a new service, never applied without review", () => { expect(initialValues("pppoe_server", undefined, { gateway_cidr: "10.9.0.1/24" })).toMatchObject({ name: "subscribers", gateway_cidr: "10.9.0.1/24", dns: "1.1.1.1,8.8.8.8" }); });
  it("reads the protected port from the existing rule", () => { expect(initialValues("firewall", inv.objects!.filters[0], {}).interface).toBe("ether1"); });
  it("strips the managed prefix when editing fairshare", () => { expect(initialValues("fairshare", inv.objects!.queues[0], {}).name).toBe("guests"); });
});
