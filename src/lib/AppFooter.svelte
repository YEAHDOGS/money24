<script>
  /**
   * The one footer line every Money 24 app page ends on: who made it, that it is not advice, when
   * the demo on screen was recorded, a way to see the tour again, and the Terms of Use. Squat
   * windows drop it to keep the room; the notice at the top links the terms too.
   */
  import nav from './app-nav.json';
  import tourCopy from './tour/tour.json';
  import { tour } from './tour/tour.svelte.js';
  import { demoHref } from './demo.js';

  const CURRENT_YEAR = new Date().getFullYear();
  /** @type {Intl.DateTimeFormatOptions} */
  const RECORDED_DATE = { month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit' };

  /** @type {{ recorded?: string }} recorded: when the demo snapshot was taken (ISO time), empty off the demo */
  let { recorded = '' } = $props();
</script>

<footer aria-label={nav.footer.label} class="border-t border-line short:hidden">
  <div class="mx-auto flex h-6 max-w-7xl items-center gap-3 px-3 text-[0.62rem] font-bold tracking-[0.16em] whitespace-nowrap text-muted uppercase sm:px-4 lg:px-6 ultrawide:max-w-none tv:h-10 tv:text-sm">
    <span class="truncate">&copy; {CURRENT_YEAR} {nav.footer.copyright} · {nav.footer.disclaimer}</span>
    <!-- The demo note fades in once the snapshot has loaded; its room is always there, so nothing moves. -->
    <span class="min-w-0 flex-1 truncate text-center text-paper transition-opacity duration-300 max-sm:hidden {recorded ? 'opacity-100' : 'opacity-0'}">
      {recorded ? nav.footer.recorded.replace('{when}', new Date(recorded).toLocaleString(undefined, RECORDED_DATE)) : ''}
    </span>
    <button type="button" onclick={() => tour.restart()} class="spring shrink-0 rounded-sm tracking-[0.16em] uppercase max-sm:ml-auto hover:text-white focus-visible:text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red">{tourCopy.restart}</button>
    <a href={demoHref(nav.terms.href)} class="spring shrink-0 rounded-sm underline decoration-neon-red underline-offset-4 hover:text-white focus-visible:text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red">{nav.terms.label}</a>
  </div>
</footer>
