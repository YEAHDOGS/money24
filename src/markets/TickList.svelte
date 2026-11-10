<script>
  /** A list of tickers with price and day change. Picking one opens it. */
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import strings from './strings.json';
  import { percent, price } from './market-format.js';

  /** @type {{ ticks: any[], current: string, onPick: (symbol: string) => void }} */
  let { ticks, current, onPick } = $props();
</script>

<ul class="divide-y divide-line">
  {#each ticks as t (t.symbol)}
    {@const down = t.change_pct < 0}
    <li>
      <button type="button" onclick={() => onPick(t.symbol)} aria-current={t.symbol === current}
        class="spring flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none {t.symbol === current ? 'bg-white/5' : ''}">
        <span class="min-w-0 flex-1">
          <span class="block truncate text-sm font-semibold text-white">{t.name || t.symbol}</span>
          <span class="block font-mono text-xs tabular-nums text-muted">{price(t.price)} {t.currency}</span>
        </span>
        <span class="flex w-22 shrink-0 items-center justify-center gap-1 rounded-sm py-1 font-mono text-xs font-bold tabular-nums text-app {down ? 'bg-bad' : 'bg-ok'}">
          {#if down}<ArrowDown class="size-3.5" aria-hidden="true" />{:else}<ArrowUp class="size-3.5" aria-hidden="true" />{/if}
          {percent(t.change_pct)}
        </span>
      </button>
    </li>
  {:else}
    <li class="px-4 py-6 text-sm text-muted">{strings.emptyList}</li>
  {/each}
</ul>
