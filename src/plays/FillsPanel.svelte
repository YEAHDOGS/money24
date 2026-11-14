<script>
  /**
   * What the paper runner did with a play: each exit it sold, in order, and how the play ended.
   * The watcher prices open plays every 30 seconds; this list fills in as it goes.
   */
  import strings from './strings.json';
  import { pnl, price, clock, realized, signedPct } from './play-format.js';

  /** @type {{ play: any }} */
  let { play } = $props();

  const total = $derived(realized(play));
</script>

<ol class="mt-2 grid gap-1 border-l border-paper/40 pl-3">
  <li class="font-mono text-xs tabular-nums text-paper">
    {clock(play.armed_at)} · {play.qty} @ {price(play.entry)}
  </li>
  {#each play.fills ?? [] as f, i (i)}
    <li class="grid grid-cols-[minmax(0,1fr)_auto] gap-x-2 font-mono text-xs tabular-nums">
      <span class="truncate text-white/80">{clock(f.at)} · {strings.fillReasons[f.reason] ?? f.reason} · {f.qty} @ {price(f.price)}</span>
      <span class={f.pnl_usd < 0 ? 'text-bad' : 'text-ok'}>{pnl(f.pnl_usd)}</span>
    </li>
  {:else}
    <li class="text-xs text-muted">{strings.detail.noFills}</li>
  {/each}
</ol>
{#if play.result}
  <p class="mt-3 font-mono text-sm font-bold tabular-nums {total < 0 ? 'text-bad' : 'text-ok'}">
    {strings.detail.result}: {pnl(total)} <span class="text-muted">{signedPct(play.result.pnl_pct)} · {strings.fillReasons[play.result.reason] ?? play.result.reason}</span>
  </p>
{/if}
