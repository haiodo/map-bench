#include <stdint.h>
typedef struct entry entry;
void cput(entry **m, const void *key, unsigned len, uint32_t value);
uint64_t cget(entry *m, const void *key, unsigned len);
void cdel(entry **m, const void *key, unsigned len);
unsigned csize(entry *m);
uint64_t csum(entry *m);
void cclear(entry **m);
