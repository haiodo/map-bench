package harness

import (
	"slices"
	"testing"
)

// Эталон снят с js/harness.mjs: newWorkload(1000, 42) обязан давать те же ключи и операции.
func TestWorkloadMatchesJS(t *testing.T) {
	w := NewWorkload(1000, 42)
	if got := w.Keys[:3]; !slices.Equal(got, []uint64{1290860478, 962696645, 1830656352}) {
		t.Fatalf("keys %v", got)
	}
	if w.Miss[999] != 1836414630 || w.SKeys[1] != "key-0962696645" {
		t.Fatalf("miss %d, skey %q", w.Miss[999], w.SKeys[1])
	}
	want := []Op{{1, 265}, {2, 206}, {2, 335}, {0, 992}, {1, 763}, {1, 363}, {1, 662}, {2, 382}}
	if !slices.Equal(w.Mixed[:8], want) {
		t.Fatalf("mixed %v", w.Mixed[:8])
	}
}
