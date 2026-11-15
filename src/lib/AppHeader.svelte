<script>
  /**
   * The one header every Money 24 app page wears: the wordmark (home) and the page's section, the
   * page's own tools (a search box), its status, then the same nav to every app in the same order.
   * One slim row from tablet up; on phones the nav goes icon-only and the tools take a second row.
   * The page list lives in app-nav.json, mirrored by hand in terms.html (static, no JS).
   */
  import nav from './app-nav.json';
  import Zap from '@lucide/svelte/icons/zap';
  import LayoutDashboard from '@lucide/svelte/icons/layout-dashboard';
  import ChartCandlestick from '@lucide/svelte/icons/chart-candlestick';
  import Newspaper from '@lucide/svelte/icons/newspaper';
  import FileText from '@lucide/svelte/icons/file-text';

  /**
   * @type {{
   *   page: 'plays' | 'desk' | 'markets' | 'news' | 'terms',
   *   tools?: import('svelte').Snippet,
   *   status?: import('svelte').Snippet,
   * }}
   */
  let { page, tools, status } = $props();

  const HOME = './';
  const ICONS = { plays: Zap, desk: LayoutDashboard, markets: ChartCandlestick, news: Newspaper, terms: FileText };
  const current = $derived(nav.pages.find((p) => p.id === page));
</script>

<header class="border-b border-line">
  <div class="mx-auto flex max-w-7xl flex-wrap items-center gap-x-3 gap-y-2 px-3 py-2 sm:flex-nowrap sm:px-4 short:py-1 lg:px-6 ultrawide:max-w-none tv:py-4">
    <a href={HOME} aria-label={nav.home} class="spring group shrink-0 rounded-sm focus-visible:outline focus-visible:outline-offset-4 focus-visible:outline-neon-red">
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

    {#if status}
      <div class="flex shrink-0 items-center gap-2 max-sm:ml-auto">{@render status()}</div>
    {/if}

    <!-- Phones: its own full-width row of evenly spread icons, right under the wordmark on every page; page tools go below it. -->
    <nav aria-label={nav.label} class="shrink-0 max-sm:order-1 max-sm:-mx-1 max-sm:basis-full">
      <ul class="flex items-center gap-0.5 max-sm:justify-between">
        {#each nav.pages as p (p.id)}
          {@const Icon = ICONS[p.id]}
          {@const here = p.id === page}
          <li>
            <a href={p.href} aria-current={here ? 'page' : undefined} title={p.label}
              class="spring nav-link relative flex items-center gap-1.5 rounded-sm px-2 py-1.5 text-xs font-bold tracking-wider uppercase max-sm:flex-col max-sm:gap-0.5 max-sm:px-1.5 max-sm:py-1 max-sm:text-[0.6rem] max-sm:tracking-normal hover:bg-white/5 focus-visible:bg-white/5 focus-visible:outline-none short:py-1 tv:px-4 tv:py-3 tv:text-base {here ? 'text-white' : 'text-white/55 hover:text-white'}">
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

<style>
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
  @media (prefers-reduced-motion: reduce) {
    .nav-mark {
      transition: none;
    }
  }
</style>
