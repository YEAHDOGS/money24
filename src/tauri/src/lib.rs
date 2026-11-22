// Money 24 app shell, shared by desktop and Android.
// Desktop: the owner's window into the running system. It owns Money 24's tray icon (tray.rs): a left
// click opens the pocket, a small status window over the tray (pocket.rs, pocket.html), and "Open the
// desk" opens the desk in the app's main window, never in the browser. It attaches to the services the
// supervisor (scripts/forever.js) already runs, and starts its own newsd and m24d as sidecars only when
// nothing runs (engines.rs). --tray starts it with only the tray; a second launch focuses this one.
// Android: no engines on the phone. The window opens Home, which talks to your runner (VPS or home
// PC) at the URL saved in the Runner settings on the Plays page; Home links there while it is offline.

use tauri::{AppHandle, Url, WebviewUrl, WebviewWindowBuilder, Wry};
use tauri_plugin_opener::OpenerExt;

#[cfg(desktop)]
mod engines;
#[cfg(desktop)]
mod local_http;
#[cfg(desktop)]
mod pocket;
#[cfg(desktop)]
mod supervisor;
#[cfg(desktop)]
mod tray;

#[cfg(desktop)]
use tauri::{window::Color, Manager, RunEvent};

const WINDOW_LABEL: &str = "main";
#[cfg(desktop)]
const WINDOW_TITLE: &str = "Money 24";
// Every shell opens on Home: open positions first, then the account and the news.
const START_PAGE: &str = "home.html";
// The desk the supervisor's runner serves, rebuilt each time the supervisor starts: in attach mode the
// main window opens it, so the app always shows the newest pages. The page checked first, then the address.
#[cfg(desktop)]
const RUNNER_DESK_PAGE: &str = "/desk/home.html";
#[cfg(desktop)]
const RUNNER_DESK_URL: &str = "http://127.0.0.1:8025/desk/";
#[cfg(desktop)]
const WINDOW_SIZE: (f64, f64) = (1280.0, 800.0);
#[cfg(desktop)]
const WINDOW_MIN_SIZE: (f64, f64) = (320.0, 320.0);
// Matches --color-app in src/styles/theme.css, so a window never flashes white while loading.
#[cfg(desktop)]
pub(crate) const WINDOW_BACKGROUND: Color = Color(5, 5, 8, 255);
// Hosts the windows may load. Every other link opens in the system browser.
const APP_HOSTS: [&str; 3] = ["tauri.localhost", "localhost", "127.0.0.1"];
const APP_SCHEME: &str = "tauri";
// Starts the app with only its tray icon; the supervisor launches it this way at logon.
#[cfg(desktop)]
const TRAY_FLAG: &str = "--tray";
// The supervisor runs the services: never start the app's own, even for a moment while one restarts.
#[cfg(desktop)]
const ATTACH_FLAG: &str = "--attach";
// Shows the pocket over the tray, as a click on the icon does: `money24.exe --pocket` while the app runs.
#[cfg(desktop)]
const POCKET_FLAG: &str = "--pocket";

/// What the desktop shell found at launch: attached or on its own, and where the supervisor lives.
#[cfg(desktop)]
pub(crate) struct Shell {
    pub mode: engines::Mode,
    pub supervisor: supervisor::Supervisor,
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let builder = tauri::Builder::default();
    // Single instance: a second launch (the Start menu while the supervisor's copy runs, or a double
    // click on the exe) would add a second tray icon and fight over the windows. It hands its
    // arguments to this one and exits: the desk opens here, unless it asked for the tray alone.
    #[cfg(desktop)]
    let builder = builder.plugin(tauri_plugin_single_instance::init(|app, args, _cwd| {
        if args.iter().any(|a| a == POCKET_FLAG) {
            pocket::show(app, false);
            return;
        }
        if args.iter().any(|a| a == TRAY_FLAG) {
            return;
        }
        if let Err(err) = open_desk(app) {
            eprintln!("[money24] could not open the desk: {err}");
        }
    }));
    let builder = builder.plugin(tauri_plugin_shell::init()).plugin(tauri_plugin_opener::init());
    #[cfg(desktop)]
    let builder = builder
        .manage(engines::Engines::new())
        .manage(pocket::Pocket::default())
        .invoke_handler(tauri::generate_handler![open_desk_command, hide_pocket, stop_money24, shell_info]);
    let app = builder
        .setup(|app| {
            #[cfg(desktop)]
            {
                let args: Vec<String> = std::env::args().collect();
                let mode = engines::attach_or_start(app.handle(), args.iter().any(|a| a == ATTACH_FLAG))?;
                let supervisor = supervisor::Supervisor::find(app.handle(), &args);
                tray::build(app.handle(), mode, &supervisor)?;
                app.manage(Shell { mode, supervisor });
                if !args.iter().any(|a| a == TRAY_FLAG) {
                    open_desk(app.handle())?;
                }
            }
            #[cfg(mobile)]
            app_window(app.handle(), WINDOW_LABEL, WebviewUrl::App(START_PAGE.into())).build()?;
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("Money 24 failed to start");
    app.run(|_app, _event| {
        #[cfg(desktop)]
        match _event {
            // Closing the desk window leaves the app in the tray; only Quit (an exit with a code) ends it.
            RunEvent::ExitRequested { code: None, api, .. } => api.prevent_exit(),
            RunEvent::Exit => engines::stop(_app),
            _ => {}
        }
    });
}

fn is_app_url(url: &Url) -> bool {
    url.scheme() == APP_SCHEME || url.host_str().is_some_and(|host| APP_HOSTS.contains(&host))
}

/// A window that keeps the app's pages in it and sends every link that leaves them (headlines,
/// posts) to the system browser.
pub(crate) fn app_window<'a>(app: &'a AppHandle, label: &str, url: WebviewUrl) -> WebviewWindowBuilder<'a, Wry, AppHandle> {
    let handle = app.clone();
    WebviewWindowBuilder::new(app, label, url).on_navigation(move |url| {
        if is_app_url(url) {
            return true;
        }
        let _ = handle.opener().open_url(url.as_str(), None::<&str>);
        false
    })
}

/// Opens the desk in the main window, or brings the open one to the front. Attached to the
/// supervisor's runner, the window shows the pages it serves; otherwise the app's own.
#[cfg(desktop)]
pub(crate) fn open_desk(app: &AppHandle) -> tauri::Result<()> {
    pocket::hide(app);
    if let Some(window) = app.get_webview_window(WINDOW_LABEL) {
        window.unminimize()?;
        window.show()?;
        return window.set_focus();
    }
    let served = local_http::get(engines::RUNNER_PORT, RUNNER_DESK_PAGE).is_some();
    let url = match RUNNER_DESK_URL.parse() {
        Ok(desk) if served => WebviewUrl::External(desk),
        _ => WebviewUrl::App(START_PAGE.into()),
    };
    app_window(app, WINDOW_LABEL, url)
        .title(WINDOW_TITLE)
        .inner_size(WINDOW_SIZE.0, WINDOW_SIZE.1)
        .min_inner_size(WINDOW_MIN_SIZE.0, WINDOW_MIN_SIZE.1)
        .background_color(WINDOW_BACKGROUND)
        .build()?;
    Ok(())
}

/// The pocket's "Open the desk" button.
#[cfg(desktop)]
#[tauri::command]
fn open_desk_command(app: AppHandle) -> Result<(), String> {
    open_desk(&app).map_err(|e| e.to_string())
}

/// The pocket's Escape key and its Cancel button.
#[cfg(desktop)]
#[tauri::command]
fn hide_pocket(app: AppHandle) {
    pocket::hide(&app);
}

/// The pocket's answer to "Stop every service?": yes.
#[cfg(desktop)]
#[tauri::command]
fn stop_money24(app: AppHandle) -> Result<(), String> {
    app.state::<Shell>().supervisor.stop().map_err(|e| e.to_string())
}

/// What the pocket says about the app itself: attached to running services or running its own, and
/// whether Stop can reach the supervisor.
#[cfg(desktop)]
#[tauri::command]
fn shell_info(app: AppHandle) -> serde_json::Value {
    let shell = app.state::<Shell>();
    serde_json::json!({ "attached": shell.mode == engines::Mode::Attached, "supervisor_known": shell.supervisor.known() })
}
