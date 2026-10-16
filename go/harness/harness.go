// Package harness - общая методика замеров для всех Go-реализаций и для JS-стенда (js/mapbench.mjs):
// одни и те же ключи (mulberry32), одни и те же операции, медиана из Reps прогонов.
// Любое изменение методики здесь нужно повторить в js/mapbench.mjs, иначе цифры несравнимы.
package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"time"
)

// IntMap - ассоциативный массив uint64 -> uint64. Ключи всегда < 2^31: в Node это SMI, но в Chromium с
// pointer compression SMI только до 2^30 - старшая половина ключей там HeapNumber (см. README).
type IntMap interface {
	Put(k, v uint64)
	Get(k uint64) (uint64, bool)
	Delete(k uint64) bool
	Len() int
}

// IntRanger - необязательный обход; у реализаций без обхода метрика iterate не снимается.
type IntRanger interface {
	Range(fn func(k, v uint64) bool)
}

type StrMap interface {
	Put(k string, v uint64)
	Get(k string) (uint64, bool)
	Delete(k string) bool
	Len() int
}

type StrRanger interface {
	Range(fn func(k string, v uint64) bool)
}

// Impl - реализация под замер; nil-фабрика значит, что этот тип ключей реализация не поддерживает.
type Impl struct {
	Name string
	Int  func(hint int) IntMap
	Str  func(hint int) StrMap
}

type Result struct {
	Runtime string  `json:"runtime"`
	Impl    string  `json:"impl"`
	Keys    string  `json:"keys"`
	N       int     `json:"n"`
	Metric  string  `json:"metric"`
	Median  float64 `json:"median"`
	Min     float64 `json:"min"`
	Unit    string  `json:"unit"`
	Ops     int     `json:"ops"`
	Reps    int     `json:"reps"`
}

// Config - параметры прогона. MinOps - минимум операций в одном замере: на малых N замер
// повторяет проход по ключам, чтобы таймер (и performance.now в браузере) мерил десятки мс.
type Config struct {
	Runtime string
	Reps    int
	MinOps  int
	Out     io.Writer // JSON lines
	Log     io.Writer // человекочитаемая строка на метрику
}

// mulberry32 - ГПСЧ, побитово совпадающий с JS-версией (Math.imul и >>>).
type mulberry32 struct{ a uint32 }

func (r *mulberry32) next() uint32 {
	r.a += 0x6D2B79F5
	t := (r.a ^ r.a>>15) * (r.a | 1)
	t = (t + (t^t>>7)*(t|61)) ^ t
	return t ^ t>>14
}

const (
	opPut = iota
	opGet
	opDelete
)

type Op struct {
	Kind uint8
	Idx  uint32 // индекс ключа: < n - в keys, иначе в miss (Idx-n)
}

// Workload - ключи и поток смешанных операций для одного N; одинаков для всех реализаций и языков.
type Workload struct {
	N     int
	Keys  []uint64
	Miss  []uint64
	SKeys []string
	SMiss []string
	Mixed []Op
}

// NewWorkload: n различных ключей в случайном порядке, n отсутствующих ключей и n смешанных
// операций (50% get, 30% put, 20% delete; четверть операций - по отсутствующим ключам).
func NewWorkload(n int, seed uint32) *Workload {
	r := &mulberry32{a: seed}
	seen := make(map[uint64]struct{}, 2*n)
	gen := func() []uint64 {
		out := make([]uint64, 0, n)
		for len(out) < n {
			k := uint64(r.next() >> 1)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, k)
		}
		return out
	}
	w := &Workload{N: n}
	w.Keys = gen()
	w.Miss = gen()
	w.SKeys = strKeys(w.Keys)
	w.SMiss = strKeys(w.Miss)
	w.Mixed = make([]Op, n)
	for i := range w.Mixed {
		kind := r.next() % 10
		idx := r.next() % uint32(n)
		if r.next()%4 == 0 {
			idx += uint32(n)
		}
		switch {
		case kind < 5:
			w.Mixed[i] = Op{opGet, idx}
		case kind < 8:
			w.Mixed[i] = Op{opPut, idx}
		default:
			w.Mixed[i] = Op{opDelete, idx}
		}
	}
	return w
}

func strKeys(keys []uint64) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = fmt.Sprintf("key-%010d", k)
	}
	return out
}

var sink uint64

// measure прогоняет setup (не в замере) и body Reps раз; возвращает медиану и минимум нс на операцию.
func measure(cfg Config, ops int, setup func(), body func()) (median, min float64) {
	ds := make([]float64, 0, cfg.Reps)
	for range cfg.Reps {
		if setup != nil {
			setup()
		}
		runtime.GC()
		start := time.Now()
		body()
		ds = append(ds, float64(time.Since(start).Nanoseconds())/float64(ops))
	}
	slices.Sort(ds)
	return ds[len(ds)/2], ds[0]
}

func passesFor(cfg Config, n int) int {
	return max(1, cfg.MinOps/n)
}

func (cfg Config) emit(r Result) {
	r.Runtime = cfg.Runtime
	b, _ := json.Marshal(r)
	fmt.Fprintln(cfg.Out, string(b))
	if cfg.Log != nil {
		fmt.Fprintf(cfg.Log, "%-28s %-3s n=%-9d %-12s %10.2f %s (min %.2f)\n", r.Impl, r.Keys, r.N, r.Metric, r.Median, r.Unit, r.Min)
	}
}

// RunInt снимает все метрики реализации на целых ключах.
func RunInt(cfg Config, impl Impl, w *Workload) {
	if impl.Int == nil {
		return
	}
	n, f, keys, miss := w.N, impl.Int, w.Keys, w.Miss
	p := passesFor(cfg, n)
	res := func(metric string, ops int, med, min float64) {
		cfg.emit(Result{Impl: impl.Name, Keys: "int", N: n, Metric: metric, Median: med, Min: min, Unit: "ns/op", Ops: ops, Reps: cfg.Reps})
	}
	build := func(hint int) IntMap {
		m := f(hint)
		for _, k := range keys {
			m.Put(k, k)
		}
		return m
	}

	for _, hint := range []int{0, n} {
		metric := "insert"
		if hint > 0 {
			metric = "insert_hint"
		}
		var last IntMap
		med, min := measure(cfg, p*n, nil, func() {
			for range p {
				last = build(hint)
			}
		})
		runtime.KeepAlive(last)
		res(metric, p*n, med, min)
	}

	m := build(0)
	for _, metric := range []string{"lookup_hit", "lookup_miss"} {
		src := keys
		if metric == "lookup_miss" {
			src = miss
		}
		med, min := measure(cfg, p*n, nil, func() {
			var s uint64
			for range p {
				for _, k := range src {
					if v, ok := m.Get(k); ok {
						s += v
					}
				}
			}
			sink += s
		})
		res(metric, p*n, med, min)
	}

	if rg, ok := m.(IntRanger); ok {
		med, min := measure(cfg, p*n, nil, func() {
			var s uint64
			for range p {
				rg.Range(func(k, v uint64) bool { s += v; return true })
			}
			sink += s
		})
		res("iterate", p*n, med, min)
	}
	m = nil

	var batch []IntMap
	setup := func() {
		batch = batch[:0]
		for range p {
			batch = append(batch, build(0))
		}
	}
	med, min := measure(cfg, p*n, setup, func() {
		for _, t := range batch {
			for _, k := range keys {
				t.Delete(k)
			}
		}
	})
	res("delete", p*n, med, min)

	med, min = measure(cfg, p*n, setup, func() {
		var s uint64
		for _, t := range batch {
			for _, o := range w.Mixed {
				k := keyOf(o.Idx, keys, miss)
				switch o.Kind {
				case opPut:
					t.Put(k, k)
				case opGet:
					if v, ok := t.Get(k); ok {
						s += v
					}
				case opDelete:
					t.Delete(k)
				}
			}
		}
		sink += s
	})
	res("mixed", p*n, med, min)
	batch = nil

	cfg.emit(Result{Impl: impl.Name, Keys: "int", N: n, Metric: "mem", Median: memPerElem(func() any { return build(0) }, n), Unit: "B/elem", Ops: n, Reps: 1})
}

// RunStr - то же на строковых ключах вида "key-0123456789".
func RunStr(cfg Config, impl Impl, w *Workload) {
	if impl.Str == nil {
		return
	}
	n, f, keys, miss := w.N, impl.Str, w.SKeys, w.SMiss
	p := passesFor(cfg, n)
	res := func(metric string, ops int, med, min float64) {
		cfg.emit(Result{Impl: impl.Name, Keys: "str", N: n, Metric: metric, Median: med, Min: min, Unit: "ns/op", Ops: ops, Reps: cfg.Reps})
	}
	build := func(hint int) StrMap {
		m := f(hint)
		for i, k := range keys {
			m.Put(k, w.Keys[i])
		}
		return m
	}

	for _, hint := range []int{0, n} {
		metric := "insert"
		if hint > 0 {
			metric = "insert_hint"
		}
		var last StrMap
		med, min := measure(cfg, p*n, nil, func() {
			for range p {
				last = build(hint)
			}
		})
		runtime.KeepAlive(last)
		res(metric, p*n, med, min)
	}

	m := build(0)
	for _, metric := range []string{"lookup_hit", "lookup_miss"} {
		src := keys
		if metric == "lookup_miss" {
			src = miss
		}
		med, min := measure(cfg, p*n, nil, func() {
			var s uint64
			for range p {
				for _, k := range src {
					if v, ok := m.Get(k); ok {
						s += v
					}
				}
			}
			sink += s
		})
		res(metric, p*n, med, min)
	}

	if rg, ok := m.(StrRanger); ok {
		med, min := measure(cfg, p*n, nil, func() {
			var s uint64
			for range p {
				rg.Range(func(k string, v uint64) bool { s += v; return true })
			}
			sink += s
		})
		res("iterate", p*n, med, min)
	}
	m = nil

	var batch []StrMap
	setup := func() {
		batch = batch[:0]
		for range p {
			batch = append(batch, build(0))
		}
	}
	med, min := measure(cfg, p*n, setup, func() {
		for _, t := range batch {
			for _, k := range keys {
				t.Delete(k)
			}
		}
	})
	res("delete", p*n, med, min)

	med, min = measure(cfg, p*n, setup, func() {
		var s uint64
		for _, t := range batch {
			for _, o := range w.Mixed {
				k := keyOf(o.Idx, keys, miss)
				switch o.Kind {
				case opPut:
					t.Put(k, uint64(o.Idx))
				case opGet:
					if v, ok := t.Get(k); ok {
						s += v
					}
				case opDelete:
					t.Delete(k)
				}
			}
		}
		sink += s
	})
	res("mixed", p*n, med, min)
	batch = nil

	cfg.emit(Result{Impl: impl.Name, Keys: "str", N: n, Metric: "mem", Median: memPerElem(func() any { return build(0) }, n), Unit: "B/elem", Ops: n, Reps: 1})
}

func keyOf[K any](idx uint32, keys, miss []K) K {
	if int(idx) < len(keys) {
		return keys[idx]
	}
	return miss[int(idx)-len(keys)]
}

// memPerElem - прирост живой кучи после построения структуры, на элемент. Ключи строкового
// варианта уже лежат в Workload, поэтому в замер попадают только заголовки строк, не их байты.
func memPerElem(build func() any, n int) float64 {
	runtime.GC()
	runtime.GC()
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	m := build()
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&b)
	runtime.KeepAlive(m)
	return float64(int64(b.HeapAlloc)-int64(a.HeapAlloc)) / float64(n)
}

// Stderr - лог по умолчанию.
var Stderr io.Writer = os.Stderr
