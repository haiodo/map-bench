package bench

// Участники, которых нет в upstream_bench_test.go: нагрузка и хелперы берутся оттуда,
// чтобы результаты легли в одну таблицу с upstream.

import (
	"fmt"
	"hash/maphash"
	"sync"
	"testing"

	csmap "github.com/mhmtszr/concurrent-swiss-map"
)

type mutexMap[K comparable] struct {
	mu sync.Mutex
	m  map[K]int
}

func (t *mutexMap[K]) load(k K) (int, bool) {
	t.mu.Lock()
	v, ok := t.m[k]
	t.mu.Unlock()
	return v, ok
}
func (t *mutexMap[K]) store(k K, v int) { t.mu.Lock(); t.m[k] = v; t.mu.Unlock() }
func (t *mutexMap[K]) del(k K)          { t.mu.Lock(); delete(t.m, k); t.mu.Unlock() }

type rwMap[K comparable] struct {
	mu sync.RWMutex
	m  map[K]int
}

func (t *rwMap[K]) load(k K) (int, bool) {
	t.mu.RLock()
	v, ok := t.m[k]
	t.mu.RUnlock()
	return v, ok
}
func (t *rwMap[K]) store(k K, v int) { t.mu.Lock(); t.m[k] = v; t.mu.Unlock() }
func (t *rwMap[K]) del(k K)          { t.mu.Lock(); delete(t.m, k); t.mu.Unlock() }
func (t *rwMap[K]) rangeFn(f func(K, int) bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for k, v := range t.m {
		if !f(k, v) {
			return
		}
	}
}

// shardedMap - "сделай сам": 256 шардов map+RWMutex, шард по maphash ключа; паддинг до
// кэш-линии, чтобы соседние мьютексы не делили одну линию.
const shardCount = 256

type shard[K comparable] struct {
	mu sync.RWMutex
	m  map[K]int
	_  [64 - 32]byte
}

type shardedMap[K comparable] struct {
	seed   maphash.Seed
	shards [shardCount]shard[K]
}

func newSharded[K comparable](size int) *shardedMap[K] {
	s := &shardedMap[K]{seed: maphash.MakeSeed()}
	for i := range s.shards {
		s.shards[i].m = make(map[K]int, size/shardCount)
	}
	return s
}

func (s *shardedMap[K]) shard(k K) *shard[K] {
	return &s.shards[maphash.Comparable(s.seed, k)%shardCount]
}

func (s *shardedMap[K]) load(k K) (int, bool) {
	sh := s.shard(k)
	sh.mu.RLock()
	v, ok := sh.m[k]
	sh.mu.RUnlock()
	return v, ok
}
func (s *shardedMap[K]) store(k K, v int) {
	sh := s.shard(k)
	sh.mu.Lock()
	sh.m[k] = v
	sh.mu.Unlock()
}
func (s *shardedMap[K]) del(k K) {
	sh := s.shard(k)
	sh.mu.Lock()
	delete(sh.m, k)
	sh.mu.Unlock()
}
func (s *shardedMap[K]) rangeFn(f func(K, int) bool) {
	for i := range s.shards {
		sh := &s.shards[i]
		sh.mu.RLock()
		for k, v := range sh.m {
			if !f(k, v) {
				sh.mu.RUnlock()
				return
			}
		}
		sh.mu.RUnlock()
	}
}

type fns[K comparable] struct {
	load  func(K) (int, bool)
	store func(K, int)
	del   func(K)
	rng   func(func(K, int) bool)
}

// extraImpls - фабрики участников; size - сколько ключей будет в карте (для предразмера).
func extraImpls[K comparable]() []struct {
	name string
	mk   func(size int) fns[K]
} {
	return []struct {
		name string
		mk   func(size int) fns[K]
	}{
		{"MutexMap", func(size int) fns[K] {
			t := &mutexMap[K]{m: make(map[K]int, size)}
			return fns[K]{load: t.load, store: t.store, del: t.del}
		}},
		{"RWMutexMap", func(size int) fns[K] {
			t := &rwMap[K]{m: make(map[K]int, size)}
			return fns[K]{load: t.load, store: t.store, del: t.del, rng: t.rangeFn}
		}},
		{"ShardedMap256", func(size int) fns[K] {
			t := newSharded[K](size)
			return fns[K]{load: t.load, store: t.store, del: t.del, rng: t.rangeFn}
		}},
		{"CSwissMap", func(size int) fns[K] {
			t := csmap.Create[K, int](csmap.WithSize[K, int](uint64(size)))
			return fns[K]{load: t.Load, store: t.Store, del: func(k K) { t.Delete(k) },
				rng: func(f func(K, int) bool) { t.Range(func(k K, v int) bool { return !f(k, v) }) }}
		}},
	}
}

func BenchmarkExtra_WarmUp_IntKeys(b *testing.B) {
	for _, im := range extraImpls[int]() {
		b.Run(im.name, func(b *testing.B) {
			for _, size := range benchmarkSizes {
				b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
					for _, bc := range benchmarkCases {
						b.Run(bc.name, func(b *testing.B) {
							f := im.mk(size)
							for i := range size {
								f.store(intKeys[i], i)
							}
							benchmarkIntKeys(b, size, f.load, f.store, f.del, bc.readPercentage)
						})
					}
				})
			}
		})
	}
}

func BenchmarkExtra_WarmUp_StringKeys(b *testing.B) {
	for _, im := range extraImpls[string]() {
		b.Run(im.name, func(b *testing.B) {
			for _, size := range benchmarkSizes {
				b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
					for _, bc := range benchmarkCases {
						b.Run(bc.name, func(b *testing.B) {
							f := im.mk(size)
							for i := range size {
								f.store(stringKeys[i], i)
							}
							benchmarkStringKeys(b, size, f.load, f.store, f.del, bc.readPercentage)
						})
					}
				})
			}
		})
	}
}

func BenchmarkExtra_Range_IntKeys(b *testing.B) {
	for _, im := range extraImpls[int]() {
		if im.mk(0).rng == nil {
			continue
		}
		b.Run(im.name, func(b *testing.B) {
			for _, size := range benchmarkSizes {
				b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
					f := im.mk(size)
					for i := range size {
						f.store(intKeys[i], i)
					}
					benchmarkRangeIntKeys(b, size, f.store, f.rng)
				})
			}
		})
	}
}

func BenchmarkExtra_Range_StringKeys(b *testing.B) {
	for _, im := range extraImpls[string]() {
		if im.mk(0).rng == nil {
			continue
		}
		b.Run(im.name, func(b *testing.B) {
			for _, size := range benchmarkSizes {
				b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
					f := im.mk(size)
					for i := range size {
						f.store(stringKeys[i], i)
					}
					benchmarkRangeStringKeys(b, size, f.store, f.rng)
				})
			}
		})
	}
}

// Проверка, что обёртки ведут себя как map: иначе бенчмарк мерил бы сломанную карту.
func TestExtraImplsMatchMap(t *testing.T) {
	for _, im := range extraImpls[int]() {
		f := im.mk(0)
		ref := map[int]int{}
		for i := range 20_000 {
			k := (i * 7919) % 1000
			switch i % 3 {
			case 0:
				f.store(k, i)
				ref[k] = i
			case 1:
				f.del(k)
				delete(ref, k)
			}
			want, wok := ref[k]
			if got, ok := f.load(k); ok != wok || got != want {
				t.Fatalf("%s: load(%d) = %d,%v; want %d,%v", im.name, k, got, ok, want, wok)
			}
		}
		if f.rng != nil {
			n := 0
			f.rng(func(int, int) bool { n++; return true })
			if n != len(ref) {
				t.Fatalf("%s: range %d, want %d", im.name, n, len(ref))
			}
		}
	}
}
