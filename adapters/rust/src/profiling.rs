use anyhow::{Context, Result};
use serde_json::json;
use std::collections::BTreeMap;
use std::env;
use std::fs;
use std::path::PathBuf;
use std::sync::{Mutex, OnceLock};
use std::time::Instant;

#[derive(Default)]
struct AdapterProfile {
    path: Option<PathBuf>,
    phases: Vec<serde_json::Value>,
    counts: BTreeMap<String, i64>,
}

static PROFILE: OnceLock<Mutex<AdapterProfile>> = OnceLock::new();

fn profile() -> &'static Mutex<AdapterProfile> {
    PROFILE.get_or_init(|| {
        Mutex::new(AdapterProfile {
            path: env::var_os("LEXICON_ADAPTER_PROFILE").map(PathBuf::from),
            ..AdapterProfile::default()
        })
    })
}

pub(crate) fn measure<T>(name: &str, operation: impl FnOnce() -> T) -> T {
    let enabled = profile()
        .lock()
        .expect("adapter profile lock")
        .path
        .is_some();
    if !enabled {
        return operation();
    }
    let started = Instant::now();
    let result = operation();
    profile()
        .lock()
        .expect("adapter profile lock")
        .phases
        .push(json!({
            "name": name,
            "duration_ns": started.elapsed().as_nanos() as u64,
        }));
    result
}

pub(crate) fn set(name: &str, value: usize) {
    let mut profile = profile().lock().expect("adapter profile lock");
    if profile.path.is_some() {
        profile.counts.insert(name.to_string(), value as i64);
    }
}

pub(crate) fn write() -> Result<()> {
    let profile = profile().lock().expect("adapter profile lock");
    let Some(path) = profile.path.as_ref() else {
        return Ok(());
    };
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)
            .with_context(|| format!("create adapter profile directory {}", parent.display()))?;
    }
    let mut data = serde_json::to_vec_pretty(&json!({
        "version": 1,
        "phases": profile.phases,
        "counts": profile.counts,
    }))?;
    data.push(b'\n');
    fs::write(path, data).with_context(|| format!("write adapter profile {}", path.display()))?;
    Ok(())
}
