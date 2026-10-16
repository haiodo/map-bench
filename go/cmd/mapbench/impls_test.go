package main

import (
	"math/rand"
	"strconv"
	"testing"
)

// Каждая обёртка ведёт себя как встроенный map на случайном потоке put/get/delete:
// иначе бенчмарк мог бы мерить сломанную реализацию.
func TestImplsMatchMap(t *testing.T) {
	for _, im := range impls() {
		t.Run(im.Name, func(t *testing.T) {
			r := rand.New(rand.NewSource(1))
			ref := map[uint64]uint64{}
			ints := im.Int(0)
			var strs interface {
				Put(string, uint64)
				Get(string) (uint64, bool)
				Delete(string) bool
				Len() int
			}
			if im.Str != nil {
				strs = im.Str(0)
			}
			for i := range 50_000 {
				k := uint64(r.Intn(5_000))
				sk := strconv.FormatUint(k, 10)
				switch r.Intn(3) {
				case 0:
					ref[k] = uint64(i)
					ints.Put(k, uint64(i))
					if strs != nil {
						strs.Put(sk, uint64(i))
					}
				case 1:
					delete(ref, k)
					ints.Delete(k)
					if strs != nil {
						strs.Delete(sk)
					}
				default:
					want, wok := ref[k]
					if got, ok := ints.Get(k); ok != wok || got != want {
						t.Fatalf("int Get(%d) = %d,%v; want %d,%v", k, got, ok, want, wok)
					}
					if strs != nil {
						if got, ok := strs.Get(sk); ok != wok || got != want {
							t.Fatalf("str Get(%s) = %d,%v; want %d,%v", sk, got, ok, want, wok)
						}
					}
				}
			}
			if ints.Len() != len(ref) {
				t.Fatalf("Len = %d, want %d", ints.Len(), len(ref))
			}
			if strs != nil && strs.Len() != len(ref) {
				t.Fatalf("str Len = %d, want %d", strs.Len(), len(ref))
			}
		})
	}
}
