package main

import (
	"cmp"
	"encoding/binary"

	cswiss "github.com/cockroachdb/swiss"
	dswiss "github.com/dolthub/swiss"
	gbtree "github.com/google/btree"
	"github.com/huandu/skiplist"
	art "github.com/plar/go-adaptive-radix-tree"
	tbtree "github.com/tidwall/btree"
	"github.com/tidwall/hashmap"

	"hashcmp/ht"

	"github.com/haiodo/map-bench/go/harness"
)

// impls - всё, что меряем. Обёртки только приводят API к harness.IntMap/StrMap и ничего не
// оптимизируют: стоимость API (boxing в interface{}, []byte-ключи у ART) - часть результата.
func impls() []harness.Impl {
	out := []harness.Impl{
		{Name: "go/map", Int: func(h int) harness.IntMap { return newBuiltin[uint64](h) }, Str: func(h int) harness.StrMap { return newBuiltin[string](h) }},
		{Name: "dolthub/swiss", Int: func(h int) harness.IntMap { return newDolt[uint64](h) }, Str: func(h int) harness.StrMap { return newDolt[string](h) }},
		{Name: "cockroachdb/swiss", Int: func(h int) harness.IntMap { return newRoach[uint64](h) }, Str: func(h int) harness.StrMap { return newRoach[string](h) }},
		{Name: "tidwall/hashmap", Int: func(h int) harness.IntMap { return newTidHash[uint64](h) }, Str: func(h int) harness.StrMap { return newTidHash[string](h) }},
		{Name: "tidwall/btree", Int: func(int) harness.IntMap { return newTidBtree[uint64]() }, Str: func(int) harness.StrMap { return newTidBtree[string]() }},
		{Name: "google/btree", Int: func(int) harness.IntMap { return newGBtree[uint64]() }, Str: func(int) harness.StrMap { return newGBtree[string]() }},
		{Name: "huandu/skiplist", Int: func(int) harness.IntMap { return &skipInt{skiplist.New(skiplist.Uint64)} }, Str: func(int) harness.StrMap { return &skipStr{skiplist.New(skiplist.String)} }},
		{Name: "plar/art", Int: func(int) harness.IntMap { return &artInt{art.New()} }, Str: func(int) harness.StrMap { return &artStr{art.New()} }},
	}
	// Схемы из go-hashtable-shootout - только uint64-ключи и своя общая хеш-функция (fmix64).
	for _, name := range ht.Names {
		if name == "builtin" {
			continue
		}
		f := ht.Implementations[name]
		out = append(out, harness.Impl{Name: "shootout/" + name, Int: func(h int) harness.IntMap { return f(h) }})
	}
	return out
}

type builtin[K comparable] struct{ m map[K]uint64 }

func newBuiltin[K comparable](h int) *builtin[K] { return &builtin[K]{make(map[K]uint64, h)} }
func (t *builtin[K]) Put(k K, v uint64)          { t.m[k] = v }
func (t *builtin[K]) Get(k K) (uint64, bool)     { v, ok := t.m[k]; return v, ok }

// Delete без предварительного поиска: так map используют в обычном коде, результат харнессу не нужен.
func (t *builtin[K]) Delete(k K) bool { delete(t.m, k); return true }
func (t *builtin[K]) Len() int        { return len(t.m) }
func (t *builtin[K]) Range(fn func(K, uint64) bool) {
	for k, v := range t.m {
		if !fn(k, v) {
			return
		}
	}
}

type dolt[K comparable] struct{ m *dswiss.Map[K, uint64] }

func newDolt[K comparable](h int) *dolt[K] { return &dolt[K]{dswiss.NewMap[K, uint64](uint32(h))} }
func (t *dolt[K]) Put(k K, v uint64)       { t.m.Put(k, v) }
func (t *dolt[K]) Get(k K) (uint64, bool)  { return t.m.Get(k) }
func (t *dolt[K]) Delete(k K) bool         { return t.m.Delete(k) }
func (t *dolt[K]) Len() int                { return t.m.Count() }
func (t *dolt[K]) Range(fn func(K, uint64) bool) {
	t.m.Iter(func(k K, v uint64) bool { return !fn(k, v) })
}

type roach[K comparable] struct{ m *cswiss.Map[K, uint64] }

func newRoach[K comparable](h int) *roach[K]      { return &roach[K]{cswiss.New[K, uint64](h)} }
func (t *roach[K]) Put(k K, v uint64)             { t.m.Put(k, v) }
func (t *roach[K]) Get(k K) (uint64, bool)        { return t.m.Get(k) }
func (t *roach[K]) Delete(k K) bool               { t.m.Delete(k); return true }
func (t *roach[K]) Len() int                      { return t.m.Len() }
func (t *roach[K]) Range(fn func(K, uint64) bool) { t.m.All(fn) }

type tidHash[K comparable] struct{ m *hashmap.Map[K, uint64] }

func newTidHash[K comparable](h int) *tidHash[K]    { return &tidHash[K]{hashmap.New[K, uint64](h)} }
func (t *tidHash[K]) Put(k K, v uint64)             { t.m.Set(k, v) }
func (t *tidHash[K]) Get(k K) (uint64, bool)        { return t.m.Get(k) }
func (t *tidHash[K]) Delete(k K) bool               { _, ok := t.m.Delete(k); return ok }
func (t *tidHash[K]) Len() int                      { return t.m.Len() }
func (t *tidHash[K]) Range(fn func(K, uint64) bool) { t.m.Scan(fn) }

type tidBtree[K cmp.Ordered] struct{ m *tbtree.Map[K, uint64] }

func newTidBtree[K cmp.Ordered]() *tidBtree[K]       { return &tidBtree[K]{tbtree.NewMap[K, uint64](0)} }
func (t *tidBtree[K]) Put(k K, v uint64)             { t.m.Set(k, v) }
func (t *tidBtree[K]) Get(k K) (uint64, bool)        { return t.m.Get(k) }
func (t *tidBtree[K]) Delete(k K) bool               { _, ok := t.m.Delete(k); return ok }
func (t *tidBtree[K]) Len() int                      { return t.m.Len() }
func (t *tidBtree[K]) Range(fn func(K, uint64) bool) { t.m.Scan(fn) }

type kv[K cmp.Ordered] struct {
	k K
	v uint64
}

type gBtree[K cmp.Ordered] struct{ t *gbtree.BTreeG[kv[K]] }

// Степень 32 - значение из README google/btree для деревьев в памяти.
func newGBtree[K cmp.Ordered]() *gBtree[K] {
	return &gBtree[K]{gbtree.NewG(32, func(a, b kv[K]) bool { return a.k < b.k })}
}
func (t *gBtree[K]) Put(k K, v uint64) { t.t.ReplaceOrInsert(kv[K]{k, v}) }
func (t *gBtree[K]) Get(k K) (uint64, bool) {
	it, ok := t.t.Get(kv[K]{k: k})
	return it.v, ok
}
func (t *gBtree[K]) Delete(k K) bool { _, ok := t.t.Delete(kv[K]{k: k}); return ok }
func (t *gBtree[K]) Len() int        { return t.t.Len() }
func (t *gBtree[K]) Range(fn func(K, uint64) bool) {
	t.t.Ascend(func(it kv[K]) bool { return fn(it.k, it.v) })
}

type skipInt struct{ l *skiplist.SkipList }

func (t *skipInt) Put(k, v uint64) { t.l.Set(k, v) }
func (t *skipInt) Get(k uint64) (uint64, bool) {
	v, ok := t.l.GetValue(k)
	if !ok {
		return 0, false
	}
	return v.(uint64), true
}
func (t *skipInt) Delete(k uint64) bool { return t.l.Remove(k) != nil }
func (t *skipInt) Len() int             { return t.l.Len() }
func (t *skipInt) Range(fn func(uint64, uint64) bool) {
	for e := t.l.Front(); e != nil; e = e.Next() {
		if !fn(e.Key().(uint64), e.Value.(uint64)) {
			return
		}
	}
}

type skipStr struct{ l *skiplist.SkipList }

func (t *skipStr) Put(k string, v uint64) { t.l.Set(k, v) }
func (t *skipStr) Get(k string) (uint64, bool) {
	v, ok := t.l.GetValue(k)
	if !ok {
		return 0, false
	}
	return v.(uint64), true
}
func (t *skipStr) Delete(k string) bool { return t.l.Remove(k) != nil }
func (t *skipStr) Len() int             { return t.l.Len() }
func (t *skipStr) Range(fn func(string, uint64) bool) {
	for e := t.l.Front(); e != nil; e = e.Next() {
		if !fn(e.Key().(string), e.Value.(uint64)) {
			return
		}
	}
}

// ART хранит []byte-ключи; big-endian сохраняет порядок чисел.
type artInt struct{ t art.Tree }

func artKey(k uint64) art.Key { return binary.BigEndian.AppendUint64(make([]byte, 0, 8), k) }

func (t *artInt) Put(k, v uint64) { t.t.Insert(artKey(k), v) }
func (t *artInt) Get(k uint64) (uint64, bool) {
	v, ok := t.t.Search(artKey(k))
	if !ok {
		return 0, false
	}
	return v.(uint64), true
}
func (t *artInt) Delete(k uint64) bool { _, ok := t.t.Delete(artKey(k)); return ok }
func (t *artInt) Len() int             { return t.t.Size() }
func (t *artInt) Range(fn func(uint64, uint64) bool) {
	t.t.ForEach(func(n art.Node) bool { return fn(binary.BigEndian.Uint64(n.Key()), n.Value().(uint64)) })
}

type artStr struct{ t art.Tree }

func (t *artStr) Put(k string, v uint64) { t.t.Insert(art.Key(k), v) }
func (t *artStr) Get(k string) (uint64, bool) {
	v, ok := t.t.Search(art.Key(k))
	if !ok {
		return 0, false
	}
	return v.(uint64), true
}
func (t *artStr) Delete(k string) bool { _, ok := t.t.Delete(art.Key(k)); return ok }
func (t *artStr) Len() int             { return t.t.Size() }
func (t *artStr) Range(fn func(string, uint64) bool) {
	t.t.ForEach(func(n art.Node) bool { return fn(string(n.Key()), n.Value().(uint64)) })
}
