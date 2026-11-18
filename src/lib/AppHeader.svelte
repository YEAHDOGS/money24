<script>
  /**
   * The one header every Money 24 page wears, the Terms of Use included: the work-in-progress
   * notice, then the wordmark (home) and the page's section, the page's own tools (a search box),
   * the connection status, and the same nav to every app in the same order.
   * Nothing in it changes size from page to page: the row has one height, the tabs are equally wide
   * and the status keeps the width of its longest label. So moving between pages moves nothing.
   * One slim row from tablet up; on phones the nav goes full width under the wordmark and the
   * tools take a third row. The page list lives in app-nav.json.
   */
  import nav from './app-nav.json';
  import NoticeBar from './NoticeBar.svelte';
  import Tour from './tour/Tour.svelte';
  import { demoHref } from './demo.js';
  import Zap from '@lucide/svelte/icons/zap';
  import LayoutDashboard from '@lucide/svelte/icons/layout-dashboard';
  import ChartCandlestick from '@lucide/svelte/icons/chart-candlestick';
  import Newspaper from '@lucide/svelte/icons/newspaper';

  /**
   * @type {{
   *   page: 'plays' | 'desk' | 'markets' | 'news' | 'terms',
   *   tools?: import('svelte').Snippet,
   *   status?: string,
   *   service?: 'runner' | 'news',
   * }} status: the page's connection (a key of app-nav.json "status"), empty when it has none.
   *   service: which engine that connection is to; each one's last state is remembered apart.
   */
  let { page, tools, status = '', service = 'runner' } = $props();

  const HOME = './';
  const ICONS = { plays: Zap, desk: LayoutDashboard, markets: ChartCandlestick, news: Newspaper };
  const STORE_STATUS = 'm24.status.';
  /** States worth carrying to the next page. */
  const SETTLED = ['live', 'offline', 'demo'];
  /** States that say nothing yet: the header keeps showing what the last page knew. */
  const WAITING = ['', 'connecting'];
  const STATUS_COLOR = { live: 'text-ok', connecting: 'text-warn', reconnecting: 'text-warn', offline: 'text-bad', demo: 'text-paper' };

  /** What the last page in this tab knew about a service. Session storage can be missing or throw. */
  function recall(/** @type {string} */ key) {
    try {
      return sessionStorage.getItem(key) ?? '';
    } catch {
      return '';
    }
  }

  const current = $derived(page === nav.terms.id ? nav.terms : nav.pages.find((p) => p.id === page));
  const remembered = $derived(recall(STORE_STATUS + service));
  /** A fresh page is "connecting" for a moment: show the remembered state instead, so the dot never blinks between pages. */
  const shown = $derived(WAITING.includes(status) ? remembered || status : status);

  $effect(() => {
    if (!SETTLED.includes(status)) return;
    try {
      sessionStorage.setItem(STORE_STATUS + service, status);
    } catch {
      // Not remembered on this device: the next page starts from "connecting".
    }
  });
</script>

<div class="app-header bg-app">
  <NoticeBar />
  <header class="border-b border-line">
    <div class="mx-auto flex max-w-7xl flex-wrap items-center gap-x-3 gap-y-1 px-3 py-1.5 sm:h-12 sm:flex-nowrap sm:px-4 sm:py-0 lg:px-6 short:sm:h-10 ultrawide:max-w-none tv:h-20">
      <a href={demoHref(HOME)} aria-label={nav.home} class="spring group flex h-8 shrink-0 items-center rounded-sm focus-visible:outline focus-visible:outline-offset-4 focus-visible:outline-neon-red short:h-6">
        <span class="text-xl leading-none font-extrabold tracking-tight whitespace-nowrap short:text-base md:text-2xl tv:text-4xl">
          {nav.brand}<span class="text-neon-red">{nav.brandAccent}</span><span
            class="ml-1.5 align-middle text-[0.6rem] font-bold tracking-[0.3em] text-muted transition-colors duration-150 group-hover:text-white/80 max-[399px]:hidden tv:text-sm">{current?.section}</span>
        </span>
      </a>

      {#if tools}
        <div class="order-2 flex min-w-0 basis-full items-center sm:order-none sm:flex-1 sm:basis-auto">{@render tools()}</div>
      {:else}
        <span class="flex-1" aria-hidden="true"></span>
      {/if}

      <!-- STATUS: on every page, in the same place, as wide as its longest label. -->
      <div data-tour="status" class="flex h-8 shrink-0 items-center gap-2 max-sm:ml-auto short:h-6">
        <p class="flex items-center gap-2 text-xs font-semibold {STATUS_COLOR[shown] ?? 'invisible'}" role="status">
          <span class="status-dot relative size-2 rounded-full bg-current {shown === 'live' ? 'live-dot' : ''}"></span>
          <span class="grid max-[419px]:sr-only">
            <span class="invisible col-start-1 row-start-1" aria-hidden="true">{nav.statusWidest}</span>
            <span class="col-start-1 row-start-1">{nav.status[shown] ?? ''}</span>
          </span>
        </p>
        <span class="rounded-sm border border-warn px-1.5 py-px text-[0.62rem] font-bold tracking-wider whitespace-nowrap text-warn uppercase">{nav.paper}</span>
      </div>

      <!-- NAV: equal columns, so every tab is as wide as the widest. Phones: a full-width row under the wordmark; page tools go below it. -->
      <nav aria-label={nav.label} class="shrink-0 max-sm:order-1 max-sm:-mx-1 max-sm:basis-full">
        <ul class="grid auto-cols-fr grid-flow-col items-center gap-0.5">
          {#each nav.pages as p (p.id)}
            {@const Icon = ICONS[p.id]}
            {@const here = p.id === page}
            <li>
              <a href={demoHref(p.href)} aria-current={here ? 'page' : undefined} title={p.label}
                class="spring nav-link relative flex items-center justify-center gap-1.5 rounded-sm px-2 py-1.5 text-xs font-bold tracking-wider uppercase max-sm:flex-col max-sm:gap-0.5 max-sm:px-1.5 max-sm:py-1 max-sm:text-[0.6rem] max-sm:tracking-normal hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none short:py-1 tv:px-4 tv:py-3 tv:text-base {here ? 'text-white' : 'text-white/55 hover:text-white'}">
                <Icon class="size-4 shrink-0 transition-transform duration-150 tv:size-6 {here ? 'scale-110 text-neon-red' : ''}" aria-hidden="true" />
                <span class="sm:max-lg:sr-only">{p.label}</span>
                <span class="nav-mark pointer-events-none absolute inset-x-2 -bottom-px h-0.5 bg-neon-red {here ? 'is-here' : ''}" aria-hidden="true"></span>
              </a>
            </li>
          {/each}
        </ul>
      </nav>
    </div>
  </header>
</div>

<!-- The demo's first-run tour. The Terms of Use are read, not toured. -->
{#if page !== nav.terms.id}
  <Tour {page} />
{/if}

<style>
  /* Between pages the browser carries the header across as one piece (view transitions, see
     src/styles/shell.css): the header holds still while the red mark slides to the new tab. */
  .app-header {
    view-transition-name: m24-header;
  }
  .nav-mark.is-here {
    view-transition-name: m24-nav-mark;
  }

  /* The red mark under the current page draws in from the middle; a hovered link previews it. */
  .nav-mark {
    transform: scaleX(0);
    transition: transform 150ms cubic-bezier(0.34, 1.56, 0.64, 1);
  }
  .nav-mark.is-here {
    transform: scaleX(1);
  }
  .nav-link:hover .nav-mark:not(.is-here),
  .nav-link:focus-visible .nav-mark:not(.is-here) {
    transform: scaleX(0.35);
  }
  /* The status changes color, never size. */
  .status-dot {
    transition: color 200ms ease;
  }
  @media (prefers-reduced-motion: reduce) {
    .nav-mark,
    .status-dot {
      transition: none;
    }
  }
</style>
