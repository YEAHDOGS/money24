// The pocket: a small window over the tray with every service, recorder and desk at a glance
// (pocket.html, src/pocket/). Frameless and always on top, placed above the tray on the screen the
// taskbar is on, and hidden as soon as it loses focus, the way the taskbar's own flyouts behave.

use std::sync::Mutex;
use std::time::{Duration, Instant};

use tauri::{AppHandle, Manager, PhysicalPosition, WebviewUrl, WebviewWindow, WindowEvent};

pub const POCKET_LABEL: &str = "pocket";
const POCKET_PAGE: &str = "pocket.html";
// The page shows its stop question when its address ends in this.
const CONFIRM_STOP_HASH: &str = "#stop";
const NO_HASH: &str = "";
const POCKET_TITLE: &str = "Money 24 status";
const SHOWN_SCRIPT: &str = "window.dispatchEvent(new Event('pocket-shown'))";
const POCKET_SIZE: (f64, f64) = (380.0, 520.0);
// Logical pixels between the pocket and the edges of the work area (the screen less the taskbar).
const POCKET_GAP: f64 = 12.0;
// A tray click that comes this soon after the pocket hid is the click that hid it: it stays hidden.
const REOPEN_GUARD: Duration = Duration::from_millis(350);

/// Where the last tray click was, and when the pocket last hid.
#[derive(Default)]
pub struct Pocket(Mutex<PocketState>);

#[derive(Default)]
struct PocketState {
    anchor: Option<PhysicalPosition<f64>>,
    hidden_at: Option<Instant>,
}

/// A left click on the tray icon: shows the pocket over it, or hides it when it is showing.
pub fn toggle(app: &AppHandle, click: PhysicalPosition<f64>) {
    let state = app.state::<Pocket>();
    let just_hid = {
        let mut s = state.0.lock().unwrap();
        s.anchor = Some(click);
        s.hidden_at.is_some_and(|at| at.elapsed() < REOPEN_GUARD)
    };
    let window = app.get_webview_window(POCKET_LABEL);
    if window.as_ref().is_some_and(|w| w.is_visible().unwrap_or(false)) {
        hide(app);
        return;
    }
    if just_hid {
        return;
    }
    show(app, false);
}

/// Shows the pocket over the last tray click (or the corner of the main screen). confirm_stop: it opens
/// on the question that asks before every service is stopped.
pub fn show(app: &AppHandle, confirm_stop: bool) {
    let window = match app.get_webview_window(POCKET_LABEL) {
        Some(window) => window,
        None => match build(app, confirm_stop) {
            Ok(window) => window,
            Err(err) => {
                eprintln!("[money24] the pocket could not open: {err}");
                return;
            }
        },
    };
    // The stop question, or (when it was left open and the pocket hid) back to the status.
    let hash = if confirm_stop { CONFIRM_STOP_HASH } else { NO_HASH };
    let _ = window.eval(format!("location.hash = '{hash}'"));
    let anchor = app.state::<Pocket>().0.lock().unwrap().anchor;
    if let Err(err) = place(&window, anchor) {
        eprintln!("[money24] the pocket could not be placed: {err}");
    }
    let _ = window.show();
    let _ = window.set_focus();
    // The page reads everything again at once and plays its entrance (src/pocket/pocket-client.svelte.js).
    let _ = window.eval(SHOWN_SCRIPT);
}

pub fn hide(app: &AppHandle) {
    let Some(window) = app.get_webview_window(POCKET_LABEL) else {
        return;
    };
    let _ = window.hide();
    app.state::<Pocket>().0.lock().unwrap().hidden_at = Some(Instant::now());
}

fn build(app: &AppHandle, confirm_stop: bool) -> tauri::Result<WebviewWindow> {
    let page = if confirm_stop { format!("{POCKET_PAGE}{CONFIRM_STOP_HASH}") } else { POCKET_PAGE.to_string() };
    let window = crate::app_window(app, POCKET_LABEL, WebviewUrl::App(page.into()))
        .title(POCKET_TITLE)
        .inner_size(POCKET_SIZE.0, POCKET_SIZE.1)
        .decorations(false)
        .resizable(false)
        .always_on_top(true)
        .skip_taskbar(true)
        .visible(false)
        .background_color(crate::WINDOW_BACKGROUND)
        .build()?;
    let handle = app.clone();
    window.on_window_event(move |event| {
        if let WindowEvent::Focused(false) = event {
            hide(&handle);
        }
    });
    Ok(window)
}

/// Puts the pocket above the tray click, inside the work area of the screen the click was on. A
/// taskbar at the top, left or right works too: the pocket opens below the click and is kept inside.
fn place(window: &WebviewWindow, anchor: Option<PhysicalPosition<f64>>) -> tauri::Result<()> {
    let near = match anchor {
        Some(at) => window.monitor_from_point(at.x, at.y)?,
        None => None,
    };
    let Some(monitor) = near.or(window.primary_monitor()?) else {
        return Ok(());
    };
    let scale = monitor.scale_factor();
    let area = monitor.work_area();
    let (width, height, gap) = (POCKET_SIZE.0 * scale, POCKET_SIZE.1 * scale, POCKET_GAP * scale);
    let (left, top) = (f64::from(area.position.x) + gap, f64::from(area.position.y) + gap);
    let right = (f64::from(area.position.x) + f64::from(area.size.width) - width - gap).max(left);
    let bottom = (f64::from(area.position.y) + f64::from(area.size.height) - height - gap).max(top);
    let middle = f64::from(area.position.y) + f64::from(area.size.height) / 2.0;
    let (x, y) = match anchor {
        Some(at) if at.y < middle => (at.x - width / 2.0, at.y + gap),
        Some(at) => (at.x - width / 2.0, at.y - height - gap),
        None => (right, bottom),
    };
    window.set_position(PhysicalPosition::new(x.clamp(left, right).round() as i32, y.clamp(top, bottom).round() as i32))
}
