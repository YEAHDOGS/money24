<script>
  /**
   * Crowd room: how many of the 10,000 players this play's contract can take, why, and what covers the rest
   * (matching contracts across strikes and names). The math is in crowd.js.
   */
  import strings from './strings.json';
  import { CROWD_MAX, crowdRoom } from './crowd.js';
  import { price } from './play-format.js';

  /** @type {{ item: any, bare?: boolean }} bare: the caller already wrote the heading */
  let { item, bare = false } = $props();

  const S = strings.crowd;
  const TONES = { fits: 'text-ok', basket: 'text-warn', thin: 'text-warn', closed: 'text-bad' };
  const room = $derived(crowdRoom(item.play.market, item.play.entry));
  const nf = new Intl.NumberFormat('en-US');
</script>

{#if !bare}<h3 class="text-[0.62rem] font-bold tracking-[0.18em] text-muted uppercase">{S.title}</h3>{/if}
<p class="mt-2 text-sm font-semibold {TONES[room.room]}">{S.verdict[room.room]}</p>
<p class="mt-0.5 font-mono text-2xl font-semibold tabular-nums">{nf.format(room.players)} <span class="font-sans text-xs font-normal text-muted">{S.of} {nf.format(CROWD_MAX)} {S.players}</span></p>
<!-- The share of the crowd this contract holds: one bar, grown with a transform. -->
<span class="mt-1.5 block h-1 overflow-hidden bg-line" aria-hidden="true"><span class="block h-1 origin-left bg-neon-red transition-transform duration-500 ease-out" style:transform="scaleX({room.share})"></span></span>
<p class="mt-2 text-xs leading-snug text-white/75">{S.explain[room.room].replace('{copies}', Number.isFinite(room.copies) ? nf.format(room.copies) : '')}</p>
<dl class="mt-2 grid grid-cols-2 gap-x-3 gap-y-0.5 text-xs">
  <dt class="text-muted">{S.oi}</dt><dd class="text-right font-mono tabular-nums">{nf.format(room.oiCap)}</dd>
  <dt class="text-muted">{S.volume}</dt><dd class="text-right font-mono tabular-nums">{room.volumeCap === null ? '—' : nf.format(room.volumeCap)}</dd>
  <dt class="text-muted">{S.collar}</dt><dd class="text-right font-mono tabular-nums">{price(room.collar.low)} – {price(room.collar.high)}</dd>
</dl>
<p class="mt-2 text-[0.65rem] leading-snug text-muted">{S.order}</p>
