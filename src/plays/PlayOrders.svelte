<script>
  /**
   * A play's orders with their status and exact times: when it was made, the entry, every exit,
   * the stop, the time exit and how it ended. An active play says when it began, what it is worth
   * right now and where it ends for a win or a loss. A finished one that lost says what went wrong.
   */
  import { onMount } from 'svelte';
  import strings from './strings.json';
  import { price, usd, pnl, age } from './play-format.js';
  import { orders } from './orders.js';

  const T = strings.orders;
  const LIVE_REFRESH_MS = 30000; // how often an active play's worth is read again
  /** @type {Intl.DateTimeFormatOptions} */
  const EXACT = { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', second: '2-digit', timeZoneName: 'short' };
  const STATUS_CLASS = {
    made: 'border-line text-white/70', not_placed: 'border-line text-muted', working: 'border-paper text-paper', scheduled: 'border-paper text-paper',
    filled: 'border-ok text-ok', triggered: 'border-bad text-bad', cancelled: 'border-line text-muted',
  };

  /** @type {{ item: import('./plays-client.svelte.js').Item, client: import('./plays-client.svelte.js').PlaysClient, now: number }} */
  let { item, client, now } = $props();

  const play = $derived(item.play);
  const record = $derived(item.report.record);
  const rows = $derived(orders(play, record.stops));
  const active = $derived(play.status === 'armed');
  const lost = $derived(play.result && play.result.pnl_usd < 0);
  const began = $derived(play.filled?.at ?? play.armed_at);
  /** @type {number | null} what the active play is up or down right now, from its replay */
  let worth = $state(null);

  const exact = (/** @type {string | undefined} */ iso) => (iso ? new Date(iso).toLocaleString(undefined, EXACT) : T.noTime);
  const firstExit = $derived(record.exits.find((/** @type {number} */ x) => x > 0));
  /** What went wrong, in sentences: how it ended, the rules it went through anyway, and the gap to the plan. */
  const wrong = $derived.by(() => {
    if (!lost) return [];
    const out = [T.wrong[play.result.reason] ?? T.wrong.other];
    const broke = (play.broke_rules ?? []).map((/** @type {string} */ id) => strings.rules[id]?.label ?? id);
    if (broke.length) out.push(T.wrongRules.replace('{rules}', broke.join(', ')));
    const planned = item.report.payoff.planned_loss_usd;
    if (Math.abs(play.result.pnl_usd) > planned) out.push(T.wrongSlip.replace('{lost}', usd(Math.abs(play.result.pnl_usd))).replace('{planned}', usd(planned)));
    return out;
  });

  onMount(() => {
    const read = () => {
      if (!active) return;
      client.curve(play.id).then((c) => { worth = c.pnl_usd; }).catch(() => {});
    };
    read();
    const timer = setInterval(read, LIVE_REFRESH_MS);
    return () => clearInterval(timer);
  });
</script>

{#if active}
  <div class="mb-3 border border-paper bg-paper/8 px-3 py-2.5" role="status">
    <p class="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm font-bold text-paper">
      <span class="relative size-2 rounded-full bg-current live-dot"></span>{T.active}
      <span class="font-mono text-xs font-normal text-white/80">{T.began} {exact(began)} · {age(began, now)} {strings.detail.ago}</span>
    </p>
    <dl class="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 font-mono text-xs tabular-nums @xl:grid-cols-4">
      <div><dt class="font-sans text-muted">{T.amountIn}</dt><dd class="text-white">{usd(record.actual_amount_usd)}</dd></div>
      <div><dt class="font-sans text-muted">{T.now}</dt><dd class={worth === null ? 'text-muted' : worth < 0 ? 'text-bad' : 'text-ok'}>{worth === null ? T.reading : pnl(worth)}</dd></div>
      <div><dt class="font-sans text-muted">{T.winAt}</dt><dd class="text-ok">{firstExit ? price(firstExit) : T.none}</dd></div>
      <div><dt class="font-sans text-muted">{T.loseAt}</dt><dd class="text-bad">{price(record.stops[0])} · −{usd(item.report.payoff.planned_loss_usd)}</dd></div>
    </dl>
  </div>
{/if}

<ol class="grid gap-px">
  {#each rows as o, i (o.kind + (o.n ?? '') + i)}
    <li class="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-3 gap-y-0.5 border-b border-line/60 py-1.5 text-sm">
      <span class="flex min-w-0 flex-wrap items-baseline gap-x-2">
        <span class="font-semibold text-white">{T.kinds[o.kind]}{o.n ? ` ${o.n}` : ''}</span>
        {#if o.qty}<span class="font-mono text-xs tabular-nums text-white/70">{o.qty} @ {price(o.price ?? 0)}</span>{:else if o.price}<span class="font-mono text-xs tabular-nums text-white/70">@ {price(o.price)}</span>{/if}
        {#if o.pnl !== undefined && o.kind !== 'made'}<span class="font-mono text-xs tabular-nums {o.pnl < 0 ? 'text-bad' : 'text-ok'}">{pnl(o.pnl)}</span>{/if}
      </span>
      <span class="rounded-sm border px-1.5 py-px text-[0.6rem] font-bold tracking-wider whitespace-nowrap uppercase {STATUS_CLASS[o.status] ?? (o.pnl !== undefined && o.pnl < 0 ? STATUS_CLASS.triggered : STATUS_CLASS.filled)}">{T.status[o.status] ?? strings.fillReasons[o.status] ?? o.status}</span>
      <time class="col-span-2 font-mono text-[0.7rem] tabular-nums text-muted" datetime={o.at}>{o.at ? exact(o.at) : T.noTime}</time>
    </li>
  {/each}
</ol>

{#if wrong.length}
  <div class="mt-3 border-l-2 border-bad pl-3">
    <h4 class="text-[0.62rem] font-bold tracking-[0.18em] text-bad uppercase">{T.wrongTitle}</h4>
    {#each wrong as line (line)}<p class="mt-1 text-sm leading-snug text-white/85">{line}</p>{/each}
  </div>
{/if}
