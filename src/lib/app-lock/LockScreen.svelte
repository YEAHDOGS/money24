<script>
  import LockKeyhole from '@lucide/svelte/icons/lock-keyhole';
  import strings from './strings.json';
  import { MIN_PASSWORD_LENGTH } from './vault.js';
  import { STORE_VAULT, unlockSession } from './session.js';

  const DEFAULT_RUNNER = 'https://';
  const STORE_RUNNER = 'm24.trading.runner';
  const STORE_SECRET = 'm24.trading.secret';
  let { onUnlock, booting = false, bootError = '' } = $props();
  let password = $state('');
  let confirm = $state('');
  let runner = $state(DEFAULT_RUNNER);
  let secret = $state('');
  let error = $state('');
  let busy = $state(false);
  let setup = $state(false);

  $effect(() => {
    try {
      setup = !localStorage.getItem(STORE_VAULT);
      if (!setup) return;
      runner = localStorage.getItem(STORE_RUNNER) || DEFAULT_RUNNER;
      secret = localStorage.getItem(STORE_SECRET) || '';
    } catch {
      error = strings.unavailable;
    }
  });

  /** Unlocking never mounts the desk until authenticated decryption has succeeded. */
  async function submit(event) {
    event.preventDefault();
    if (busy) return;
    error = '';
    if (setup && password !== confirm) {
      error = strings.mismatch;
      return;
    }
    busy = true;
    try {
      await unlockSession(password, setup ? { runner: runner.trim(), secret: secret.trim() } : null);
      password = confirm = secret = '';
      onUnlock();
    } catch (reason) {
      error = setup ? reason.message : strings.failed;
      password = confirm = '';
    } finally {
      busy = false;
    }
  }
</script>

<div class="fixed inset-0 z-[1000] flex flex-col bg-app text-white" aria-label={strings.brand}>
  <header class="flex shrink-0 items-center gap-2 border-b border-line px-5 py-3">
    <LockKeyhole class="size-4 text-neon-red" aria-hidden="true" />
    <span class="text-sm font-bold">{strings.brand}</span>
  </header>
  <main class="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 py-5 sm:py-10">
    <form class="mx-auto grid w-full max-w-md gap-4" onsubmit={submit}>
      <div>
        <h1 class="text-2xl font-bold tracking-tight">{setup ? strings.setupTitle : strings.unlockTitle}</h1>
        <p class="mt-2 text-sm leading-relaxed text-muted">{setup ? strings.setupHint : strings.unlockHint}</p>
      </div>
      {#if booting}
        <p role="status" class="text-sm text-muted">{strings.boot}</p>
      {:else}
        <label class="grid gap-2 text-sm">{strings.password}
          <input type="password" bind:value={password} autocomplete={setup ? 'new-password' : 'current-password'} required minlength={setup ? MIN_PASSWORD_LENGTH : undefined} disabled={busy}
            class="min-h-11 min-w-0 rounded-sm border border-line bg-card px-3 text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red" />
        </label>
        {#if setup}
          <label class="grid gap-2 text-sm">{strings.confirm}
            <input type="password" bind:value={confirm} autocomplete="new-password" required minlength={MIN_PASSWORD_LENGTH} disabled={busy}
              class="min-h-11 min-w-0 rounded-sm border border-line bg-card px-3 text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red" />
          </label>
          <p class="text-xs leading-relaxed text-muted">{strings.length}</p>
          <label class="grid gap-2 text-sm">{strings.runner}
            <input type="url" bind:value={runner} autocomplete="off" spellcheck="false" required disabled={busy}
              class="min-h-11 min-w-0 rounded-sm border border-line bg-card px-3 text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red" />
          </label>
          <label class="grid gap-2 text-sm">{strings.secret}
            <input type="password" bind:value={secret} autocomplete="off" disabled={busy}
              class="min-h-11 min-w-0 rounded-sm border border-line bg-card px-3 text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red" />
          </label>
          <p class="text-xs leading-relaxed text-muted">{strings.runnerHint}</p>
        {/if}
        <p role="alert" class="min-h-5 text-sm text-bad">{error || bootError}</p>
        <button type="submit" disabled={busy || !!bootError}
          class="spring min-h-11 rounded-sm bg-neon-red px-4 py-2 text-sm font-bold text-app hover:opacity-90 focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red disabled:opacity-50">
          {busy ? strings.busy : setup ? strings.setup : strings.unlock}
        </button>
        {#if setup}<p class="text-xs leading-relaxed text-muted">{strings.scope}</p>{/if}
      {/if}
    </form>
  </main>
</div>
