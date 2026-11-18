<script>
  /**
   * The first-run tutorial of the demo, on every app page: the cards first (once), then the
   * bubbles over this page until each has been put away. It waits while the work-in-progress
   * notice is open. The state, and what was seen before, is tour.svelte.js; the words are
   * tour.json. Renders nothing while the tour is off.
   */
  import { tour } from './tour.svelte.js';
  import { notice } from '../notice-state.svelte.js';
  import Carousel from './Carousel.svelte';
  import Blurbs from './Blurbs.svelte';

  /** @type {{ page: string }} */
  let { page } = $props();
</script>

{#if tour.on && !notice.open}
  {#if tour.carousel}
    <Carousel />
  {:else}
    <Blurbs {page} />
  {/if}
{/if}
