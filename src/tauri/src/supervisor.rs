// The supervisor (scripts/forever.js) and its scheduled task: where the repository is, where its logs
// are, and Restart and Stop, done the way scripts/tray.ps1 does them. The supervisor stops the app
// when it stops, so each action runs in a process of its own that outlives the app: started through
// `start /b` from a throwaway cmd, it is no longer the app's child, and a forced stop of the
// supervisor's process tree (forever.js --stop, taskkill /T) cannot reach it either.

use std::fs;
use std::io;
use std::path::{Path, PathBuf};
use std::process::Command;

use tauri::{AppHandle, Manager};

const ROOT_FLAG: &str = "--root";
// The last repository the supervisor named, so a later launch from the Start menu still finds it.
const ROOT_FILE: &str = "supervisor-root.txt";
const FOREVER: [&str; 2] = ["scripts", "forever.js"];
const LOGS: [&str; 2] = [".run", "logs"];
const TASK_NAME: &str = "Money24";
const STOP_FLAG: &str = "--stop";
const NO_BUILD_FLAG: &str = "--no-build";
const NODE: &str = "node";
#[cfg(windows)]
const CREATE_NO_WINDOW: u32 = 0x0800_0000;

pub struct Supervisor {
    root: Option<PathBuf>,
}

fn forever(root: &Path) -> PathBuf {
    FOREVER.iter().fold(root.to_path_buf(), |path, part| path.join(part))
}

/// A folder is the repository when it has the supervisor script in it.
fn valid_root(path: PathBuf) -> Option<PathBuf> {
    forever(&path).is_file().then_some(path)
}

impl Supervisor {
    /// The repository from --root, saved for next time, or the one saved last time.
    pub fn find(app: &AppHandle, args: &[String]) -> Self {
        let saved = app.path().app_data_dir().ok().map(|dir| dir.join(ROOT_FILE));
        let given = args.iter().position(|a| a == ROOT_FLAG).and_then(|i| args.get(i + 1)).map(PathBuf::from);
        if let Some(root) = given.and_then(valid_root) {
            if let Some(file) = &saved {
                let _ = file.parent().map(fs::create_dir_all);
                let _ = fs::write(file, root.to_string_lossy().as_bytes());
            }
            return Supervisor { root: Some(root) };
        }
        let root = saved.and_then(|file| fs::read_to_string(file).ok()).map(|s| PathBuf::from(s.trim())).and_then(valid_root);
        Supervisor { root }
    }

    pub fn known(&self) -> bool {
        self.root.is_some()
    }

    /// The supervisor's logs folder, when the repository is known.
    pub fn logs(&self) -> Option<PathBuf> {
        let root = self.root.as_ref()?;
        Some(LOGS.iter().fold(root.clone(), |path, part| path.join(part)))
    }

    /// Stops every service, then starts them again through the scheduled task (or directly without one).
    pub fn restart(&self) -> io::Result<()> {
        let root = self.root.as_ref().ok_or_else(unknown)?;
        let script = forever(root);
        let start = if task_installed() {
            format!("schtasks /Run /TN {TASK_NAME}")
        } else {
            format!("{NODE} \"{}\" {NO_BUILD_FLAG}", script.display())
        };
        detached(root, &format!("{NODE} \"{}\" {STOP_FLAG} & {start}", script.display()))
    }

    /// Stops the supervisor and every service. It stops this app too.
    pub fn stop(&self) -> io::Result<()> {
        let root = self.root.as_ref().ok_or_else(unknown)?;
        detached(root, &format!("{NODE} \"{}\" {STOP_FLAG}", forever(root).display()))
    }
}

fn unknown() -> io::Error {
    io::Error::new(io::ErrorKind::NotFound, "the supervisor's folder is not known: start Money 24 with npm run forever once")
}

#[cfg(windows)]
fn task_installed() -> bool {
    use std::os::windows::process::CommandExt;
    Command::new("schtasks")
        .args(["/Query", "/TN", TASK_NAME])
        .creation_flags(CREATE_NO_WINDOW)
        .output()
        .is_ok_and(|out| out.status.success())
}

#[cfg(not(windows))]
fn task_installed() -> bool {
    false
}

/// Runs a command line, hidden, in a process that outlives the app.
#[cfg(windows)]
fn detached(root: &Path, line: &str) -> io::Result<()> {
    use std::os::windows::process::CommandExt;
    // The outer cmd starts the inner one with no window of its own and exits; the inner one strips
    // the outer quotes from "line" and runs it, & and all.
    Command::new("cmd")
        .raw_arg(format!("/d /c start \"\" /b cmd /d /c \"{line}\""))
        .current_dir(root)
        .creation_flags(CREATE_NO_WINDOW)
        .spawn()
        .map(|_| ())
}

#[cfg(not(windows))]
fn detached(root: &Path, line: &str) -> io::Result<()> {
    let line = line.replace(" & ", " ; ").replace('\'', "'\\''");
    Command::new("sh").arg("-c").arg(format!("nohup sh -c '{line}' >/dev/null 2>&1 &")).current_dir(root).spawn().map(|_| ())
}
