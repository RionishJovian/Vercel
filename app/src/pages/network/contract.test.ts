import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { initialValues, payloadValues, schema, choices, workspaces } from "./schema";

// The server accepts a fixed set of fields per task and rejects anything else with
// "unexpected network input". This reads that list from the Go source so the forms
// can never drift from it.
const goSource = readFileSync(join(__dirname, "../../../../internal/provisioner/routeros/network_plan.go"), "utf8");
const block = goSource.slice(goSource.indexOf("allowed := map[string]string{"));
const allowed: Record<string, string[]> = {};
for (const m of block.slice(0, block.indexOf("\n\t}")).matchAll(/"([a-z_]+)":\s*"([^"]*)"/g)) allowed[m[1]] = m[2].split(" ");

describe("network forms match the server's accepted fields", () => {
  it("found the server's list", () => { expect(Object.keys(allowed).length).toBeGreaterThan(15); expect(allowed.ip_address).toEqual(["address", "interface", "disabled"]); expect(allowed.vlan).toEqual(["name", "interface", "vlan-id", "disabled"]); });

  const kinds = [...new Set(workspaces.flatMap((w) => choices[w]))].filter((k) => !schema[k].readOnly && schema[k].fields.length > 0 && !["routes", "addresses"].includes(k));
  for (const kind of kinds) {
    it(`${kind}: a new record and an edited record only send fields the server accepts`, () => {
      expect(allowed[kind], `the server has no "${kind}" task`).toBeTruthy();
      const everything = { id: "*1", values: Object.fromEntries([...schema[kind].fields.map((f) => [f[0], "x"]), ["comment", "c"], ["dynamic", "no"], ["name", "n"], ["interface", "i"], ["distance", "2"]]) };
      for (const values of [initialValues(kind, undefined, {}), initialValues(kind, everything, {})]) {
        const sent = Object.keys(payloadValues(kind, values));
        expect(sent.filter((k) => !allowed[kind].includes(k)), `${kind} sends fields the server rejects`).toEqual([]);
      }
    });
    it(`${kind}: every field the form shows is accepted by the server`, () => {
      expect(schema[kind].fields.map((f) => f[0]).filter((k) => !allowed[kind].includes(k)), `${kind} shows fields the server rejects`).toEqual([]);
    });
  }

  it("never sends a record's other properties or the creation suggestions", () => {
    const edit = initialValues("lease", { id: "*1", values: { address: "1.2.3.4", "mac-address": "AA:BB", server: "dhcp1", comment: "c", dynamic: "no", disabled: "no" } }, {});
    expect(payloadValues("lease", edit)).toEqual({ address: "1.2.3.4", "mac-address": "AA:BB", server: "dhcp1", disabled: "no" });
    expect(Object.keys(payloadValues("lease", initialValues("lease", undefined, {})))).toEqual(["disabled"]);
  });
});
