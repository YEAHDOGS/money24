<script>
  import strings from './schedule.json';
  import { scheduleTime } from './play-schedule.js';

  /** @type {{ play: any, compact?: boolean }} */
  let { play, compact = false } = $props();
</script>

<dl class="grid min-w-0 gap-1 text-xs {compact ? '' : 'mt-3 @md:grid-cols-2 @md:gap-x-4'}">
  <div class="min-w-0"><dt class="text-muted">{strings.entry}</dt><dd class="break-words font-mono text-white/80">{scheduleTime(play.expected_entry_at)}</dd></div>
  <div class="min-w-0"><dt class="text-muted">{strings.exit}</dt><dd class="break-words font-mono text-white/80">{scheduleTime(play.expected_exit_at)}</dd></div>
</dl>
{#if !compact && play.schedule_mode === 'planned'}
  <p class="mt-1.5 text-xs leading-snug text-muted">{strings.planned}</p>
{:else if !compact && play.schedule_mode === 'immediate'}
  <p class="mt-1.5 text-xs text-muted">{strings.immediate}</p>
{/if}
