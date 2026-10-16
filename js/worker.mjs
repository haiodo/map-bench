// Одна пара (структура, ключи) на воркер: у воркера свой изолят и JIT,
// как у отдельного процесса в Node.
import { newWorkload, run } from "./harness.mjs";

self.onmessage = ({ data: { impl, keys, sizes, reps, minOps, label } }) => {
  const cfg = {
    runtime: label,
    reps,
    minOps,
    // gc в воркере есть только у Chromium с --js-flags=--expose-gc; без него память не меряем
    gc: typeof self.gc === "function" ? () => self.gc() : () => {},
    heapUsed: null,
    emit: (r) => self.postMessage({ result: r }),
  };
  for (const n of sizes) run(cfg, impl, keys, newWorkload(n, 42));
  self.postMessage({ done: true });
};
