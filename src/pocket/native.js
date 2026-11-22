// The desktop app's commands (src/tauri/src/lib.rs), when the pocket runs inside it: Tauri puts them on
// window.__TAURI__ (app.withGlobalTauri). In a browser there is no app, so this is null and the pocket
// links to the desk instead of asking the app to open it.

/** @type {{ invoke: (command: string, args?: object) => Promise<any> } | null} */
export const native = /** @type {any} */ (globalThis).__TAURI__?.core ?? null;

/** Command names, as the app registers them. */
export const COMMAND = {
  openDesk: 'open_desk_command',
  hide: 'hide_pocket',
  stop: 'stop_money24',
  info: 'shell_info',
};
