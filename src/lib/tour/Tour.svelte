<script>
  /**
   * The tour of one app page: a stepper of at most four pop-ups (Steps.svelte). The demo starts it
   * on a first visit to each page; it waits while the work-in-progress notice is open. The state,
   * and which pages were toured, is tour.svelte.js; the words are tour.json. Renders nothing while
   * the tour is off.
   */
  import { untrack } from 'svelte';
  import { tour } from './tour.svelte.js';
  import { notice } from '../notice-state.svelte.js';
  import Steps from './Steps.svelte';

  /** @type {{ page: string }} */
  let { page } = $props();

  $effect(() => {
    const here = page;
    untrack(() => tour.arrive(here));
  });
</script>

{#if tour.on && !notice.open}
  <Steps />
{/if}
