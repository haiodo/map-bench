module github.com/haiodo/map-bench/go

go 1.23

require (
	github.com/cockroachdb/swiss v0.0.0-20260820225851-333444432258
	github.com/dolthub/swiss v0.2.1
	github.com/google/btree v1.1.3
	github.com/huandu/skiplist v1.2.1
	github.com/plar/go-adaptive-radix-tree v1.0.7
	github.com/tidwall/btree v1.8.2
	github.com/tidwall/hashmap v1.8.1
	hashcmp v0.0.0
)

require (
	github.com/dolthub/maphash v0.1.0 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	github.com/zeebo/xxh3 v1.0.2 // indirect
)

replace hashcmp => ../third_party/go-hashtable-shootout
