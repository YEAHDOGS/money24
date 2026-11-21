<script>
  /**
   * Money 24 Home: the screen the app opens on, and the one to watch the desk from. First the plays
   * running on the desk now, what the desk has been doing and the account; then the news: a banner
   * of top stories and headlines by category. A ribbon of quotes runs under the header, and under
   * it today on the trading calendar. Everything is read-only, and every dollar on it is the desk's own.
   * Wide: positions, the desk and the account on the left, news in a side column, split by 1px lines.
   * Phones: one column that scrolls as a list. Phones on their side: the two halves side by side,
   * each its own list. Portrait monitors: news in a row underneath, headlines two abreast. Ultra-wide: three columns.
   */
  import { onMount } from 'svelte';
  import strings from './strings.json';
  import { HomeClient } from './home-client.svelte.js';
  import Ribbon from './Ribbon.svelte';
  import DayStrip from './DayStrip.svelte';
  import Desk from './Desk.svelte';
  import Positions from './Positions.svelte';
  import WalletChart from './WalletChart.svelte';
  import NewsCarousel from './NewsCarousel.svelte';
  import Headlines from './Headlines.svelte';
  import AppHeader from '../lib/AppHeader.svelte';
  import AppFooter from '../lib/AppFooter.svelte';

  const CLOCK_TICK_MS = 30000; // ages and the chart's right edge move on
  const IS_TAURI = typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;
  // In the desktop app, links navigate and the shell hands them to the system browser.
  const LINK_TARGET = IS_TAURI ? undefined : '_blank';

  const client = new HomeClient();
  let now = $state(Date.now());

  onMount(() => {
    client.start();
    const tick = setInterval(() => { now = Date.now(); }, CLOCK_TICK_MS);
    return () => {
      clearInterval(tick);
      client.stop();
    };
  });
</script>

<svelte:head><title>{strings.title}</title></svelte:head>

<div class="flex h-dvh w-full flex-col">
  <AppHeader page="home" status={client.status} />
  <Ribbon quotes={client.ribbon} />
  <DayStrip days={client.days} {now} />

  <!-- Phones: the whole page is one list that scrolls. From tablet up each column keeps to the screen and only its lists scroll. -->
  <div class="list-scroll mx-auto min-h-0 w-full max-w-7xl flex-1 border-line select-text max-md:overflow-x-hidden max-md:overflow-y-auto md:grid md:grid-cols-[minmax(0,1fr)_minmax(0,22rem)] lg:grid-cols-[minmax(0,1fr)_minmax(0,26rem)] xl:border-x desktop-portrait:grid-cols-1! desktop-portrait:grid-rows-[auto_minmax(0,1fr)] landscape-phone:grid landscape-phone:grid-cols-2! landscape-phone:overflow-hidden! ultrawide:max-w-none ultrawide:grid-cols-[minmax(0,1fr)_minmax(0,32rem)]">
    <main class="list-scroll flex min-h-0 flex-col border-line md:overflow-x-hidden md:overflow-y-auto md:border-r desktop-portrait:border-r-0 landscape-phone:overflow-x-hidden landscape-phone:overflow-y-auto landscape-phone:border-r ultrawide:grid ultrawide:grid-cols-3 ultrawide:grid-rows-[minmax(0,1fr)] ultrawide:divide-x ultrawide:divide-line ultrawide:overflow-y-hidden">
      <Positions rows={client.rows} status={client.status} />
      <Desk journal={client.journal} flow={client.flow} crypto={client.crypto} finder={client.finder} status={client.status} {now} />
      <WalletChart journal={client.journal} capital={client.capital} ready={client.walletReady} status={client.status} account={client.account} {now} />
    </main>

    <aside aria-label={strings.news.title} class="list-scroll flex min-h-0 flex-col border-line desktop-portrait:border-t landscape-phone:block landscape-phone:overflow-x-hidden landscape-phone:overflow-y-auto">
      <NewsCarousel slides={client.slides} mode={client.news.mode} {now} linkTarget={LINK_TARGET} />
      <Headlines {client} {now} linkTarget={LINK_TARGET} />
    </aside>
  </div>

  <AppFooter recorded={client.demoTakenAt} />
</div>
