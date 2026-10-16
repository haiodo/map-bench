#!/usr/bin/env bash
# Все фазы по порядку; отдельную фазу: ./run.sh A (или B, C, D, R).
#   A - встроенный map Go: 1.23 (бакеты), 1.25 noswissmap и 1.25 (один компилятор), 1.27
#   B - схемы и библиотеки на Go 1.27
#   C - конкурентные карты (go test -bench, -cpu 1,4,8,16)
#   D - JS: Node и Bun
#   W - JS в браузерах: Safari и Brave (headless), через js/server.mjs
#   R - исходные бенчмарки статей как есть (TFMV/go-map, go-hashtable-shootout)
set -euo pipefail
cd "$(dirname "$0")"
RAW=results/raw
mkdir -p "$RAW"
PHASES="${1:-ARBDWC}"
SIZES="${SIZES:-1000,10000,100000,1000000}"
BIG="${BIG:-10000000}"
REPS="${REPS:-7}"
LAUNCHES="${LAUNCHES:-5}"

./env.sh >/dev/null

# Каждая реализация - в своём процессе: память и состояние GC прошлых участников не влияют на замер.
mapbench() { # toolchain experiment impl out
  local tc=$1 exp=$2 impl=$3 out=$4
  (cd go && GOTOOLCHAIN=$tc GOEXPERIMENT=$exp go build -o "../$RAW/mapbench-$tc-$exp" ./cmd/mapbench)
  "$RAW/mapbench-$tc-$exp" -impl "^${impl}\$" -sizes "$SIZES" -reps "$REPS" -out "$out"
  "$RAW/mapbench-$tc-$exp" -impl "^${impl}\$" -sizes "$BIG" -reps 3 -out "$out"
}

if [[ $PHASES == *A* ]]; then
  out=$RAW/A-builtin.jsonl; : >"$out"
  mapbench go1.23.12 "" go/map "$out"
  mapbench go1.25.8 noswissmap go/map "$out"
  mapbench go1.25.8 "" go/map "$out"
  mapbench go1.27.1 "" go/map "$out"
fi

if [[ $PHASES == *B* ]]; then
  out=$RAW/B-schemes.jsonl; : >"$out"
  (cd go && go build -o "../$RAW/mapbench-go1.27.1-" ./cmd/mapbench)
  for impl in $("$RAW/mapbench-go1.27.1-" -list); do
    mapbench go1.27.1 "" "$impl" "$out"
  done
fi

if [[ $PHASES == *D* ]]; then
  out=$RAW/D-js.jsonl; : >"$out"
  for impl in js/Map js/Object; do
    for keys in int str; do
      node --expose-gc js/mapbench.mjs --impl $impl --keys $keys --sizes "$SIZES" --reps "$REPS" --out "$out"
      # V8: после 2^23-1 строковых свойств каждая вставка в Object перенумеровывает все (~1 с), 10 млн не дождаться
      if [[ $impl != js/Object || $keys != str ]]; then
        node --expose-gc js/mapbench.mjs --impl $impl --keys $keys --sizes "$BIG" --reps 3 --out "$out"
      fi
      bun js/mapbench.mjs --impl $impl --keys $keys --sizes "$SIZES" --reps "$REPS" --out "$out"
      bun js/mapbench.mjs --impl $impl --keys $keys --sizes "$BIG" --reps 3 --out "$out"
    done
  done
fi

if [[ $PHASES == *W* ]]; then
  out=$RAW/D-browser.jsonl; : >"$out"
  url="http://localhost:8765/browser.html?sizes=$SIZES&reps=$REPS&launches=$LAUNCHES"
  sv=$(defaults read /Applications/Safari.app/Contents/Info CFBundleShortVersionString)
  OUT=$out EXIT_ON_DONE=1 node js/server.mjs & srv=$!
  sleep 1
  # Safari не умеет headless: вкладка откроется, её нельзя уводить в фон до "готово"
  open -a Safari "$url&label=safari%20$sv"
  wait $srv
  brave="/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
  if [[ -x $brave ]]; then
    bv=$("$brave" --version | awk '{print $NF}')
    OUT=$out EXIT_ON_DONE=1 node js/server.mjs & srv=$!
    sleep 1
    prof=$(mktemp -d)
    "$brave" --headless=new --user-data-dir="$prof" --js-flags=--expose-gc "$url&label=brave%20$bv" >/dev/null 2>&1 & br=$!
    wait $srv
    kill $br 2>/dev/null || true
    rm -rf "$prof"
  fi
fi

if [[ $PHASES == *R* ]]; then
  for tc in go1.23.12 go1.27.1; do
    (cd third_party/go-map && GOTOOLCHAIN=$tc go run .) >"$RAW/R-tfmv-$tc.txt" 2>&1
    (cd third_party/go-hashtable-shootout && GOTOOLCHAIN=$tc go run ./bench -out "../../$RAW/R-shootout-$tc") >"$RAW/R-shootout-$tc.txt" 2>&1
  done
fi

if [[ $PHASES == *C* ]]; then
  # upstream и extra - одна нагрузка; count 3 даёт разброс, benchstat сводит
  for cpu in 1 4 8 16; do
    (cd concurrent && go test -run '^$' -bench . -cpu $cpu -benchtime 1s -count 3 -timeout 0) >"$RAW/C-cpu$cpu.txt"
  done
fi
echo "готово: $RAW"
