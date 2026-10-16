// Command mapbench меряет ассоциативные массивы на Go по методике go/harness.
//
//	go run ./cmd/mapbench -impl go/map -sizes 1000,1000000 -out ../results/raw/go.jsonl
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/haiodo/map-bench/go/harness"
)

func main() {
	implRe := flag.String("impl", ".", "регулярное выражение по имени реализации")
	keys := flag.String("keys", "int,str", "типы ключей: int, str")
	sizes := flag.String("sizes", "1000,10000,100000,1000000,10000000", "размеры через запятую")
	reps := flag.Int("reps", 7, "повторов на метрику, берётся медиана")
	minOps := flag.Int("minops", 1_000_000, "минимум операций в одном замере")
	out := flag.String("out", "", "файл JSON lines (дописывается); пусто - stdout")
	label := flag.String("label", "", "подпись рантайма; пусто - runtime.Version()")
	list := flag.Bool("list", false, "только напечатать реализации")
	flag.Parse()

	re := regexp.MustCompile(*implRe)
	var sel []harness.Impl
	for _, im := range impls() {
		if re.MatchString(im.Name) {
			sel = append(sel, im)
		}
	}
	if *list {
		for _, im := range sel {
			fmt.Println(im.Name)
		}
		return
	}

	w := os.Stdout
	if *out != "" {
		f, err := os.OpenFile(*out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	}
	rt := *label
	if rt == "" {
		// runtime.Version() сама дописывает эксперименты: "go1.25.8 X:noswissmap"
		rt = runtime.Version()
	}
	cfg := harness.Config{Runtime: rt, Reps: *reps, MinOps: *minOps, Out: w, Log: os.Stderr}
	fmt.Fprintf(os.Stderr, "runtime %s, GOMAXPROCS %d, impls %d\n", rt, runtime.GOMAXPROCS(0), len(sel))

	for _, s := range strings.Split(*sizes, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			fmt.Fprintln(os.Stderr, "sizes:", err)
			os.Exit(2)
		}
		wl := harness.NewWorkload(n, 42)
		for _, im := range sel {
			if strings.Contains(*keys, "int") {
				harness.RunInt(cfg, im, wl)
			}
			if strings.Contains(*keys, "str") {
				harness.RunStr(cfg, im, wl)
			}
		}
	}
}
