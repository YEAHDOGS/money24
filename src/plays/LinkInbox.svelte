<script>
  /**
   * The link inbox: paste a Robinhood or Webull link, an option symbol or "AAL 12.5P 11/20". The
   * runner reads the contract, loads the market, builds the full bracket and (by default) paper
   * trades it at once. An AI read can run alongside. Every paste stays in the history below.
   */
  import strings from './strings.json';
  import { clock } from './play-format.js';
  import Link2 from '@lucide/svelte/icons/link-2';
  import Sparkles from '@lucide/svelte/icons/sparkles';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';

  /** @type {{ client: import('./plays-client.svelte.js').PlaysClient, onPick: (id: string) => void }} */
  let { client, onPick } = $props();

  const PLATFORM_LABEL = { robinhood: 'Robinhood', webull: 'Webull', text: 'Text' };
  const PLATFORM_CLASS = { robinhood: 'border-ok text-ok', webull: 'border-kind-news text-kind-news', text: 'border-line text-muted' };
  const URL_TAIL_CHARS = 42;

  let url = $state('');
  let theme = $state('');
  let aiRead = $state(true);
  let provider = $state('');
  let busy = $state(false);
  let message = $state('');
  let failed = $state(false);

  const connected = $derived(client.providers.filter((p) => p.connected));

  async function submit(/** @type {SubmitEvent} */ event) {
    event.preventDefault();
    const value = url.trim();
    if (!value || busy) return;
    busy = true;
    message = '';
    try {
      const item = await client.link({ url: value, theme: theme.trim(), analyze: aiRead && connected.length > 0, provider });
      failed = false;
      url = '';
      message = `${item.play.status === 'armed' ? strings.inbox.paperNow : strings.inbox.drafted}: ${item.play.title}`;
      onPick(item.play.id);
    } catch (err) {
      failed = true;
      message = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }

  /** "robinhood.com/options/6d33…_L1": the link without its scheme, cut from the middle. */
  function shortUrl(/** @type {string} */ u) {
    const bare = u.replace(/^https?:\/\/(www\.)?/, '');
    if (bare.length <= URL_TAIL_CHARS) return bare;
    return `${bare.slice(0, URL_TAIL_CHARS / 2)}…${bare.slice(-URL_TAIL_CHARS / 2 + 1)}`;
  }

  /** "AAL 12.5P 11/20" from a link's contract. */
  function contractLabel(/** @type {any} */ c) {
    if (!c?.symbol) return '';
    if (!c.right) return c.symbol;
    const [, mm, dd] = (c.expiry ?? '').split('-');
    return `${c.symbol} ${c.strike}${c.right} ${Number(mm)}/${Number(dd)}`;
  }
</script>

<form class="border-y border-line px-4 py-3" onsubmit={submit}>
  <label for="inbox-url" class="flex items-center gap-1.5 text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted">
    <Link2 class="size-3" aria-hidden="true" />{strings.inbox.label}
  </label>
  <textarea id="inbox-url" bind:value={url} rows="3" placeholder={strings.inbox.placeholder} autocomplete="off" spellcheck="false"
    onkeydown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); e.currentTarget.form?.requestSubmit(); } }}
    class="mt-1.5 block w-full resize-none rounded-sm border border-line bg-card px-2.5 py-2 font-mono text-xs leading-snug text-white placeholder:text-muted focus:border-neon-red focus:outline-none"></textarea>
  <div class="mt-2 flex gap-2">
    <input bind:value={theme} placeholder={strings.inbox.themePlaceholder} aria-label={strings.inbox.theme} autocomplete="off"
      class="min-w-0 flex-1 rounded-sm border border-line bg-card px-2.5 py-1.5 text-xs text-white placeholder:text-muted focus:border-neon-red focus:outline-none" />
    <button type="submit" disabled={busy || !url.trim()}
      class="spring shrink-0 rounded-sm bg-neon-red px-3 py-1.5 text-xs font-bold uppercase tracking-wider text-app hover:brightness-110 focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-white disabled:opacity-40">
      {busy ? strings.inbox.busy : strings.inbox.submit}
    </button>
  </div>
  {#if connected.length}
    <div class="mt-2 flex items-center gap-2 text-xs text-white/70">
      <label class="flex items-center gap-1.5">
        <input type="checkbox" bind:checked={aiRead} class="accent-neon-red" />
        <Sparkles class="size-3 text-kind-insider" aria-hidden="true" />{strings.inbox.aiRead}
      </label>
      <select bind:value={provider} aria-label={strings.inbox.provider} disabled={!aiRead}
        class="min-w-0 flex-1 rounded-sm border border-line bg-card px-1.5 py-1 text-xs text-white focus:border-neon-red focus:outline-none disabled:opacity-40">
        {#each connected as p (p.id)}<option value={p.id}>{p.name} · {p.model}</option>{/each}
      </select>
    </div>
  {:else}
    <p class="mt-2 text-[0.7rem] leading-snug text-muted">{strings.inbox.noProvider}</p>
  {/if}
  <p class="mt-1.5 min-h-4 text-xs leading-snug {failed ? 'text-bad' : 'text-ok'}" role="status">{message}</p>
</form>

<h2 class="px-4 pt-3 pb-1 text-[0.62rem] font-bold uppercase tracking-[0.18em] text-muted">{strings.inbox.history} · <span class="font-mono tabular-nums">{client.links.length}</span></h2>
<ol class="list-scroll min-h-0 flex-1 overflow-x-hidden overflow-y-auto pb-2 [scrollbar-gutter:stable]">
  {#each client.links as l (l.id)}
    <li>
      <button type="button" disabled={!l.play_id} onclick={() => onPick(l.play_id)} title={l.url}
        class="spring grid w-full grid-cols-[minmax(0,1fr)_auto] gap-x-2 px-4 py-1.5 text-left hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none disabled:cursor-default">
        <span class="col-span-2 flex min-w-0 items-center gap-1.5 text-xs">
          <span class="shrink-0 rounded-sm border px-1 text-[0.58rem] font-bold uppercase tracking-wider {PLATFORM_CLASS[l.platform] ?? PLATFORM_CLASS.text}">{PLATFORM_LABEL[l.platform] ?? l.platform}</span>
          <span class="truncate font-mono font-semibold text-white">{contractLabel(l.contract) || shortUrl(l.url)}</span>
        </span>
        {#if l.error}
          <span class="flex min-w-0 items-start gap-1 text-[0.68rem] leading-snug text-bad"><TriangleAlert class="mt-px size-3 shrink-0" aria-hidden="true" />{l.error}</span>
        {:else}
          <span class="truncate font-mono text-[0.65rem] text-white/45">{shortUrl(l.url)}</span>
        {/if}
        <time class="self-start font-mono text-[0.62rem] tabular-nums text-muted" datetime={l.at}>{clock(l.at)}</time>
      </button>
    </li>
  {:else}
    <li class="px-4 py-3 text-xs text-muted">{strings.inbox.historyEmpty}</li>
  {/each}
</ol>
