<script>
  /**
   * The price spine: one vertical scale of stock prices, highest at the top, with the stock now, the
   * stop, the breakeven, every rung and the runner hung from a 1px line. Each node shows the stock
   * price it takes, the contract price there, what it sells and makes, and the market's odds.
   * Nodes glide (transform only) when the stake dial or a refresh moves them.
   */
  import strings from './strings.json';
  import { price, pnl, pct, move, levelName } from './play-format.js';
  import { spineLayout, NODE_SPOT, NODE_DECAY } from './spine-layout.js';

  /** @type {{ play: any, report: any }} */
  let { play, report } = $props();

  const LEVEL_STOP = 'stop';
  const LEVEL_BREAKEVEN = 'breakeven';
  const DOT_CLASS = {
    [NODE_SPOT]: 'bg-white border-white',
    [LEVEL_STOP]: 'bg-app border-bad',
    [NODE_DECAY]: 'bg-app border-bad border-dashed',
    [LEVEL_BREAKEVEN]: 'bg-app border-white/70',
    exit: 'bg-ok border-ok',
  };
  const TEXT_CLASS = { [NODE_SPOT]: 'text-white', [LEVEL_STOP]: 'text-bad', [NODE_DECAY]: 'text-bad/80', [LEVEL_BREAKEVEN]: 'text-white/80', exit: 'text-ok' };

  const PERCENT = 100;

  const nodes = $derived(spineLayout(report, play));
  /** The stock move to a node: the stop is measured from today's trigger, every other level by the exit. */
  const moveTo = $derived((/** @type {any} */ n) => {
    const spot = play.market?.spot ?? 0;
    if (n.name !== LEVEL_STOP || !spot) return n.level.move_pct;
    return (n.level.spot_now / spot - 1) * PERCENT;
  });
  const kind = $derived((/** @type {string} */ name) => (DOT_CLASS[name] ? name : 'exit'));
</script>

<div class="relative mt-3 h-[clamp(15rem,40dvh,24rem)]" role="list" aria-label={strings.detail.spine}>
  <span class="absolute top-0 bottom-0 left-[4.6rem] w-px bg-line" aria-hidden="true"></span>
  <div class="absolute inset-x-0 top-0 bottom-9">
    {#each nodes as n (n.key)}
      {@const k = kind(n.name)}
      {@const l = n.level}
      <div class="spine-node spine-at pointer-events-none absolute inset-0" style="--y:{n.y}" role="listitem">
        <div class="pointer-events-auto grid grid-cols-[4rem_1.2rem_minmax(0,1fr)] items-start gap-x-1">
          <span class="text-right font-mono text-xs font-bold tabular-nums leading-4 {TEXT_CLASS[k]}">${price(n.stock)}</span>
          <span class="mt-0.5 ml-[0.05rem] size-3 justify-self-center rounded-full border-2 {DOT_CLASS[k]}" aria-hidden="true"></span>
          <span class="min-w-0 text-xs leading-4">
            {#if n.name === NODE_SPOT}
              <span class="font-bold uppercase tracking-wider text-white">{strings.levels.spot}</span>
            {:else if n.name === NODE_DECAY}
              <span class="text-bad/80">{strings.spine.decayMarker}</span>
            {:else}
              <span class="font-bold uppercase tracking-wider {TEXT_CLASS[k]}">{levelName(n.name)}</span>
              <span class="ml-1 font-mono tabular-nums text-white/70">@{price(l.price)}{l.qty ? ` ×${l.qty}` : ''}</span>
              {#if l.qty}<span class="ml-1 font-mono tabular-nums {l.pnl_usd < 0 ? 'text-bad' : 'text-ok'}">{pnl(l.pnl_usd)}</span>{/if}
              <span class="block truncate font-mono text-[0.65rem] tabular-nums text-muted">
                {n.name === LEVEL_STOP ? strings.spine.triggerNow : l.already_past ? strings.spine.already : `${pct(l.chance_pct)} ${strings.spine.odds}`}
                · {move(moveTo(n))}
              </span>
            {/if}
          </span>
        </div>
      </div>
    {/each}
  </div>
</div>
