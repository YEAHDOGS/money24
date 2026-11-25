import { mount, unmount } from 'svelte';
import LockScreen from './LockScreen.svelte';
import { LOCK_EVENT, isMobileLock, restoreSession, clearSession } from './session.js';
import strings from './strings.json';

/** Every mobile page waits for the vault before constructing clients or reading account data. */
export function mountProtected(component, options) {
  if (!isMobileLock()) return mount(component, options);
  let desk;
  let gate;
  let locking = false;
  let unlocked = false;
  const showDesk = () => {
    if (locking || document.hidden || unlocked) return;
    unlocked = true;
    if (gate) unmount(gate);
    desk = mount(component, options);
  };
  const lock = () => {
    if (locking) return;
    locking = true;
    // Hide synchronously, before any await; reload discards in-flight requests and all client state.
    document.documentElement.style.visibility = 'hidden';
    if (desk) unmount(desk);
    clearSession().finally(() => location.reload());
  };
  window.addEventListener(LOCK_EVENT, lock);
  // A restored back/forward cache must not revive a page holding a previous key.
  window.addEventListener('pageshow', (event) => { if (event.persisted) lock(); });
  gate = mount(LockScreen, { target: document.body, props: { onUnlock: showDesk, booting: true } });
  restoreSession().then((restored) => {
    if (locking) return;
    if (restored) {
      showDesk();
      return;
    }
    unmount(gate);
    gate = mount(LockScreen, { target: document.body, props: { onUnlock: showDesk } });
  }).catch(() => {
    if (locking) return;
    unmount(gate);
    gate = mount(LockScreen, { target: document.body, props: { onUnlock: showDesk, bootError: strings.unavailable } });
  });
  return gate;
}
