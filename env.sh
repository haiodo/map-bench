#!/usr/bin/env bash
# Окружение прогона: без него цифры в results/ нельзя ни сравнить, ни повторить.
set -u
cd "$(dirname "$0")"
mkdir -p results
{
  echo "date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "host: $(sysctl -n hw.model 2>/dev/null || uname -m)"
  echo "cpu: $(sysctl -n machdep.cpu.brand_string 2>/dev/null || grep -m1 'model name' /proc/cpuinfo)"
  echo "cores: $(sysctl -n hw.ncpu 2>/dev/null || nproc) (perf $(sysctl -n hw.perflevel0.logicalcpu 2>/dev/null), eff $(sysctl -n hw.perflevel1.logicalcpu 2>/dev/null))"
  echo "mem_bytes: $(sysctl -n hw.memsize 2>/dev/null)"
  echo "os: $(sw_vers -productVersion 2>/dev/null || uname -sr)"
  echo "power: $(pmset -g batt 2>/dev/null | head -1)"
  # Energy Mode: 0 - Automatic, 1 - Low Power, 2 - High Power (на старых macOS - lowpowermode 0/1)
  echo "powermode: $(pmset -g 2>/dev/null | awk '/ powermode|lowpowermode/ {print $1 "=" $2}')"
  for v in 1.23.12 1.25.8 1.27.1; do echo "go$v: $(GOTOOLCHAIN=go$v go version 2>/dev/null)"; done
  echo "node: $(node --version 2>/dev/null) v8 $(node -p process.versions.v8 2>/dev/null)"
  echo "bun: $(bun --version 2>/dev/null)"
  echo "go (фаза C): $(cd concurrent && go version 2>/dev/null)"
  echo "safari: $(defaults read /Applications/Safari.app/Contents/Info CFBundleShortVersionString 2>/dev/null)"
  echo "brave: $("/Applications/Brave Browser.app/Contents/MacOS/Brave Browser" --version 2>/dev/null)"
  echo "submodules:"
  git submodule status 2>/dev/null | sed 's/^/  /'
} | tee results/env.txt
