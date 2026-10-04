// Dumps what allsorts makes of WOFF 2 font collections: for each file, each
// font's tables, as allsorts rebuilds them, in hex; or why it refused the file.
use allsorts::binary::read::ReadScope;
use allsorts::woff2::Woff2Font;
use std::io::Write;

fn tag(t: u32) -> String {
    t.to_be_bytes().iter().map(|&b| b as char).collect()
}

fn main() {
    let out = std::io::stdout();
    let mut out = out.lock();
    for path in std::env::args().skip(1) {
        let name = std::path::Path::new(&path).file_name().unwrap().to_string_lossy().to_string();
        let data = std::fs::read(&path).expect("read");
        writeln!(out, "file {}", name).unwrap();
        let font = match ReadScope::new(&data).read::<Woff2Font>() {
            Ok(f) => f,
            Err(e) => {
                writeln!(out, "refused {:?}", e).unwrap();
                continue;
            }
        };
        let n = match &font.collection_directory {
            Some(d) => d.fonts().count(),
            None => 1,
        };
        for i in 0..n {
            match font.table_provider(i) {
                Ok(p) => {
                    let mut tables: Vec<_> = p.into_tables().into_iter().collect();
                    tables.sort_by_key(|(t, _)| *t);
                    writeln!(out, "font {} tables {}", i, tables.len()).unwrap();
                    for (t, d) in tables {
                        let hex: String = d.iter().map(|b| format!("{:02x}", b)).collect();
                        writeln!(out, "table {} {}", tag(t).replace(" ", "_"), hex).unwrap();
                    }
                }
                Err(e) => writeln!(out, "font {} refused {:?}", i, e).unwrap(),
            }
        }
    }
}
