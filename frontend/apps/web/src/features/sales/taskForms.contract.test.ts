import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { load } from "js-yaml";
import { describe, expect, it } from "vitest";
import { taskForms } from "./taskForms";

/**
 * The task forms are written by hand, the backend's data objects are not: this checks that every field a form sends
 * exists in the backend's OpenAPI document (a misspelt key is a 400 at runtime), and that no field the backend
 * accepts is missing from its form. It reads ../backend/api/openapi.yaml, so it only runs inside the monorepo.
 */
// apps/web/src/features/sales → the repository root, where backend/ sits next to frontend/.
const file = resolve(__dirname, "../../../../../../backend/api/openapi.yaml");
type Schema = { properties?: Record<string, { $ref?: string }> };

const spec = existsSync(file)
  ? (load(readFileSync(file, "utf8")) as {
      components: { schemas: Record<string, Schema> };
    })
  : undefined;

const schemaOf: Record<number, string> = {
  1: "Task1Data",
  2: "Task2Data",
  3: "Task3Data",
  4: "PaymentData",
  5: "PaymentData",
  6: "Task6Data",
  8: "Task8Data",
  9: "Task9Data",
  10: "Task10Data",
  11: "Task11Data",
  12: "Task12Data",
  13: "Task13Data",
};

/** Fields the backend returns but a person never types: they come from the server. */
const serverOnly: Record<string, string[]> = {
  PaymentData: ["expectedAmount"],
  InvoiceData: ["expectedAmount", "amountsMatch"],
};

const describeIf = spec ? describe : describe.skip;

describeIf("task forms against the OpenAPI contract", () => {
  for (const [n, schemaName] of Object.entries(schemaOf)) {
    it(`task ${n} sends exactly the fields ${schemaName} accepts`, () => {
      const schemas = spec!.components.schemas;
      const sent = new Set<string>();
      for (const group of taskForms[Number(n)] ?? []) {
        for (const def of group.fields) {
          if (def.readOnly) continue;
          sent.add(group.prefix ? `${group.prefix}.${def.key}` : def.key);
        }
      }
      const accepted = new Set<string>();
      for (const [key, prop] of Object.entries(
        schemas[schemaName].properties ?? {},
      )) {
        const nested = prop.$ref
          ? schemas[prop.$ref.split("/").pop()!]
          : undefined;
        if (nested?.properties && ["Task6Data"].includes(schemaName)) {
          for (const inner of Object.keys(nested.properties))
            if (!serverOnly.InvoiceData.includes(inner))
              accepted.add(`${key}.${inner}`);
        } else if (!(serverOnly[schemaName] ?? []).includes(key))
          accepted.add(key);
      }
      expect([...sent].sort()).toEqual([...accepted].sort());
    });
  }

  it("task 7 has no data of its own", () => {
    expect(taskForms[7] ?? []).toEqual([]);
  });
});
