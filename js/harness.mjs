// Копия методики go/harness/harness.go (ключи mulberry32, операции, медиана из reps):
// поменял методику там - поменяй здесь, иначе цифры несравнимы.

export function mulberry32(seed) {
  let a = seed | 0;
  return function () {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), a | 1);
    t = (t + Math.imul(t ^ (t >>> 7), t | 61)) ^ t;
    return (t ^ (t >>> 14)) >>> 0;
  };
}

const OP_PUT = 0, OP_GET = 1, OP_DELETE = 2;

export function newWorkload(n, seed) {
  const next = mulberry32(seed);
  // V8 ограничивает Set 2^24 элементами, а при N = 10 млн здесь 2N ключей - делим на 16 Set
  const seen = Array.from({ length: 16 }, () => new Set());
  const gen = () => {
    const out = new Array(n);
    let i = 0;
    while (i < n) {
      const k = next() >>> 1;
      const s = seen[k & 15];
      if (s.has(k)) continue;
      s.add(k);
      out[i++] = k;
    }
    return out;
  };
  const keys = gen();
  const miss = gen();
  const skey = (k) => "key-" + String(k).padStart(10, "0");
  const kinds = new Uint8Array(n);
  const idxs = new Uint32Array(n);
  for (let i = 0; i < n; i++) {
    const kind = next() % 10;
    let idx = next() % n;
    if (next() % 4 === 0) idx += n;
    kinds[i] = kind < 5 ? OP_GET : kind < 8 ? OP_PUT : OP_DELETE;
    idxs[i] = idx;
  }
  return { n, keys, miss, skeys: keys.map(skey), smiss: miss.map(skey), kinds, idxs };
}

// Реализации: класс-обёртка с put/get/del/size/range. Значение - число (ключ или индекс операции).
class JsMap {
  constructor() { this.m = new Map(); }
  put(k, v) { this.m.set(k, v); }
  get(k) { return this.m.get(k); }
  del(k) { this.m.delete(k); }
  size() { return this.m.size; }
  range(fn) { for (const v of this.m.values()) fn(v); }
}

// Object с целыми ключами V8 хранит как elements (индексы массива), со строковыми - как свойства;
// это разные механизмы, поэтому меряем оба.
class JsObject {
  constructor() { this.o = {}; this.n = 0; }
  put(k, v) { if (this.o[k] === undefined) this.n++; this.o[k] = v; }
  get(k) { return this.o[k]; }
  del(k) { if (this.o[k] !== undefined) { delete this.o[k]; this.n--; } }
  size() { return this.n; }
  range(fn) { const o = this.o; for (const k in o) fn(o[k]); }
}

export const impls = { "js/Map": JsMap, "js/Object": JsObject };

let sink = 0;

function median(xs) {
  xs.sort((a, b) => a - b);
  return xs[xs.length >> 1];
}

function measure(cfg, ops, setup, body) {
  const ds = [];
  for (let r = 0; r < cfg.reps; r++) {
    if (setup) setup();
    cfg.gc();
    const t0 = performance.now();
    body();
    ds.push(((performance.now() - t0) * 1e6) / ops);
  }
  const min = Math.min(...ds);
  return [median(ds), min];
}

// Один процесс (или вкладка) - одна реализация и один тип ключей: иначе call site в циклах
// становится полиморфным, и JIT мерит сам себя, а не структуру.
export function run(cfg, implName, keyType, w) {
  const Impl = impls[implName];
  const n = w.n;
  const keys = keyType === "str" ? w.skeys : w.keys;
  const miss = keyType === "str" ? w.smiss : w.miss;
  const p = Math.max(1, Math.floor(cfg.minOps / n));
  const ops = p * n;
  const res = (metric, med, min, unit = "ns/op", o = ops, reps = cfg.reps) =>
    cfg.emit({ runtime: cfg.runtime, impl: implName, keys: keyType, n, metric, median: med, min, unit, ops: o, reps });
  const build = () => {
    const m = new Impl();
    for (let i = 0; i < n; i++) m.put(keys[i], w.keys[i]);
    return m;
  };

  // У JS-коллекций нет подсказки размера, поэтому insert_hint не снимается.
  let last;
  let [med, min] = measure(cfg, ops, null, () => { for (let j = 0; j < p; j++) last = build(); });
  res("insert", med, min);
  last = null;

  let m = build();
  for (const metric of ["lookup_hit", "lookup_miss"]) {
    const src = metric === "lookup_miss" ? miss : keys;
    [med, min] = measure(cfg, ops, null, () => {
      let s = 0;
      for (let j = 0; j < p; j++) {
        for (let i = 0; i < n; i++) {
          const v = m.get(src[i]);
          if (v !== undefined) s += v;
        }
      }
      sink += s;
    });
    res(metric, med, min);
  }

  [med, min] = measure(cfg, ops, null, () => {
    let s = 0;
    for (let j = 0; j < p; j++) m.range((v) => { s += v; });
    sink += s;
  });
  res("iterate", med, min);
  m = null;

  let batch = [];
  const setup = () => { batch = []; for (let j = 0; j < p; j++) batch.push(build()); };
  [med, min] = measure(cfg, ops, setup, () => {
    for (const t of batch) for (let i = 0; i < n; i++) t.del(keys[i]);
  });
  res("delete", med, min);

  const { kinds, idxs } = w;
  [med, min] = measure(cfg, ops, setup, () => {
    let s = 0;
    for (const t of batch) {
      for (let i = 0; i < n; i++) {
        const idx = idxs[i];
        const k = idx < n ? keys[idx] : miss[idx - n];
        const kind = kinds[i];
        if (kind === OP_PUT) t.put(k, keyType === "str" ? idx : k);
        else if (kind === OP_GET) { const v = t.get(k); if (v !== undefined) s += v; }
        else t.del(k);
      }
    }
    sink += s;
  });
  res("mixed", med, min);
  batch = null;

  const heap = cfg.heapUsed;
  if (heap) {
    cfg.gc(); cfg.gc();
    const a = heap();
    const mm = build();
    cfg.gc(); cfg.gc();
    const b = heap();
    res("mem", (b - a) / n, 0, "B/elem", n, 1);
    if (mm.size() !== n) throw new Error("size mismatch");
  }
  return sink;
}
