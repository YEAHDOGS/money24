<script>
  /**
   * The account button at the end of the header and its panel. For now the account holds one thing:
   * how the site looks (Auto follows the sun, or Light, or Dark). It sits in the header's row 1 on phones
   * and at the end of the row from tablet up, and its panel floats, so opening it moves nothing.
   * The choice and the sunset math live in ./theme.js.
   */
  import strings from './account.json';
  import { MODES, THEME_EVENT, getMode, setMode, resolve, knowsLocation, shareLocation } from './theme.js';
  import HeaderPopover from './HeaderPopover.svelte';
  import CircleUser from '@lucide/svelte/icons/circle-user';
  import Sun from '@lucide/svelte/icons/sun';
  import Moon from '@lucide/svelte/icons/moon';
  import SunMoon from '@lucide/svelte/icons/sun-moon';
  import MapPin from '@lucide/svelte/icons/map-pin';

  const ICONS = { auto: SunMoon, light: Sun, dark: Moon };
  const TIME_FORMAT = { hour: 'numeric', minute: '2-digit' };

  let mode = $state(getMode());
  let sun = $state(resolve());
  let located = $state(knowsLocation());
  let denied = $state(false);

  const untilText = $derived(new Date(sun.until).toLocaleTimeString([], TIME_FORMAT));
  const nowText = $derived(strings.now.replace('{theme}', strings.themes[sun.theme]).replace('{time}', untilText));

  function choose(/** @type {string} */ next) {
    setMode(next);
    mode = getMode();
    sun = resolve();
  }

  async function locate() {
    denied = !(await shareLocation());
    located = knowsLocation();
    sun = resolve();
  }

  $effect(() => {
    const refresh = () => {
      mode = getMode();
      sun = resolve();
    };
    window.addEventListener(THEME_EVENT, refresh);
    return () => window.removeEventListener(THEME_EVENT, refresh);
  });
</script>

<HeaderPopover icon={CircleUser} label={strings.button} tour="account">
  <div class="p-3 tv:p-5">
    <h2 class="text-[0.62rem] font-bold tracking-[0.2em] text-muted uppercase tv:text-sm">{strings.appearance}</h2>
    <div role="radiogroup" aria-label={strings.appearance} class="mt-2 grid grid-cols-3 gap-1">
      {#each MODES as m (m)}
        {@const Icon = ICONS[m]}
        <button type="button" role="radio" aria-checked={mode === m} title={strings.modes[m].hint} onclick={() => choose(m)}
          class="spring flex flex-col items-center gap-1 rounded-sm border px-2 py-2 text-xs font-bold tracking-wide uppercase focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red tv:py-3 tv:text-base {mode === m ? 'border-neon-red bg-neon-red/10 text-white' : 'border-line text-white/70 hover:bg-white/5 hover:text-white'}">
          <Icon class="size-4 tv:size-6 {mode === m ? 'text-neon-red' : ''}" aria-hidden="true" />
          {strings.modes[m].label}
        </button>
      {/each}
    </div>

    {#if mode === 'auto'}
      <p class="mt-2.5 text-xs leading-snug text-white/80 tv:text-base" aria-live="polite">{nowText}</p>
      {#if located}
        <p class="mt-1 text-xs leading-snug text-muted tv:text-sm">{strings.locationOn}</p>
      {:else}
        <p class="mt-1 text-xs leading-snug text-muted tv:text-sm">{denied ? strings.locationDenied : strings.guess}</p>
        <button type="button" onclick={locate}
          class="spring mt-2 flex items-center gap-1.5 rounded-sm px-1 py-1 text-xs font-semibold text-white/80 underline decoration-neon-red/70 underline-offset-4 hover:text-white focus-visible:outline focus-visible:outline-offset-2 focus-visible:outline-neon-red tv:text-base">
          <MapPin class="size-3.5 shrink-0" aria-hidden="true" />{strings.useLocation}
        </button>
      {/if}
    {/if}
  </div>
</HeaderPopover>
