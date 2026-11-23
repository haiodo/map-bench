#include "cmap.h"
#include "uthash.h"
struct entry { uint32_t value; UT_hash_handle hh; };
void cput(entry **m, const void *key, unsigned len, uint32_t value) {
    entry *e;
    unsigned hash;
    HASH_VALUE(key, len, hash);
    HASH_FIND_BYHASHVALUE(hh, *m, key, len, hash, e);
    if (!e) {
        e = malloc(sizeof(*e));
        if (!e) abort();
        HASH_ADD_KEYPTR_BYHASHVALUE(hh, *m, key, len, hash, e);
    }
    e->value = value;
}
uint64_t cget(entry *m, const void *key, unsigned len) {
    entry *e;
    HASH_FIND(hh, m, key, len, e);
    return e ? e->value : 0;
}
void cdel(entry **m, const void *key, unsigned len) {
    entry *e;
    HASH_FIND(hh, *m, key, len, e);
    if (e) { HASH_DEL(*m, e); free(e); }
}
unsigned csize(entry *m) { return HASH_COUNT(m); }
uint64_t csum(entry *m) {
    uint64_t sum = 0;
    for (entry *e = m; e; e = e->hh.next) sum += e->value;
    return sum;
}
void cclear(entry **m) {
    entry *e, *tmp;
    HASH_ITER(hh, *m, e, tmp) { HASH_DEL(*m, e); free(e); }
}
