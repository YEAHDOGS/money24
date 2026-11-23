<script>
  /**
   * One play, top to bottom: what must happen for it to pay, the stake dial (every number below
   * follows it), the price spine, the payoff and odds, the rules, the evidence, the bracket or the
   * fills, and the AI reads. The disclaimer stays pinned under it. On a phone (compact) the
   * dial comes first and the rest folds under its headings.
   */
  import { untrack } from 'svelte';
  import strings from './strings.json';
  import { mustHappen, price, compact as short, age, pct, pnl, fill, failing } from './play-format.js';
  import Fold from './Fold.svelte';
  import StakeDial from './StakeDial.svelte';
  import SuccessRate from './SuccessRate.svelte';
  import LevelSpine from './LevelSpine.svelte';
  import OddsPanel from './OddsPanel.svelte';
  import CrowdPanel from './CrowdPanel.svelte';
  import RuleChecks from './RuleChecks.svelte';
  import EvidenceList from './EvidenceList.svelte';
  import BracketEditor from './BracketEditor.svelte';
  import FillsPanel from './FillsPanel.svelte';
  import AiPanel from './AiPanel.svelte';
  import PlayRecord from './PlayRecord.svelte';
  import PlayOrders from './PlayOrders.svelte';
  import Play from '@lucide/svelte/icons/play';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import Sparkles from '@lucide/svelte/icons/sparkles';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import LoaderCircle from '@lucide/svelte/icons/loader-circle';

  /**
   * @type {{ item: import('./plays-client.svelte.js').Item, client: import('./plays-client.svelte.js').PlaysClient, now: number, compact?: boolean }}
   * compact: a phone. Shorter sentences, the dial first, and every other section folded under its heading.
   */
  let { item, client, now, compact = false } = $props();

  const LINE_CLASS = { gain: 'text-ok', loss: 'text-bad', neutral: 'text-white/80', warn: 'text-warn', bad: 'text-bad' };
  const LINE_MARK = { gain: '▲', loss: '▼', neutral: '•', warn: '!', bad: '×' };
  const STATUS_CLASS = { draft: 'border-line text-white/70', armed: 'border-paper text-paper', closed: 'border-muted text-muted' };
  const SECTION_LABEL = 'text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted';
  const PERCENT = 100;
  const ALL_PROVIDERS = 'all';

  /** @type {import('./plays-client.svelte.js').Item | null} the stake dial's unsaved view */
  let preview = $state(null);
  let busy = $state('');
  let error = $state('');
  let provider = $state(untrack(() => client.aiChoices[0]?.id ?? ''));

  const shown = $derived(preview ?? item);
  const play = $derived(shown.play);
  const report = $derived(shown.report);
  const draft = $derived(item.play.status === 'draft');
  const lines = $derived(mustHappen(play, report, compact));
  const fails = $derived(failing(report));
  /** A few words beside a folded heading, so the fold says what is inside. */
  const endsNote = $derived(`${pnl(report.payoff.max_profit_usd)} / ${pnl(-report.payoff.planned_loss_usd)}`);
  const checksNote = $derived(fails.blocks || fails.warns ? fill(strings.detail.checksNote, { blocks: fails.blocks, warns: fails.warns }) : strings.detail.checksClear);
  const market = $derived(play.market);
  /** Who the play can be sent to; the picked one, or the first while the list is still loading. */
  const choices = $derived(client.aiChoices);
  const chosen = $derived(choices.find((p) => p.id === provider) ?? choices[0]);
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

<!-- The sections both layouts share: wide screens lay them out in the open, phones fold them (Fold.svelte). -->
{#snippet dial()}
  <section class="reveal border-b border-line px-4 py-4 lg:px-6" style="--i:2">
    <StakeDial {item} {client} {draft} bankroll={client.rules?.bankroll_usd ?? 0} sizing={report.sizing}
      onPreview={(p) => { preview = p; }} onSaved={() => { preview = null; }} />
  </section>
{/snippet}
{#snippet ordersBody()}{#key item.play.id}<PlayOrders {item} {client} {now} />{/key}{/snippet}
{#snippet recordBody()}{#key item.play.id}<PlayRecord {item} {client} />{/key}{/snippet}
{#snippet spineBody()}
  <p class="mt-0.5 text-[0.68rem] text-muted">{strings.detail.spineNote}</p>
  <LevelSpine {play} {report} />
{/snippet}
{#snippet bracketBody()}
  {#if draft}
    {#key item.play.updated_at}<BracketEditor play={item.play} onSave={save} />{/key}
  {:else}
    <FillsPanel play={item.play} />
  {/if}
{/snippet}

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
      {#if chosen}
        <span class="flex items-stretch">
          <button type="button" onclick={() => act('analyze', () => client.analyze(item.play.id, chosen.id))} disabled={!!busy || !!analyzing}
            class="spring flex items-center gap-1.5 rounded-l-sm border border-kind-insider/60 px-3 py-2 text-xs font-bold uppercase tracking-wider text-kind-insider hover:bg-kind-insider/10 focus-visible:outline-none focus-visible:bg-kind-insider/10 disabled:opacity-40">
            {#if analyzing}<LoaderCircle class="spin size-3.5" aria-hidden="true" />{:else}<Sparkles class="size-3.5" aria-hidden="true" />{/if}
            {analyzing ? strings.detail.analyzing : chosen.id === ALL_PROVIDERS ? strings.detail.analyzeAll : fill(strings.detail.analyze, { name: chosen.name })}
          </button>
          <select value={chosen.id} onchange={(e) => (provider = e.currentTarget.value)} aria-label={strings.inbox.provider}
            class="max-w-32 rounded-r-sm border border-l-0 border-kind-insider/60 bg-card px-1 text-xs text-white focus:border-neon-red focus:outline-none">
            {#each choices as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
          </select>
        </span>
      {:else if !client.demo}
        <span class="self-center text-[0.68rem] leading-snug text-muted">{strings.detail.noAi}</span>
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
          <div><dt class="text-muted">{strings.detail.oi}</dt><dd class="text-white">{short(market.open_interest)}</dd></div>
        {/if}
        <div><dt class="text-muted">{strings.detail.volume}</dt><dd class="text-white">{short(market.volume)}{market.avg_volume ? ` · ${(market.day_volume / market.avg_volume).toFixed(2)}x` : ''}</dd></div>
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

  {#if compact}
    <!-- PHONES: the dial first, then every other section folded under its heading. -->
    {@render dial()}
    <Fold id="orders" title={strings.detail.orders} open={!draft} i={3}>{@render ordersBody()}</Fold>
    <Fold id="ends" title={strings.detail.scenarios} note={endsNote} i={4}><OddsPanel {report} bare /></Fold>
    <Fold id="crowd" title={strings.crowd.title} i={5}><CrowdPanel {item} bare /></Fold>
    <Fold id="spine" title={strings.detail.spine} i={5}>{@render spineBody()}</Fold>
    <Fold id="checks" title={strings.detail.checks} note={checksNote} tone={fails.blocks ? 'text-bad' : fails.warns ? 'text-warn' : 'text-ok'} i={6}><RuleChecks checks={report.checks} /></Fold>
    <Fold id="evidence" title={strings.detail.evidence} note={String(item.play.evidence?.length ?? 0)} i={7}><EvidenceList play={item.play} {draft} onSave={save} /></Fold>
    <Fold id="bracket" title={draft ? strings.detail.bracket : strings.detail.fills} i={8}>{@render bracketBody()}</Fold>
    <Fold id="record" title={strings.detail.record} i={9}>{@render recordBody()}</Fold>
    <Fold id="ai" title={strings.detail.ai} note={analyzing ? strings.detail.analyzing : String(item.play.analyses?.length ?? 0)} tone={analyzing ? 'text-kind-insider' : 'text-muted'} i={10}><AiPanel play={item.play} {draft} onSave={save} /></Fold>
  {:else}
  <section class="reveal border-b border-line px-4 py-4 lg:px-6" style="--i:1" aria-labelledby="orders">
    <h3 id="orders" class="{SECTION_LABEL} mb-2">{strings.detail.orders}</h3>
    {@render ordersBody()}
  </section>

  <section class="reveal border-b border-line px-4 py-4 lg:px-6" style="--i:2" aria-labelledby="record">
    <h3 id="record" class="{SECTION_LABEL} mb-2">{strings.detail.record}</h3>
    {@render recordBody()}
  </section>

  {@render dial()}

  <!-- SPINE + PAYOFF/ODDS -->
  <div class="reveal grid border-b border-line @2xl:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]" style="--i:3">
    <section class="border-line px-4 py-4 @2xl:border-r lg:px-6" aria-labelledby="spine">
      <h3 id="spine" class={SECTION_LABEL}>{strings.detail.spine}</h3>
      {@render spineBody()}
    </section>
    <section class="border-t border-line px-4 py-4 @2xl:border-t-0 lg:px-6">
      <OddsPanel {report} />
      <div class="mt-4 border-t border-line pt-3"><CrowdPanel {item} /></div>
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
    {@render bracketBody()}
  </section>

  <section class="reveal px-4 py-4 lg:px-6" style="--i:7" aria-labelledby="ai">
    <h3 id="ai" class={SECTION_LABEL}>{strings.detail.ai}</h3>
    <AiPanel play={item.play} {draft} onSave={save} />
  </section>
  </div>
  {/if}
</div>

<footer class="border-t border-line px-4 py-2 text-[0.68rem] leading-snug text-muted short:py-1 short:text-[0.6rem] lg:px-6">
  {compact ? strings.detail.disclaimerShort : strings.detail.disclaimer}
  <a href="terms.html" class="spring text-white/70 underline underline-offset-2 hover:text-white focus-visible:text-white focus-visible:outline-none">{strings.detail.termsLink}</a>
</footer>
