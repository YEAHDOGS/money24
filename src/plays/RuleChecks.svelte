<script>
  /**
   * Every rule's verdict on the play: blocks first, then warnings, then passes; rules that are off
   * fold away at the bottom. Status travels by word and icon, not color alone.
   */
  import strings from './strings.json';
  import CircleX from '@lucide/svelte/icons/circle-x';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import CircleCheck from '@lucide/svelte/icons/circle-check';
  import Info from '@lucide/svelte/icons/info';
  import CircleMinus from '@lucide/svelte/icons/circle-minus';

  /** @type {{ checks: any[] }} */
  let { checks } = $props();

  const ORDER = { fail: 0, warn: 1, info: 2, pass: 3, off: 4 };
  const ICON = { fail: CircleX, warn: TriangleAlert, pass: CircleCheck, info: Info, off: CircleMinus };
  const TONE = { fail: 'text-bad', warn: 'text-warn', pass: 'text-ok', info: 'text-kind-news', off: 'text-muted' };

  const sorted = $derived([...checks].sort((a, b) => ORDER[a.status] - ORDER[b.status]));
  const on = $derived(sorted.filter((c) => c.status !== 'off'));
  const off = $derived(sorted.filter((c) => c.status === 'off'));
</script>

<ul class="mt-2 grid gap-1.5">
  {#each on as c (c.id)}
    {@const Icon = ICON[c.status]}
    <li class="grid grid-cols-[1rem_minmax(0,1fr)] gap-x-2 text-xs leading-snug">
      <Icon class="mt-px size-3.5 {TONE[c.status]}" aria-hidden="true" />
      <span>
        <span class="font-semibold text-white">{strings.rules[c.id]?.label ?? c.id}</span>
        <span class="ml-1 text-[0.6rem] font-bold uppercase tracking-wider {TONE[c.status]}">{strings.checkStatus[c.status]}</span>
        {#if c.detail}<span class="block text-white/60 tabular-nums">{c.detail}</span>{/if}
      </span>
    </li>
  {/each}
</ul>
{#if off.length}
  <p class="mt-2 text-[0.68rem] text-muted">{strings.checkStatus.off}: {off.map((c) => strings.rules[c.id]?.label ?? c.id).join(', ')}</p>
{/if}
