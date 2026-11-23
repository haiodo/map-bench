#include <cassert>
#include <chrono>
#include <cstdio>
#include <fstream>
#include <string>
#include <string_view>
#include <unordered_map>
#include <vector>
extern "C" {
#include "cmap.h"
}
using namespace std;
using U = uint32_t;
using S = string_view;
static const void* ptr(const U& k) { return &k; }
static const void* ptr(S k) { return k.data(); }
static unsigned len(U) { return 4; }
static unsigned len(S k) { return k.size(); }
template<class K> struct CMap {
    entry *m = nullptr;
    ~CMap() { cclear(&m); }
    void put(const K& k,U v) { cput(&m,ptr(k),len(k),v); }
    uint64_t get(const K& k) { return cget(m,ptr(k),len(k)); }
    void del(const K& k) { cdel(&m,ptr(k),len(k)); }
    auto size() { return csize(m); }
    auto sum() { return csum(m); }
};
template<class K> struct CppMap {
    unordered_map<K,U> m;
    void put(const K& k,U v) { m.insert_or_assign(k,v); }
    uint64_t get(const K& k) { auto i=m.find(k); return i==m.end()?0:i->second; }
    void del(const K& k) { m.erase(k); }
    auto size() { return m.size(); }
    uint64_t sum() { uint64_t s=0; for(auto& p:m) s+=p.second; return s; }
};
struct Work {
    U n, mixed_size;
    uint64_t hit, mix[2];
    vector<U> keys, trace;
    Work() {
        ifstream in("workload.bin",ios::binary);
        auto read=[&](auto *p,size_t n) { in.read(reinterpret_cast<char*>(p),sizeof(*p)*n); assert(in); };
        read(&n,1); read(&mixed_size,1); read(&hit,1); read(mix,2);
        keys.resize(2*n); trace.resize(n);
        read(keys.data(),keys.size()); read(trace.data(),n);
    }
};
template<class M,class K> void run(const Work& w,const vector<K>& keys,const char* name,int reps) {
    constexpr bool str=is_same_v<K,S>;
    for(string metric:{"insert","lookup_hit","lookup_miss","iterate","delete","mixed"}) {
        for(int rep=0; rep<reps; ++rep) {
            M m;
            auto build=[&] { for(U i=0;i<w.n;++i) m.put(keys[i],w.keys[i]); };
            if(metric!="insert") build();
            uint64_t sum=0;
            auto start=chrono::steady_clock::now();
            if(metric=="insert") build();
            else if(metric=="iterate") sum=m.sum();
            else if(metric=="delete") for(U i=0;i<w.n;++i) m.del(keys[i]);
            else if(metric=="mixed") {
                for(U x:w.trace) {
                    U j=x/4;
                    switch(x%4) {
                        case 0: m.put(keys[j],str?j:w.keys[j]); break;
                        case 1: sum+=m.get(keys[j]); break;
                        case 2: m.del(keys[j]); break;
                    }
                }
            } else {
                U off=metric=="lookup_miss"?w.n:0;
                for(U i=0;i<w.n;++i) sum+=m.get(keys[off+i]);
            }
            auto elapsed=chrono::duration<double,nano>(chrono::steady_clock::now()-start).count();
            assert(m.size()==(metric=="delete"?0:metric=="mixed"?w.mixed_size:w.n));
            assert(sum==(metric=="mixed"?w.mix[str]:metric=="lookup_hit"||metric=="iterate"?w.hit:0));
            printf("%s,%s,%u,%s,%d,%.3f\n",name,str?"str":"int",w.n,metric.c_str(),rep,elapsed/w.n);
        }
    }
}
int main(int argc,char** argv) {
    assert(argc==4);
    Work w;
    bool c=string(argv[1])=="uthash";
    int reps=stoi(argv[3]);
    if(string(argv[2])=="int") {
        if(c) run<CMap<U>>(w,w.keys,argv[1],reps);
        else run<CppMap<U>>(w,w.keys,argv[1],reps);
    } else {
        vector<string> storage;
        for(U k:w.keys) {
            char buf[16]; snprintf(buf,sizeof(buf),"key-%010u",k); storage.emplace_back(buf);
        }
        vector<S> views(storage.begin(),storage.end());
        if(c) run<CMap<S>>(w,views,argv[1],reps);
        else run<CppMap<S>>(w,views,argv[1],reps);
    }
}
