// Money 24 desktop shell: one window on the live feed, with the Go engines as sidecars: newsd (news)
// and m24d (paper trading runner, shown at trading.html). Both start on launch and stop on exit; they
// also exit on their own if this app dies, because they watch their stdin pipe (--exit-with-stdin).
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::sync::Mutex;

use tauri::{window::Color, AppHandle, Manager, RunEvent, Url, WebviewUrl, WebviewWindowBuilder};
use tauri_plugin_opener::OpenerExt;
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

// (sidecar name, listen address, data folder under the app data dir)
const ENGINES: [(&str, &str, &str); 2] = [
    ("newsd", "127.0.0.1:8024", "news"),
    ("m24d", "127.0.0.1:8025", "trading"),
];
const WINDOW_LABEL: &str = "main";
const WINDOW_TITLE: &str = "Money 24 News";
const FEED_PAGE: &str = "feed.html";
const WINDOW_SIZE: (f64, f64) = (1280.0, 800.0);
const WINDOW_MIN_SIZE: (f64, f64) = (320.0, 320.0);
// Matches --color-app in src/styles/theme.css, so the window never flashes white while loading.
const WINDOW_BACKGROUND: Color = Color(5, 5, 8, 255);
// Hosts the window itself may load. Every other link opens in the system browser.
const APP_HOSTS: [&str; 3] = ["tauri.localhost", "localhost", "127.0.0.1"];
const APP_SCHEME: &str = "tauri";

/// The running engines, kept so they can be stopped when the app exits.
struct Engines(Mutex<Vec<CommandChild>>);

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_opener::init())
        .manage(Engines(Mutex::new(Vec::new())))
        .setup(|app| {
            for (name, addr, dir) in ENGINES {
                start_engine(app.handle(), name, addr, dir)?;
            }
            open_window(app.handle())?;
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("Money 24 failed to start")
        .run(|app, event| {
            if let RunEvent::Exit = event {
                stop_engines(app);
            }
        });
}

/// Starts one engine with its data in the app data folder and forwards its log to ours.
fn start_engine(app: &AppHandle, name: &'static str, addr: &str, dir: &str) -> Result<(), Box<dyn std::error::Error>> {
    let data = app.path().app_data_dir()?.join(dir);
    std::fs::create_dir_all(&data)?;
    let data = data.to_string_lossy().into_owned();
    let (mut events, child) = app
        .shell()
        .sidecar(name)?
        .args(["--addr", addr, "--data", data.as_str(), "--exit-with-stdin"])
        .spawn()?;
    app.state::<Engines>().0.lock().unwrap().push(child);

    tauri::async_runtime::spawn(async move {
        while let Some(event) = events.recv().await {
            match event {
                CommandEvent::Stdout(line) | CommandEvent::Stderr(line) => {
                    eprintln!("[{name}] {}", String::from_utf8_lossy(&line).trim_end());
                }
                CommandEvent::Terminated(status) => eprintln!("[{name}] exited with {:?}", status.code),
                _ => {}
            }
        }
    });
    Ok(())
}

fn stop_engines(app: &AppHandle) {
    for child in app.state::<Engines>().0.lock().unwrap().drain(..) {
        let _ = child.kill();
    }
}

fn is_app_url(url: &Url) -> bool {
    url.scheme() == APP_SCHEME || url.host_str().is_some_and(|host| APP_HOSTS.contains(&host))
}

/// Opens the feed. Clicking a headline navigates away, so hand those URLs to the system browser.
fn open_window(app: &AppHandle) -> tauri::Result<()> {
    let handle = app.clone();
    WebviewWindowBuilder::new(app, WINDOW_LABEL, WebviewUrl::App(FEED_PAGE.into()))
        .title(WINDOW_TITLE)
        .inner_size(WINDOW_SIZE.0, WINDOW_SIZE.1)
        .min_inner_size(WINDOW_MIN_SIZE.0, WINDOW_MIN_SIZE.1)
        .background_color(WINDOW_BACKGROUND)
        .on_navigation(move |url| {
            if is_app_url(url) {
                return true;
            }
            let _ = handle.opener().open_url(url.as_str(), None::<&str>);
            false
        })
        .build()?;
    Ok(())
}
