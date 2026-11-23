import { newWorkload } from "../js/harness.mjs";
import { writeFileSync } from "node:fs";
const n = Number(process.argv[2] ?? 1_000_000);
const w = newWorkload(n, 42);
const keys = new Uint32Array(2 * n);
keys.set(w.keys);
keys.set(w.miss, n);
const trace = new Uint32Array(n);
const maps = [new Map(), new Map()];
let hit = 0;
for (let i = 0; i < n; i++) {
  hit += keys[i];
  for (const m of maps) m.set(i, keys[i]);
}
const mixed = [0, 0];
for (let i = 0; i < n; i++) {
  const op = w.kinds[i];
  const j = w.idxs[i];
  trace[i] = 4 * j + op;
  maps.forEach((m, t) => {
    if (op === 0) m.set(j, t ? j : keys[j]);
    if (op === 1) mixed[t] += m.get(j) ?? 0;
    if (op === 2) m.delete(j);
  });
}
const header = new Uint32Array([n, maps[0].size]);
const sums = new BigUint64Array([hit, ...mixed].map(BigInt));
writeFileSync("workload.bin", Buffer.concat([header,sums,keys,trace].map(a => Buffer.from(a.buffer))));
