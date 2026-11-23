#!/bin/sh
set -eu
src=$(CDPATH='' cd "$(dirname "$0")" && pwd)
build=$(mktemp -d)
trap 'rm -rf "$build"' EXIT
cd "$build"
bun "$src/workload.mjs" 1000000
rustc --edition 2021 -O "$src/bench.rs" -o blog-map-rust
clang -O3 -flto -c "$src/cmap.c" -o cmap.o
clang++ -std=c++20 -O3 -flto "$src/native.cpp" cmap.o -o native
./blog-map-rust rust int 7
./blog-map-rust rust str 7
./blog-map-rust rust-id int 7
./native cpp int 7
./native cpp str 7
./native uthash int 7
./native uthash str 7
