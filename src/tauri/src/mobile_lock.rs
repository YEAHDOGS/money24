// The decrypted connection never enters native storage. Only its key survives page navigation,
// in process memory, and the OS lifecycle clears that key whenever the app backgrounds.
use std::sync::Mutex;
use tauri::{Manager, State, Window, WindowEvent};

const KEY_BYTES: usize = 32;
const LOCK_SCRIPT: &str = "window.dispatchEvent(new Event('m24-app-lock'))";
pub const INIT_SCRIPT: &str = "Object.defineProperty(window, '__M24_MOBILE_LOCK__', { value: true })";

pub struct MobileLock(Mutex<Session>);

struct Session {
    key: Option<Vec<u8>>,
    active: bool,
}

impl Default for MobileLock {
    fn default() -> Self {
        Self(Mutex::new(Session {
            key: None,
            active: true,
        }))
    }
}

impl Session {
    fn clear(&mut self) {
        if let Some(key) = self.key.as_mut() {
            key.fill(0);
        }
        self.key = None;
    }
}

/// Returns an unlocked session only while the native activity is foregrounded.
#[tauri::command]
pub fn mobile_lock_session(state: State<'_, MobileLock>) -> Option<Vec<u8>> {
    let session = state.0.lock().unwrap();
    if !session.active {
        return None;
    }
    session.key.clone()
}

/// The page derives this key from the password and authenticates the encrypted vault first.
#[tauri::command]
pub fn mobile_unlock_session(state: State<'_, MobileLock>, mut key: Vec<u8>) -> Result<(), String> {
    let mut session = state.0.lock().unwrap();
    if !session.active || key.len() != KEY_BYTES {
        key.fill(0);
        return Err("The app is locked or the key is invalid.".into());
    }
    session.clear();
    session.key = Some(key);
    Ok(())
}

/// Manual locking revokes the key before the page reloads and discards its clients.
#[tauri::command]
pub fn mobile_lock(state: State<'_, MobileLock>) {
    state.0.lock().unwrap().clear();
}

/// Android onPause and onResume events are independent of WebView timer throttling.
pub fn on_window_event(window: &Window, event: &WindowEvent) {
    let state = window.state::<MobileLock>();
    match event {
        WindowEvent::Suspended => {
            let mut session = state.0.lock().unwrap();
            session.active = false;
            session.clear();
        }
        WindowEvent::Resumed => {
            let mut session = state.0.lock().unwrap();
            session.active = true;
            session.clear();
        }
        _ => return,
    }
    if let Some(webview) = window.get_webview_window(window.label()) {
        let _ = webview.eval(LOCK_SCRIPT);
    }
}
