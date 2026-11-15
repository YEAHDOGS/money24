<script>
  /**
   * One play, top to bottom: what must happen for it to pay, the stake dial (every number below
   * follows it), the price spine, the payoff and odds, the rules, the evidence, the bracket or the
   * paper fills, and the AI reads. The disclaimer stays pinned under it.
   */
  import { untrack } from 'svelte';
  import strings from './strings.json';
  import { mustHappen, price, compact, age, pct, fill } from './play-format.js';
  import StakeDial from './StakeDial.svelte';
  import SuccessRate from './SuccessRate.svelte';
  import LevelSpine from './LevelSpine.svelte';
  import OddsPanel from './OddsPanel.svelte';
  import RuleChecks from './RuleChecks.svelte';
  import EvidenceList from './EvidenceList.svelte';
  import BracketEditor from './BracketEditor.svelte';
  import FillsPanel from './FillsPanel.svelte';
  import AiPanel from './AiPanel.svelte';
  import Play from '@lucide/svelte/icons/play';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import Sparkles from '@lucide/svelte/icons/sparkles';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import LoaderCircle from '@lucide/svelte/icons/loader-circle';

  /** @type {{ item: import('./plays-client.svelte.js').Item, client: import('./plays-client.svelte.js').PlaysClient, now: number }} */
  let { item, client, now } = $props();

  const LINE_CLASS = { gain: 'text-ok', loss: 'text-bad', neutral: 'text-white/80', warn: 'text-warn', bad: 'text-bad' };
  const LINE_MARK = { gain: '▲', loss: '▼', neutral: '•', warn: '!', bad: '×' };
  const STATUS_CLASS = { draft: 'border-line text-white/70', armed: 'border-paper text-paper', closed: 'border-muted text-muted' };
  const SECTION_LABEL = 'text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted';
  const PERCENT = 100;

  /** @type {import('./plays-client.svelte.js').Item | null} the stake dial's unsaved view */
  let preview = $state(null);
  let busy = $state('');
  let error = $state('');
  let provider = $state(untrack(() => client.providers.find((p) => p.connected)?.id ?? ''));

  const shown = $derived(preview ?? item);
  const play = $derived(shown.play);
  const report = $derived(shown.report);
  const draft = $derived(item.play.status === 'draft');
  const lines = $derived(mustHappen(play, report));
  const market = $derived(play.market);
  const connected = $derived(client.providers.filter((p) => p.connected));
  const analyzing = $derived(item.play.analyzing);

  /** Runs one action with a busy flag and shows its error. */
  async function act(/** @type {string} */ name, /** @type {() => Promise<any>} */ fn) {
    if (busy) return;
    busy = name;
    error = '';
    try {
      await fn();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = '';
    }
  }

  function remove() {
    if (!confirm(strings.detail.deleteConfirm)) return;
    act('delete', () => client.remove(item.play.id));
  }

  /** Saves the play as edited (bracket, evidence, AI suggestion); the dial's preview is dropped. */
  function save(/** @type {any} */ next) {
    return act('save', async () => {
      await client.save(next);
      preview = null;
    });
  }
</script>

<!-- Vertical scrolling only, with the scrollbar's room always kept: the sections' entrance can't make
     it pop in, narrow the page and flash a sideways bar. -->
<div class="list-scroll min-h-0 flex-1 overflow-x-hidden overflow-y-auto [scrollbar-gutter:stable]">
  <!-- HEADER: the play on the left, its success rate big on the right -->
  <header class="reveal flex items-start gap-4 border-b border-line px-4 py-4 lg:gap-6 lg:px-6" style="--i:0">
    <div class="min-w-0 flex-1">
    <div class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
      <h2 class="font-mono text-xl font-bold text-white">{play.title}</h2>
      <span class="rounded-sm border px-1.5 py-px text-[0.65rem] font-bold uppercase tracking-wider {STATUS_CLASS[play.status]}">{strings.playStatus[play.status]}</span>
    </div>
    {#if play.theme}<p class="mt-0.5 text-xs text-muted">{strings.detail.theme}: {play.theme}</p>{/if}
    {#if item.play.broke_rules?.length}
      <p class="mt-2 flex items-start gap-1.5 rounded-sm border border-warn/40 bg-warn/5 px-2 py-1.5 text-xs leading-snug text-warn">
        <TriangleAlert class="mt-px size-3.5 shrink-0" aria-hidden="true" />
        {fill(strings.detail.broke, { n: item.play.broke_rules.length, list: item.play.broke_rules.map((id) => strings.rules[id]?.label ?? id).join(', ') })}
      </p>
    {/if}

    <div class="mt-3 flex flex-wrap gap-2">
      {#if draft}
        <button type="button" onclick={() => act('paper', () => client.paper(item.play.id))} disabled={!!busy}
          class="spring flex items-center gap-1.5 rounded-sm bg-neon-red px-3.5 py-2 text-xs font-bold uppercase tracking-wider text-app hover:brightness-110 focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-white disabled:opacity-40">
          <Play class="size-3.5" aria-hidden="true" />{busy === 'paper' ? strings.detail.paperBusy : strings.detail.paperNow}
        </button>
      {/if}
      <button type="button" onclick={() => act('refresh', () => client.refresh(item.play.id))} disabled={!!busy}
        class="spring flex items-center gap-1.5 rounded-sm border border-line px-3 py-2 text-xs font-bold uppercase tracking-wider text-white/80 hover:border-white/40 focus-visible:border-neon-red focus-visible:outline-none disabled:opacity-40">
        <RefreshCw class="size-3.5 {busy === 'refresh' ? 'spin' : ''}" aria-hidden="true" />{strings.detail.refresh}
      </button>
      {#if connected.length}
        <span class="flex items-stretch">
          <button type="button" onclick={() => act('analyze', () => client.analyze(item.play.id, provider))} disabled={!!busy || !!analyzing}
            class="spring flex items-center gap-1.5 rounded-l-sm border border-kind-insider/60 px-3 py-2 text-xs font-bold uppercase tracking-wider text-kind-insider hover:bg-kind-insider/10 focus-visible:outline-none focus-visible:bg-kind-insider/10 disabled:opacity-40">
            {#if analyzing}<LoaderCircle class="spin size-3.5" aria-hidden="true" />{:else}<Sparkles class="size-3.5" aria-hidden="true" />{/if}
            {analyzing ? strings.detail.analyzing : strings.detail.analyze}
          </button>
          <select bind:value={provider} aria-label={strings.inbox.provider}
            class="max-w-28 rounded-r-sm border border-l-0 border-kind-insider/60 bg-card px-1 text-xs text-white focus:border-neon-red focus:outline-none">
            {#each connected as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
          </select>
        </span>
      {/if}
      <button type="button" onclick={remove} disabled={!!busy} aria-label={strings.detail.delete}
        class="spring ml-auto flex items-center gap-1.5 rounded-sm border border-line px-2.5 py-2 text-xs text-white/60 hover:border-bad hover:text-bad focus-visible:border-bad focus-visible:outline-none disabled:opacity-40">
        <Trash2 class="size-3.5" aria-hidden="true" /><span class="max-sm:hidden">{strings.detail.delete}</span>
      </button>
    </div>
    <p class="mt-1.5 min-h-4 text-xs text-bad" role="alert">{error}</p>

    {#if market}
      <dl class="mt-1 grid grid-cols-[repeat(auto-fill,minmax(7.5rem,1fr))] gap-x-4 gap-y-1 font-mono text-xs tabular-nums">
        <div><dt class="text-muted">{strings.detail.spot}</dt><dd class="text-white">${price(market.spot)}</dd></div>
        <div><dt class="text-muted">{strings.detail.bidAsk}</dt><dd class="text-white">{price(market.bid)} × {price(market.ask)}</dd></div>
        <div><dt class="text-muted">{play.contract.market === 'option' ? strings.detail.iv : strings.detail.hv}</dt><dd class="text-white">{pct(market.iv * PERCENT)}</dd></div>
        {#if play.contract.market === 'option'}
          <div><dt class="text-muted">{strings.detail.oi}</dt><dd class="text-white">{compact(market.open_interest)}</dd></div>
        {/if}
        <div><dt class="text-muted">{strings.detail.volume}</dt><dd class="text-white">{compact(market.volume)}{market.avg_volume ? ` · ${(market.day_volume / market.avg_volume).toFixed(2)}x` : ''}</dd></div>
        <div><dt class="text-muted">{strings.detail.asOf}</dt><dd class="text-white">{age(market.at, now)} {strings.detail.ago}</dd></div>
      </dl>
      <p class="mt-1 text-[0.68rem] text-muted">{market.source}{market.note ? ` · ${market.note}` : ''}</p>
    {/if}
    </div>
    <SuccessRate odds={report.odds} />
  </header>

  <!-- WHAT MUST HAPPEN -->
  <section class="reveal border-b border-line px-4 py-4 lg:px-6" style="--i:1" aria-labelledby="must-happen">
    <h3 id="must-happen" class={SECTION_LABEL}>{strings.detail.mustHappen}</h3>
    <ul class="mt-2 grid gap-1.5">
      {#each lines as l, i (i)}
        <li class="grid grid-cols-[1rem_minmax(0,1fr)] text-sm leading-snug tabular-nums {LINE_CLASS[l.kind]}">
          <span aria-hidden="true" class="text-xs leading-5">{LINE_MARK[l.kind]}</span>
          <span class={l.kind === 'gain' ? 'text-white' : ''}>{l.text}</span>
        </li>
      {/each}
    </ul>
  </section>

  <!-- STAKE DIAL -->
  <section class="reveal border-b border-line px-4 py-4 lg:px-6" style="--i:2">
    <StakeDial {item} {client} {draft} bankroll={client.rules?.bankroll_usd ?? 0} sizing={report.sizing}
      onPreview={(p) => { preview = p; }} onSaved={() => { preview = null; }} />
  </section>

  <!-- SPINE + PAYOFF/ODDS -->
  <div class="reveal grid border-b border-line @2xl:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]" style="--i:3">
    <section class="border-line px-4 py-4 @2xl:border-r lg:px-6" aria-labelledby="spine">
      <h3 id="spine" class={SECTION_LABEL}>{strings.detail.spine}</h3>
      <p class="mt-0.5 text-[0.68rem] text-muted">{strings.detail.spineNote}</p>
      <LevelSpine {play} {report} />
    </section>
    <section class="border-t border-line px-4 py-4 @2xl:border-t-0 lg:px-6">
      <OddsPanel {report} />
    </section>
  </div>

  <!-- Very wide detail (ultra-wide, TV): rules beside evidence, bracket beside AI. -->
  <div class="grid border-b border-line @6xl:grid-cols-2">
  <section class="reveal border-line px-4 py-4 max-@6xl:border-b @6xl:border-r lg:px-6" style="--i:4" aria-labelledby="checks">
    <h3 id="checks" class={SECTION_LABEL}>{strings.detail.checks}</h3>
    <RuleChecks checks={report.checks} />
  </section>

  <section class="reveal px-4 py-4 lg:px-6" style="--i:5" aria-labelledby="evidence">
    <h3 id="evidence" class={SECTION_LABEL}>{strings.detail.evidence}</h3>
    <EvidenceList play={item.play} {draft} onSave={save} />
  </section>

  </div>

  <div class="grid @6xl:grid-cols-2">
  <section class="reveal border-line px-4 py-4 max-@6xl:border-b @6xl:border-r lg:px-6" style="--i:6" aria-labelledby="bracket">
    <h3 id="bracket" class={SECTION_LABEL}>{draft ? strings.detail.bracket : strings.detail.fills}</h3>
    {#if draft}
      {#key item.play.updated_at}<BracketEditor play={item.play} onSave={save} />{/key}
    {:else}
      <FillsPanel play={item.play} />
    {/if}
  </section>

  <section class="reveal px-4 py-4 lg:px-6" style="--i:7" aria-labelledby="ai">
    <h3 id="ai" class={SECTION_LABEL}>{strings.detail.ai}</h3>
    <AiPanel play={item.play} {draft} onSave={save} />
  </section>
  </div>
</div>

<footer class="border-t border-line px-4 py-2 text-[0.68rem] leading-snug text-muted short:py-1 short:text-[0.6rem] lg:px-6">
  {strings.detail.disclaimer}
  <a href="terms.html" class="spring text-white/70 underline underline-offset-2 hover:text-white focus-visible:text-white focus-visible:outline-none">{strings.detail.termsLink}</a>
</footer>
