<script>
  /**
   * The one footer line every Money 24 app page ends on: who made it (the DOGS logo and sticker),
   * that it is not advice, when
   * the demo on screen was recorded, a way to see the tour again, and the Terms of Use. Squat
   * windows drop it to keep the room; the notice at the top links the terms too.
   */
  import nav from './app-nav.json';
  import tourCopy from './tour/tour.json';
  import { tour } from './tour/tour.svelte.js';
  import { demoHref } from './demo.js';
  // The DOGS logo and sticker are the files from logo.dogs.red.
  import logo from '../assets/dogs-logo.svg';
  import sticker from '../assets/made-by-dogs-sticker.webp';

  const CURRENT_YEAR = new Date().getFullYear();
  /** @type {Intl.DateTimeFormatOptions} */
  const RECORDED_DATE = { month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit' };

  /** @type {{ recorded?: string }} recorded: when the demo snapshot was taken (ISO time), empty off the demo */
  let { recorded = '' } = $props();
</script>

<footer aria-label={nav.footer.label} class="border-t border-line short:hidden">
  <div class="flex h-8 w-full items-center gap-3 px-3 text-[0.62rem] font-bold tracking-[0.16em] whitespace-nowrap text-muted uppercase sm:px-4 tv:h-10 tv:text-sm">
    <a href={nav.footer.dogsUrl} target="_blank" rel="noopener noreferrer" class="spring flex shrink-0 items-center gap-2 rounded-sm opacity-80 hover:opacity-100 focus-visible:opacity-100 focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red">
      <img src={logo} alt={nav.footer.dogsLogo} width="540" height="534" loading="lazy" class="h-5 w-auto tv:h-7" />
      <img src={sticker} alt={nav.footer.dogsSticker} width="360" height="119" loading="lazy" class="h-6 w-auto max-[479px]:hidden tv:h-8" />
    </a>
    <span class="min-w-0 flex-1 truncate">&copy; {CURRENT_YEAR} {nav.footer.copyright} · {nav.footer.disclaimer}</span>
    <!-- The demo note fades in once the snapshot has loaded. It sits at the right with Tour and Terms. -->
    <span class="min-w-0 truncate text-right text-paper transition-opacity duration-300 max-md:hidden {recorded ? 'opacity-100' : 'opacity-0'}">
      {recorded ? nav.footer.recorded.replace('{when}', new Date(recorded).toLocaleString(undefined, RECORDED_DATE)) : ''}
    </span>
    <button type="button" onclick={() => tour.restart()} class="spring shrink-0 rounded-sm tracking-[0.16em] uppercase hover:text-white focus-visible:text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red">{tourCopy.restart}</button>
    <a href={demoHref(nav.terms.href)} class="spring shrink-0 rounded-sm underline decoration-neon-red underline-offset-4 hover:text-white focus-visible:text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red">{nav.terms.label}</a>
  </div>
</footer>
