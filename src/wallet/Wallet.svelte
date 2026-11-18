<script>
  /**
   * Money 24 Wallet: where money for plays will come in (crypto or a debit card) and be held for
   * orders. Deposits are not open: this page shows the place and says plainly what has to happen
   * first. Nothing on it moves money. Wide screens: balance and ways in on the left, what is
   * missing on the right, split by a 1px line. Phones: one column that scrolls as a list.
   */
  import AppHeader from '../lib/AppHeader.svelte';
  import AppFooter from '../lib/AppFooter.svelte';
  import strings from './strings.json';
  import nav from '../lib/app-nav.json';
  import { demoFirst, demoHref } from '../lib/demo.js';
  import Bitcoin from '@lucide/svelte/icons/bitcoin';
  import CreditCard from '@lucide/svelte/icons/credit-card';
  import Lock from '@lucide/svelte/icons/lock';

  const ICONS = { crypto: Bitcoin, card: CreditCard };
  const POWER_BARS = [1, 2, 3];
  const EMPTY_BALANCE = '$0.00';
  const LABEL = 'text-[0.62rem] font-bold tracking-[0.18em] text-muted uppercase';
  const status = demoFirst() ? 'demo' : '';
</script>

<svelte:head><title>{strings.title}</title></svelte:head>

<div class="flex h-dvh w-full flex-col">
  <AppHeader page="wallet" {status} />

  <div class="mx-auto grid min-h-0 w-full max-w-7xl flex-1 grid-cols-[minmax(0,1fr)] border-line select-text md:grid-cols-[minmax(0,1fr)_minmax(0,22rem)] xl:border-x ultrawide:max-w-none">
    <main class="list-scroll min-h-0 overflow-y-auto border-line md:border-r">
      <section class="cal-rise border-b border-line px-4 py-5 short:py-3 lg:px-6" style="--i:0">
        <h2 class={LABEL}>{strings.balance}</h2>
        <p class="mt-1 font-mono text-5xl leading-none font-semibold text-white tabular-nums short:text-3xl">{EMPTY_BALANCE}</p>
        <p class="mt-2 text-sm text-muted">{strings.balanceNote}</p>
      </section>

      <section class="cal-rise border-b border-line px-4 py-4 lg:px-6" style="--i:1">
        <h2 class={LABEL}>{strings.add}</h2>
        <ul class="mt-3 grid gap-px bg-line sm:grid-cols-2">
          {#each strings.methods as method (method.id)}
            {@const Icon = ICONS[method.id]}
            <li class="flex flex-col gap-2 bg-app p-4">
              <span class="flex items-center gap-2 text-lg font-extrabold text-white"><Icon class="size-5 text-neon-red" aria-hidden="true" />{method.name}</span>
              <span class="text-sm leading-snug text-white/70">{method.text}</span>
              <button type="button" disabled class="mt-auto flex items-center justify-center gap-2 rounded-sm border border-line px-3 py-2 text-xs font-bold tracking-wider text-muted uppercase">
                <Lock class="size-3.5" aria-hidden="true" />{strings.notOpen}
              </button>
            </li>
          {/each}
        </ul>
      </section>

      <section class="cal-rise px-4 py-4 lg:px-6" style="--i:2">
        <h2 class={LABEL}>{strings.power}</h2>
        <ul class="mt-3 grid grid-cols-3 gap-px bg-line">
          {#each strings.modes as mode (mode.id)}
            <li class="flex items-end gap-2 bg-app px-3 py-3 text-muted">
              <span class="flex h-5 items-end gap-0.5" aria-hidden="true">
                {#each POWER_BARS as bar (bar)}
                  <span class="w-1 {bar <= mode.level ? 'bg-white/60' : 'bg-line'}" style:height="{(bar / POWER_BARS.length) * 100}%"></span>
                {/each}
              </span>
              <span class="text-sm leading-none font-extrabold tracking-wide uppercase">{mode.name}</span>
            </li>
          {/each}
        </ul>
        <p class="mt-2 text-sm text-muted">{strings.powerNote}</p>
      </section>
    </main>

    <aside class="list-scroll min-h-0 overflow-y-auto max-md:border-t max-md:border-line">
      <section class="cal-rise border-b border-warn/40 px-4 py-4" style="--i:1" role="note">
        <h2 class="flex items-center gap-2 text-sm font-extrabold tracking-wide text-warn uppercase"><Lock class="size-4" aria-hidden="true" />{strings.closed}</h2>
        <p class="mt-2 text-sm leading-snug text-white/80">{strings.closedText}</p>
      </section>
      <section class="cal-rise px-4 py-4" style="--i:2">
        <h2 class={LABEL}>{strings.before}</h2>
        <ol class="mt-2 grid gap-2">
          {#each strings.steps as step, i (step)}
            <li class="grid grid-cols-[1.5rem_minmax(0,1fr)] text-sm leading-snug text-white/80">
              <span class="font-mono text-xs font-bold text-neon-red tabular-nums">{i + 1}</span>{step}
            </li>
          {/each}
        </ol>
        <a href={demoHref(nav.terms.href)} class="spring mt-4 inline-block text-sm font-semibold text-white underline decoration-neon-red underline-offset-4 hover:text-neon-red">{strings.terms}</a>
      </section>
    </aside>
  </div>

  <AppFooter />
</div>
