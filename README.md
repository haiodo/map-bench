# map-bench

Воспроизводимый стенд для сравнения ассоциативных массивов: встроенный `map` Go по версиям
(бакеты против Swiss Tables), классические схемы хеш-таблиц, деревья и skip list, конкурентные
карты Go, `Map` и `Object` в Node, Bun, Safari и Brave.

> **TL;DR (EN):** reproducible benchmark of associative arrays on one machine: Go's builtin map
> across versions (buckets vs Swiss Tables), hand-written hash table schemes, ordered maps,
> concurrent Go maps, and JS `Map`/`Object` in Node, Bun, Safari and Brave. Go and JS use the
> same keys (mulberry32) and the same operation stream, so the numbers are comparable.

Находки и выводы - в [FINDINGS.md](FINDINGS.md), сводные таблицы - в
[results/tables.md](results/tables.md), окружение прогона - в [results/env.txt](results/env.txt).

## Запуск

```bash
git clone --recursive https://github.com/haiodo/map-bench && cd map-bench
./run.sh           # все фазы
./run.sh A         # одна фаза
python3 parse.py   # results/tables.md из results/raw
```

Нужны Go (toolchain 1.23, 1.25 и 1.27 скачаются сами через `GOTOOLCHAIN`), Node, Bun; для фазы W -
Safari и/или Brave. Размеры и число повторов: `SIZES=1000,100000 BIG=10000000 REPS=7 ./run.sh B`.

| Фаза | Что меряет | Где код |
|---|---|---|
| A | встроенный `map`: Go 1.23 (бакеты), Go 1.25 с `GOEXPERIMENT=noswissmap` и без (один компилятор, разная map), Go 1.27 | `go/cmd/mapbench` |
| B | схемы из go-hashtable-shootout (цепочки, linear, quadratic, Robin Hood, hopscotch, cuckoo, swiss) и библиотеки: dolthub/swiss, cockroachdb/swiss, tidwall/hashmap, tidwall/btree, google/btree, huandu/skiplist, plar/go-adaptive-radix-tree | `go/cmd/mapbench/impls.go` |
| C | конкурентные карты: sync.Map, xsync, orcaman, haxmap, cornelk, плюс map+Mutex, map+RWMutex, 256 шардов на stdlib, concurrent-swiss-map; `-cpu 1,4,8,<число ядер>`, size 1000 и 1000000 | `concurrent/` |
| D | `Map` и `Object` в Node и Bun | `js/` |
| W | то же в Safari и Brave (headless), в Web Worker | `js/browser.html`, `js/server.mjs` |
| R | исходные бенчмарки как есть: TFMV/go-map, go-hashtable-shootout | `third_party/` |

## Методика (фазы A, B, D, W)

Одна методика на Go и JS: `go/harness/harness.go` и `js/harness.mjs` - копии друг друга, тест
`TestWorkloadMatchesJS` проверяет, что ключи и операции совпадают побитно.

- Ключи: N различных целых < 2^31 в случайном порядке (mulberry32, seed 42). В Node это SMI
  (сборка без pointer compression, SMI - 32 бита), а в Chromium/Brave с pointer compression SMI -
  31 бит, и ключи от 2^30 (примерно половина) становятся HeapNumber. Int-ключи в Node и Brave
  поэтому сравниваются не в равных условиях; исправить - ключи < 2^30 и полный перепрогон.
  Строковые ключи: `key-%010d` от тех же чисел. Ещё N отсутствующих ключей для промахов.
- Метрики, нс на операцию: `insert` (без подсказки размера), `insert_hint` (с подсказкой, только Go),
  `lookup_hit`, `lookup_miss`, `iterate`, `delete` (все ключи из заполненной таблицы), `mixed`
  (50% get, 30% put, 20% delete, четверть операций по отсутствующим ключам); `mem` - байт на элемент
  по приросту живой кучи после GC.
- В одном замере не меньше 1 млн операций (в браузере 5 млн): на малых N проход повторяется.
  Повторов 7 (для N = 10 млн - 3), в таблицах медиана, в JSON есть и минимум. В браузере каждая
  конфигурация ещё и запускается в 5 свежих Worker (`LAUNCHES`), в таблице - медиана медиан: V8 от
  запуска к запуску даёт на `mixed` разницу до 3 раз при стабильных повторах внутри запуска.
- Перед каждым замером - GC. Каждая реализация и каждый тип ключей - в своём процессе (в браузере -
  в своём Worker): иначе JIT получает полиморфные call site, а куча - мусор прошлых участников.
- Обёртки над библиотеками ничего не оптимизируют: boxing в `interface{}` у skiplist и `[]byte`-ключи
  у ART - часть их стоимости.

Фаза C - нагрузка и хелперы из [go-concurrent-map-bench](https://github.com/puzpuzpuz/go-concurrent-map-bench)
без изменений (`concurrent/upstream_bench_test.go`), новые участники - в `concurrent/extra_bench_test.go`.

## Как повторить честно

- Питание от сети, режим энергосбережения выключен, фоновые задачи закрыты: на Apple Silicon
  планировщик может отдать процесс эффективным ядрам, и медиана уплывает.
- Безопасно сравнивать только цифры с одной машины; сводить свои результаты с чужими таблицами
  без `results/env.txt` бессмысленно.

## Источники и лицензии

| Каталог | Источник | Лицензия |
|---|---|---|
| `third_party/go-hashtable-shootout` | [TKTHdev/go-hashtable-shootout](https://github.com/TKTHdev/go-hashtable-shootout) | не указана - подключён submodule, код не копируется |
| `third_party/go-concurrent-map-bench`, `concurrent/upstream_bench_test.go` | [puzpuzpuz/go-concurrent-map-bench](https://github.com/puzpuzpuz/go-concurrent-map-bench) | Apache-2.0, `concurrent/LICENSE.upstream` |
| `third_party/go-map` | [TFMV/go-map](https://github.com/TFMV/go-map) | MIT |

Остальной код - MIT, см. [LICENSE](LICENSE).
