#!/usr/bin/env python3
"""Сводит results/raw/*.jsonl и C-cpu*.txt в Markdown-таблицы results/tables.md (только stdlib)."""
import glob
import json
import os
import re
import statistics
import sys
from collections import defaultdict

RAW = "results/raw"
METRICS = ["insert", "insert_hint", "lookup_hit", "lookup_miss", "iterate", "delete", "mixed", "mem"]


def load(name):
    path = os.path.join(RAW, name)
    if not os.path.exists(path):
        return []
    with open(path) as f:
        return [json.loads(line) for line in f if line.strip()]


def fmt(v):
    if v is None:
        return "-"
    return f"{v:.1f}" if v >= 10 else f"{v:.2f}"


def index(rows):
    # браузер пишет по строке на каждый запуск Worker - сводим их медианой медиан
    groups = defaultdict(list)
    for r in rows:
        groups[(r["runtime"], r["impl"], r["keys"], r["n"], r["metric"])].append(r["median"])
    return {k: statistics.median(v) for k, v in groups.items()}


def table(header, lines):
    out = ["| " + " | ".join(header) + " |", "|" + "---|" * len(header)]
    out += ["| " + " | ".join(line) + " |" for line in lines]
    return "\n".join(out)


def phase_a(rows, out):
    if not rows:
        return
    ix = index(rows)
    runtimes = sorted({r["runtime"] for r in rows}, key=lambda s: (s.split()[0], "X:" not in s))
    sizes = sorted({r["n"] for r in rows})
    base = runtimes[0]
    out.append("## A. Встроенный map Go по версиям\n")
    out.append(f"ns/op (mem - B/elem), медиана; в скобках - изменение к {base}.\n")
    for keys in ["int", "str"]:
        for n in sizes:
            lines = []
            for m in METRICS:
                b = ix.get((base, "go/map", keys, n, m))
                cells = [m]
                for rt in runtimes:
                    v = ix.get((rt, "go/map", keys, n, m))
                    cell = fmt(v)
                    if v is not None and b and rt != base:
                        cell += f" ({(v / b - 1) * 100:+.0f}%)"
                    cells.append(cell)
                lines.append(cells)
            out.append(f"### keys={keys}, n={n}\n")
            out.append(table(["metric"] + runtimes, lines) + "\n")


def phase_b(rows, out, title, ref_rows=()):
    if not rows:
        return
    rows = list(rows) + list(ref_rows)
    ix = index(rows)
    impls = []
    for r in rows:
        if r["impl"] not in impls:
            impls.append(r["impl"])
    sizes = sorted({r["n"] for r in rows})
    out.append(f"## {title}\n")
    for keys in ["int", "str"]:
        for n in sizes:
            lines = []
            for (rt, impl) in sorted({(r["runtime"], r["impl"]) for r in rows if r["keys"] == keys and r["n"] == n},
                                     key=lambda x: (impls.index(x[1]), x[0])):
                lines.append([impl, rt] + [fmt(ix.get((rt, impl, keys, n, m))) for m in METRICS])
            if lines:
                out.append(f"### keys={keys}, n={n}\n")
                out.append(table(["impl", "runtime"] + METRICS, lines) + "\n")


# при -cpu 1 go test не дописывает к имени суффикс "-1"
BENCH = re.compile(r"^Benchmark(\S+?)(?:-(\d+))?\s+\d+\s+([\d.]+) ns/op\s+([\d.]+) ops/s")


def parse_name(name):
    """SyncMap_WarmUp_IntKeys/size=1000/reads=75% и Extra_WarmUp_IntKeys/MutexMap/size=1000/reads=75%
    -> (impl, case, {"size": "1000", "reads": "75%"})."""
    parts = name.split("/")
    if parts[0].startswith("Extra_"):
        impl, case, rest = parts[1], parts[0][len("Extra_"):], parts[2:]
    else:
        impl, case = parts[0].split("_", 1)
        rest = parts[1:]
    return impl, case, dict(p.split("=", 1) for p in rest)


def phase_c(out):
    files = sorted(glob.glob(os.path.join(RAW, "C-cpu*.txt")))
    if not files:
        return
    data = defaultdict(list)  # (impl, case, size, reads, cpu) -> [(ns/op, ops/s)]
    for path in files:
        with open(path) as f:
            for line in f:
                m = BENCH.match(line)
                if not m:
                    continue
                impl, case, p = parse_name(m.group(1))
                key = (impl, case, p.get("size"), p.get("reads", ""), int(m.group(2) or 1))
                data[key].append((float(m.group(3)), float(m.group(4))))
    cpus = sorted({k[4] for k in data})
    impls = sorted({k[0] for k in data})
    out.append("## C. Конкурентные карты (медиана из -count)\n")
    out.append("WarmUp/NoWarmUp - млн ops/s на всю программу; Range - нс на элемент: проходы идут в RunParallel параллельно с одним писателем, так что "
               "это пропускная способность всех горутин, а не задержка одного прохода.\n")
    for case in ["WarmUp_IntKeys", "WarmUp_StringKeys", "NoWarmUp_IntKeys", "Range_IntKeys", "Range_StringKeys"]:
        for size in ["1000", "1000000"]:
            for reads in (["100%", "99%", "90%", "75%"] if "Range" not in case else [""]):
                lines = []
                for impl in impls:
                    cells = [impl]
                    for c in cpus:
                        vs = data.get((impl, case, size, reads, c))
                        if not vs:
                            cells.append("-")
                        elif "Range" in case:
                            cells.append(fmt(statistics.median(v[0] for v in vs) / int(size)))
                        else:
                            cells.append(f"{statistics.median(v[1] for v in vs) / 1e6:.1f}")
                    if any(cell != "-" for cell in cells[1:]):
                        lines.append(cells)
                if lines:
                    out.append(f"### {case}, size={size}" + (f", reads={reads}" if reads else "") + "\n")
                    out.append(table(["impl"] + [f"cpu={c}" for c in cpus], lines) + "\n")


def main():
    out = ["# Таблицы\n", "Сгенерировано parse.py из results/raw. Окружение - results/env.txt.\n"]
    phase_a(load("A-builtin.jsonl"), out)
    b = load("B-schemes.jsonl")
    phase_b(b, out, "B. Схемы и библиотеки (Go 1.27)")
    go_ref = [r for r in b if r["impl"] == "go/map"]
    phase_b(load("D-js.jsonl") + load("D-browser.jsonl"), out, "D. JS (Node, Bun, Safari, Brave) против go/map", go_ref)
    phase_c(out)
    text = "\n".join(out)
    with open("results/tables.md", "w") as f:
        f.write(text)
    if "-q" not in sys.argv:
        print(text)


if __name__ == "__main__":
    main()
