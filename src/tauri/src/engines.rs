// The Go engines, newsd (news) and m24d (trading runner and plays engine): attached to or started.
//
// Attach mode: when the supervisor (scripts/forever.js) or anything else already runs them on their
// ports, the app starts nothing and uses those. Two runners must never trade side by side, and two
// services cannot share a port, so the app starts its own only when neither port has anything on it,
// checked twice a moment apart (a supervisor may be between a service's exit and its restart). The
// supervisor launches the app with --attach, which never starts its own. Engines the app started stop
// when it exits; they also exit on their own if it dies, because they watch their stdin pipe.

use std::sync::Mutex;
use std::thread;
use std::time::Duration;

use tauri::{AppHandle, Manager};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

use crate::local_http;

pub const NEWS_PORT: u16 = 8024;
pub const RUNNER_PORT: u16 = 8025;
const LISTEN_HOST: &str = "127.0.0.1";
// (sidecar name, port, data folder under the app data dir)
const ENGINES: [(&str, u16, &str); 2] = [
    ("newsd", NEWS_PORT, "news"),
    ("m24d", RUNNER_PORT, "trading"),
];
const RECHECK_AFTER: Duration = Duration::from_secs(2);

/// The running engines the app started, kept so they can be stopped when the app exits.
pub struct Engines(Mutex<Vec<CommandChild>>);

impl Engines {
    pub fn new() -> Self {
        Engines(Mutex::new(Vec::new()))
    }
}

/// How the app found the services: Attached to running ones, or Own sidecars it started.
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum Mode {
    Attached,
    Own,
}

fn ports_free() -> bool {
    ENGINES
        .iter()
        .all(|(_, port, _)| !local_http::listening(*port))
}

/// Attaches to the running services, or starts the app's own when nothing runs. attach_only: never start any.
pub fn attach_or_start(
    app: &AppHandle,
    attach_only: bool,
) -> Result<Mode, Box<dyn std::error::Error>> {
    let healthy = ENGINES
        .iter()
        .all(|(_, port, _)| local_http::healthy(*port));
    if attach_only || healthy {
        eprintln!("[money24] attached to the running services (healthy: {healthy})");
        return Ok(Mode::Attached);
    }
    if !ports_free() {
        eprintln!("[money24] a service port is in use: attached, starting nothing");
        return Ok(Mode::Attached);
    }
    thread::sleep(RECHECK_AFTER);
    if !ports_free() {
        eprintln!("[money24] the services came up while checking: attached");
        return Ok(Mode::Attached);
    }
    for (name, port, dir) in ENGINES {
        start(app, name, port, dir)?;
    }
    Ok(Mode::Own)
}

/// Starts one engine with its data in the app data folder and forwards its log to ours.
fn start(
    app: &AppHandle,
    name: &'static str,
    port: u16,
    dir: &str,
) -> Result<(), Box<dyn std::error::Error>> {
    let data = app.path().app_data_dir()?.join(dir);
    std::fs::create_dir_all(&data)?;
    let data = data.to_string_lossy().into_owned();
    let addr = format!("{LISTEN_HOST}:{port}");
    let (mut events, child) = app
        .shell()
        .sidecar(name)?
        .args([
            "--addr",
            addr.as_str(),
            "--data",
            data.as_str(),
            "--exit-with-stdin",
        ])
        .spawn()?;
    app.state::<Engines>().0.lock().unwrap().push(child);

    tauri::async_runtime::spawn(async move {
        while let Some(event) = events.recv().await {
            match event {
                CommandEvent::Stdout(line) | CommandEvent::Stderr(line) => {
                    eprintln!("[{name}] {}", String::from_utf8_lossy(&line).trim_end());
                }
                CommandEvent::Terminated(status) => {
                    eprintln!("[{name}] exited with {:?}", status.code)
                }
                _ => {}
            }
        }
    });
    Ok(())
}

/// Stops the engines the app started; attached services are left running.
pub fn stop(app: &AppHandle) {
    for child in app.state::<Engines>().0.lock().unwrap().drain(..) {
        let _ = child.kill();
    }
}
