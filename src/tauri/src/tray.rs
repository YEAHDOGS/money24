// Money 24's icon in the system tray, for as long as the app runs. Left click: the pocket (pocket.rs).
// Right click: open the desk, the pocket, the logs folder, restart or stop every service (the stop asks
// first, in the pocket), and quit. Quit closes the app only: the services keep running, and the
// supervisor shows its own tray icon (scripts/tray.ps1) in its place. The tooltip is a one-line
// summary, read from the services every few seconds: "Money 24: all running · 12 open".

use std::thread;
use std::time::Duration;

use serde_json::Value;
use tauri::menu::{Menu, MenuEvent, MenuItem, PredefinedMenuItem};
use tauri::tray::{MouseButton, MouseButtonState, TrayIcon, TrayIconBuilder, TrayIconEvent};
use tauri::{AppHandle, Manager};
use tauri_plugin_opener::OpenerExt;

use crate::engines::{Mode, NEWS_PORT, RUNNER_PORT};
use crate::{local_http, pocket, supervisor::Supervisor, Shell};

// Also the mark scripts/forever.js looks for inside the exe: a build without it predates the tray (and
// would open a window and start its own engines), so the supervisor never launches it as the tray.
const TRAY_ID: &str = "m24-desktop-tray-1";
const MENU_OPEN: &str = "open";
const MENU_STATUS: &str = "status";
const MENU_LOGS: &str = "logs";
const MENU_RESTART: &str = "restart";
const MENU_STOP: &str = "stop";
const MENU_QUIT: &str = "quit";
const LABEL_OPEN: &str = "Open the desk";
const LABEL_STATUS: &str = "Status";
const LABEL_LOGS: &str = "Open the logs folder";
const LABEL_RESTART: &str = "Restart Money 24";
const LABEL_STOP: &str = "Stop Money 24…";
const LABEL_QUIT: &str = "Quit (services keep running)";
const LABEL_QUIT_OWN: &str = "Quit (stops the services it started)";
const TIP_STARTING: &str = "Money 24: starting…";
const TIP_RUNNER_DOWN: &str = "Money 24: runner not answering";
const TIP_ALL_RUNNING: &str = "all running";
const TIP_PREFIX: &str = "Money 24: ";
const TIP_DOWN: &str = "down";
const TIP_OPEN: &str = "open";
const TIP_SEPARATOR: &str = " · ";
const SUMMARY_EVERY: Duration = Duration::from_secs(10);
// What the tooltip calls each service the supervisor runs.
const SERVICE_NAMES: [(&str, &str); 3] = [("newsd", "news"), ("m24d", "runner"), ("tunnel", "tunnel")];
const NEWS_NAME: &str = "news";
const SUPERVISOR_NAME: &str = "supervisor";
const SUPERVISOR_STALE: &str = "stale";
const ROUTE_SUPERVISOR: &str = "/api/supervisor";
const ROUTE_PREDICT: &str = "/api/predict";
const ROUTE_MEMEDESK: &str = "/api/memedesk";
const ROUTE_SCOUT: &str = "/api/plays/scout";
pub const QUIT_CODE: i32 = 0; // the supervisor leaves an app that exits with this closed

/// Puts the icon in the tray and keeps its tooltip current.
pub fn build(app: &AppHandle, mode: Mode, supervisor: &Supervisor) -> tauri::Result<()> {
    let item = |id: &str, label: &str, enabled: bool| MenuItem::with_id(app, id, label, enabled, None::<&str>);
    let separator = || PredefinedMenuItem::separator(app);
    let quit = if mode == Mode::Own { LABEL_QUIT_OWN } else { LABEL_QUIT };
    let menu = Menu::with_items(
        app,
        &[
            &item(MENU_OPEN, LABEL_OPEN, true)?,
            &item(MENU_STATUS, LABEL_STATUS, true)?,
            &item(MENU_LOGS, LABEL_LOGS, true)?,
            &separator()?,
            &item(MENU_RESTART, LABEL_RESTART, supervisor.known())?,
            &item(MENU_STOP, LABEL_STOP, supervisor.known())?,
            &separator()?,
            &item(MENU_QUIT, quit, true)?,
        ],
    )?;
    let mut tray = TrayIconBuilder::with_id(TRAY_ID)
        .tooltip(TIP_STARTING)
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(on_menu)
        .on_tray_icon_event(on_click);
    if let Some(icon) = app.default_window_icon() {
        tray = tray.icon(icon.clone());
    }
    let tray = tray.build(app)?;
    thread::spawn(move || keep_summary(tray));
    Ok(())
}

fn on_click(tray: &TrayIcon, event: TrayIconEvent) {
    if let TrayIconEvent::Click { button: MouseButton::Left, button_state: MouseButtonState::Up, position, .. } = event {
        pocket::toggle(tray.app_handle(), position);
    }
}

fn on_menu(app: &AppHandle, event: MenuEvent) {
    let shell = app.state::<Shell>();
    let done = match event.id.as_ref() {
        MENU_OPEN => crate::open_desk(app).map_err(|e| e.to_string()),
        MENU_STATUS => Ok(pocket::show(app, false)),
        MENU_LOGS => open_logs(app, &shell.supervisor),
        MENU_RESTART => shell.supervisor.restart().map_err(|e| e.to_string()),
        MENU_STOP => Ok(pocket::show(app, true)),
        MENU_QUIT => Ok(app.exit(QUIT_CODE)),
        _ => Ok(()),
    };
    if let Err(err) = done {
        eprintln!("[money24] {}: {err}", event.id.as_ref());
    }
}

/// The supervisor's logs, or the app's own log folder when it runs on its own.
fn open_logs(app: &AppHandle, supervisor: &Supervisor) -> Result<(), String> {
    let dir = match supervisor.logs() {
        Some(dir) => dir,
        None => app.path().app_log_dir().map_err(|e| e.to_string())?,
    };
    std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    app.opener().open_path(dir.to_string_lossy(), None::<&str>).map_err(|e| e.to_string())
}

fn keep_summary(tray: TrayIcon) {
    loop {
        let _ = tray.set_tooltip(Some(summary()));
        thread::sleep(SUMMARY_EVERY);
    }
}

/// "Money 24: all running · 12 open", "Money 24: news, tunnel down · 3 open", or the runner is down.
fn summary() -> String {
    if !local_http::healthy(RUNNER_PORT) {
        return TIP_RUNNER_DOWN.to_string();
    }
    let mut down: Vec<String> = Vec::new();
    if !local_http::healthy(NEWS_PORT) {
        down.push(NEWS_NAME.to_string());
    }
    if let Some(state) = local_http::get_json(RUNNER_PORT, ROUTE_SUPERVISOR) {
        if state["state"] == SUPERVISOR_STALE {
            down.push(SUPERVISOR_NAME.to_string());
        }
        for service in state["services"].as_array().into_iter().flatten() {
            let name = service["name"].as_str().unwrap_or_default();
            let label = SERVICE_NAMES.iter().find(|(id, _)| *id == name).map_or(name, |(_, label)| label);
            if service["up"] == false && !down.iter().any(|d| d == label) {
                down.push(label.to_string());
            }
        }
    }
    let head = if down.is_empty() { TIP_ALL_RUNNING.to_string() } else { format!("{} {TIP_DOWN}", down.join(", ")) };
    format!("{TIP_PREFIX}{head}{TIP_SEPARATOR}{} {TIP_OPEN}", open_positions())
}

fn count(value: &Value) -> u64 {
    value.as_array().map_or(0, |list| list.len() as u64)
}

/// Open positions across the desks: the prediction desk, the meme desk and the options scout.
fn open_positions() -> u64 {
    let predict = local_http::get_json(RUNNER_PORT, ROUTE_PREDICT).map_or(0, |p| p["account"]["open"].as_u64().unwrap_or(0));
    let meme = local_http::get_json(RUNNER_PORT, ROUTE_MEMEDESK)
        .map_or(0, |m| m["strategies"].as_array().into_iter().flatten().map(|s| count(&s["open_positions"])).sum());
    let scout = local_http::get_json(RUNNER_PORT, ROUTE_SCOUT).map_or(0, |s| count(&s["open"]));
    predict + meme + scout
}
