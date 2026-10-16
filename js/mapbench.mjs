// CLI для Node (нужен --expose-gc) и Bun:
// node --expose-gc js/mapbench.mjs --impl js/Map --keys int --out results/raw/node.jsonl
import { appendFileSync } from "node:fs";
import { newWorkload, run } from "./harness.mjs";

const args = Object.fromEntries(
  process.argv.slice(2).reduce((acc, a, i, all) => (a.startsWith("--") ? [...acc, [a.slice(2), all[i + 1]]] : acc), []),
);
const impl = args.impl ?? "js/Map";
const keys = args.keys ?? "int";
const sizes = (args.sizes ?? "1000,10000,100000,1000000,10000000").split(",").map(Number);
const isBun = typeof Bun !== "undefined";

let gc;
let heapUsed = () => process.memoryUsage().heapUsed;
if (isBun) {
  gc = () => Bun.gc(true);
  // в Bun 1.3 heapUsed из process.memoryUsage() не растёт с кучей JSC
  const { heapStats } = await import("bun:jsc");
  heapUsed = () => heapStats().heapSize;
}
else if (typeof globalThis.gc === "function") gc = globalThis.gc;
else {
  console.error("нет gc(): запускай node с --expose-gc");
  process.exit(2);
}

const cfg = {
  runtime: args.label ?? (isBun ? `bun ${Bun.version}` : `node ${process.version}`),
  reps: Number(args.reps ?? 7),
  minOps: Number(args.minops ?? 1_000_000),
  gc,
  heapUsed,
  emit: (r) => {
    const line = JSON.stringify(r);
    if (args.out) appendFileSync(args.out, line + "\n");
    else console.log(line);
    console.error(`${r.impl.padEnd(28)} ${r.keys} n=${String(r.n).padEnd(9)} ${r.metric.padEnd(12)} ${r.median.toFixed(2).padStart(10)} ${r.unit} (min ${r.min.toFixed(2)})`);
  },
};
console.error(`runtime ${cfg.runtime}, impl ${impl}, keys ${keys}`);
for (const n of sizes) run(cfg, impl, keys, newWorkload(n, 42));
