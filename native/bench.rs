use std::{
    collections::HashMap,
    env, fs,
    hash::{BuildHasher, BuildHasherDefault, Hash, Hasher},
    time::Instant,
};

#[derive(Default)]
struct Identity(u64);
impl Hasher for Identity {
    fn finish(&self) -> u64 {
        self.0
    }
    fn write(&mut self, _: &[u8]) {
        panic!("expected u32")
    }
    fn write_u32(&mut self, value: u32) {
        self.0 = value.into();
    }
}

struct Work {
    n: usize,
    mixed_size: usize,
    hit: u64,
    mixed: [u64; 2],
    keys: Vec<u32>,
    trace: Vec<u32>,
}
impl Work {
    fn read() -> Self {
        let bytes = fs::read("workload.bin").unwrap();
        let mut words = bytes
            .chunks_exact(4)
            .map(|b| u32::from_le_bytes(b.try_into().unwrap()));
        let n = words.next().unwrap() as usize;
        let mixed_size = words.next().unwrap() as usize;
        let mut sum =
            || u64::from(words.next().unwrap()) | (u64::from(words.next().unwrap()) << 32);
        let hit = sum();
        let mixed = [sum(), sum()];
        let keys = words.by_ref().take(2 * n).collect();
        let trace: Vec<_> = words.collect();
        assert_eq!(trace.len(), n);
        Self {
            n,
            mixed_size,
            hit,
            mixed,
            keys,
            trace,
        }
    }
}
fn run<K: Eq + Hash + Copy, H: BuildHasher + Default>(
    w: &Work,
    keys: &[K],
    name: &str,
    ty: &str,
    reps: usize,
) {
    for metric in [
        "insert",
        "insert_hint",
        "lookup_hit",
        "lookup_miss",
        "iterate",
        "delete",
        "mixed",
    ] {
        for rep in 0..reps {
            let mut m = HashMap::<K, u32, H>::default();
            let build = |m: &mut HashMap<K, u32, H>| {
                for (&k, &v) in keys[..w.n].iter().zip(&w.keys) {
                    m.insert(k, v);
                }
            };
            if !metric.starts_with("insert") {
                build(&mut m);
            }
            let mut sum = 0u64;
            let start = Instant::now();
            match metric {
                "insert" => build(&mut m),
                "insert_hint" => {
                    m = HashMap::with_capacity_and_hasher(w.n, H::default());
                    build(&mut m);
                }
                "iterate" => sum = m.values().map(|&v| u64::from(v)).sum(),
                "delete" => {
                    for k in &keys[..w.n] {
                        m.remove(k);
                    }
                }
                "mixed" => {
                    for &x in &w.trace {
                        let j = (x / 4) as usize;
                        match x % 4 {
                            0 => {
                                m.insert(keys[j], if ty == "str" { j as u32 } else { w.keys[j] });
                            }
                            1 => sum += u64::from(*m.get(&keys[j]).unwrap_or(&0)),
                            2 => {
                                m.remove(&keys[j]);
                            }
                            _ => unreachable!(),
                        }
                    }
                }
                _ => {
                    let offset = if metric == "lookup_miss" { w.n } else { 0 };
                    for k in &keys[offset..offset + w.n] {
                        sum += u64::from(*m.get(k).unwrap_or(&0));
                    }
                }
            }
            std::hint::black_box((&m, sum));
            let ns = start.elapsed().as_nanos() as f64 / w.n as f64;
            assert_eq!(
                m.len(),
                match metric {
                    "delete" => 0,
                    "mixed" => w.mixed_size,
                    _ => w.n,
                }
            );
            assert_eq!(
                sum,
                match metric {
                    "mixed" => w.mixed[(ty == "str") as usize],
                    "lookup_hit" | "iterate" => w.hit,
                    _ => 0,
                }
            );
            println!("{name},{ty},{},{metric},{rep},{ns:.3}", w.n);
        }
    }
}
fn main() {
    let args: Vec<_> = env::args().collect();
    let w = Work::read();
    let reps = args[3].parse().unwrap();
    match (args[1].as_str(), args[2].as_str()) {
        ("rust-id", "int") => {
            run::<_, BuildHasherDefault<Identity>>(&w, &w.keys, &args[1], &args[2], reps)
        }
        ("rust", "int") => run::<_, std::hash::RandomState>(&w, &w.keys, &args[1], &args[2], reps),
        ("rust", "str") => {
            let strings: Vec<_> = w.keys.iter().map(|k| format!("key-{k:010}")).collect();
            let views: Vec<_> = strings.iter().map(String::as_str).collect();
            run::<_, std::hash::RandomState>(&w, &views, &args[1], &args[2], reps);
        }
        _ => panic!("expected rust int|str or rust-id int"),
    }
}
