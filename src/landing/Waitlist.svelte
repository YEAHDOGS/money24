<script>
  /**
   * The waitlist form on the landing page's last panel: one email field and a button. The request
   * lives in waitlist.js. The row keeps its height in every state, so the panel never jumps.
   */
  import { onMount } from "svelte";
  import ArrowRight from "@lucide/svelte/icons/arrow-right";
  import { hasJoined, joinWaitlist } from "./waitlist.js";

  const MAX_EMAIL_LENGTH = 254;
  const LABEL = "Email address";
  const PLACEHOLDER = "you@example.com";
  const JOIN = "Join the waitlist";
  const JOINING = "Joining";
  const NOTE = "One email when Money 24 opens. Nothing else.";
  const JOINED = "You're on the list. We'll email you when it opens.";

  let email = $state("");
  let busy = $state(false);
  let joined = $state(false);
  let error = $state("");

  onMount(() => { joined = hasJoined(); });

  /** @param {SubmitEvent} event */
  async function submit(event) {
    event.preventDefault();
    if (busy) return;
    busy = true;
    error = "";
    try {
      await joinWaitlist(email);
      joined = true;
    } catch (failure) {
      error = failure.message;
    } finally {
      busy = false;
    }
  }
</script>

<div class="waitlist">
  {#if joined}
    <p class="joined" role="status">{JOINED}</p>
  {:else}
    <form class="row" onsubmit={submit} aria-busy={busy}>
      <label class="sr-only" for="waitlist-email">{LABEL}</label>
      <input id="waitlist-email" type="email" name="email" autocomplete="email" required maxlength={MAX_EMAIL_LENGTH} placeholder={PLACEHOLDER} bind:value={email} disabled={busy} aria-describedby="waitlist-note" />
      <button type="submit" disabled={busy}>{busy ? JOINING : JOIN}<ArrowRight size={14} strokeWidth={2.5} aria-hidden="true" /></button>
    </form>
  {/if}
  <p id="waitlist-note" class="note" class:bad={error !== ""} aria-live="polite">{error || (joined ? "" : NOTE)}</p>
</div>

<style>
  .waitlist {
    width: 100%;
    max-width: 30rem;
    margin-top: 0.4rem;
  }
  .row {
    display: flex;
    gap: 0.5rem;
    height: 2.75rem;
  }
  input {
    flex: 1;
    min-width: 0;
    padding: 0 0.9rem;
    border: 1px solid var(--step-border);
    background: var(--step-bg);
    color: white;
    font-size: 1rem;
  }
  input::placeholder { color: var(--color-muted); }
  input:focus-visible { outline: 2px solid var(--color-neon-red); outline-offset: 2px; }
  button {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0 1rem;
    border: 1px solid var(--color-neon-red);
    background: var(--color-neon-red);
    color: white;
    font-size: 0.74rem;
    font-weight: 700;
    letter-spacing: 0.2em;
    text-transform: uppercase;
    white-space: nowrap;
    cursor: pointer;
    transition: transform 150ms cubic-bezier(0.34, 1.56, 0.64, 1), background-color 150ms ease, border-color 150ms ease, color 150ms ease;
  }
  button:hover, button:focus-visible { background: white; border-color: white; color: var(--color-app); }
  button:active { transform: scale(0.96); }
  button:disabled, input:disabled { opacity: 0.6; cursor: default; }
  .joined {
    display: flex;
    align-items: center;
    height: 2.75rem;
    font-weight: 700;
  }
  .note {
    min-height: 1.2rem;
    margin-top: 0.5rem;
    font-size: 0.72rem;
    letter-spacing: 0.14em;
    color: var(--color-muted);
  }
  .note.bad { color: var(--color-neon-red); }
  /* Narrow phones: the button drops its letter-spacing so the field keeps room to type in. */
  @media (max-width: 420px) {
    button { padding: 0 0.7rem; letter-spacing: 0.08em; }
  }
</style>
