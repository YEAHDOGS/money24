<script>
  import { onMount } from "svelte";
  import sticker from "./assets/made-by-dogs-sticker.webp";

  const DOGS_URL = "https://dogs.red";
  const CURRENT_YEAR = new Date().getFullYear();

  const MARKETS = ["CRYPTO", "OPTIONS", "STOCKS", "FUTURES", "PREDICTION MARKETS"];
  const STEPS = ["SCAN", "SIZE", "RISK GATE", "EXECUTE", "LEDGER"];

  /** Reveal panels as they enter; observe the panel, never the clipped text. */
  onMount(() => {
    const root = document.getElementById("snap-root");
    const panels = root.querySelectorAll(".panel");
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) e.target.classList.add("in");
        }
      },
      { root, threshold: 0.55 }
    );
    panels.forEach((p) => io.observe(p));
    return () => io.disconnect();
  });
</script>

<main id="snap-root" class="snap">
  <!-- 1 · HERO -->
  <section class="panel">
    <div class="bg bg-hero"></div>
    <div class="scrim"></div>
    <div class="content content-bottom">
      <p class="rv kicker">MONEY 24 BY DOGS</p>
      <h2 class="rv word">MONEY<span class="text-neon-red">24</span></h2>
      <p class="rv line">An AI that trades every market it can reach.</p>
      <p class="rv line dim">Around the clock.</p>
      <p class="rv pill">IN DEVELOPMENT — PAPER TRADING</p>
    </div>
    <div class="cue" aria-hidden="true">↓</div>
  </section>

  <!-- 2 · MARKETS -->
  <section class="panel">
    <div class="bg bg-markets"></div>
    <div class="scrim"></div>
    <div class="content">
      <h2 class="rv word">5 MARKETS</h2>
      <ul class="markets">
        {#each MARKETS as market, i}
          <li class="rv market" style="transition-delay:{120 + i * 90}ms">{market}</li>
        {/each}
      </ul>
      <p class="rv line dim">One system. Every market. No sleep.</p>
    </div>
  </section>

  <!-- 3 · HOW IT WORKS -->
  <section class="panel">
    <div class="bg bg-flow"></div>
    <div class="scrim"></div>
    <div class="content content-bottom">
      <h2 class="rv word">NONSTOP</h2>
      <p class="rv line">It watches. It sizes. The risk gate decides. Then it moves.</p>
      <ol class="steps">
        {#each STEPS as step, i}
          <li class="rv step" style="transition-delay:{140 + i * 110}ms">
            <span class="step-name">{step}</span>
            {#if i < STEPS.length - 1}<span class="step-arrow" aria-hidden="true">→</span>{/if}
          </li>
        {/each}
      </ol>
      <p class="rv line dim">Every fill lands in the ledger. Nothing off the books.</p>
    </div>
  </section>

  <!-- 4 · DISCIPLINE -->
  <section class="panel">
    <div class="bg bg-discipline"></div>
    <div class="scrim"></div>
    <div class="content">
      <h2 class="rv word">PAPER FIRST</h2>
      <p class="rv line">It proves itself in paper before a real dollar moves.</p>
      <p class="rv line">Hard risk caps on every position.</p>
      <p class="rv line dim">And a kill switch that actually kills.</p>
    </div>
  </section>

  <!-- 5 · STATUS -->
  <section class="panel">
    <div class="bg bg-soon"></div>
    <div class="scrim"></div>
    <div class="content content-bottom">
      <h2 class="rv word">SOON</h2>
      <p class="rv line">In development. Paper trading now, while it earns the keys.</p>
      <a class="badge" href={DOGS_URL} target="_blank" rel="noopener noreferrer">
        <img src={sticker} alt="This product was made by DOGS" width="360" height="119" />
      </a>
      <p class="fineprint">&copy; {CURRENT_YEAR} DOGS. Not financial advice.</p>
    </div>
  </section>
</main>

<style>
  .snap {
    height: 100dvh;
    overflow-y: auto;
    scroll-snap-type: y mandatory;
    overscroll-behavior: contain;
    touch-action: pan-y;
    background: var(--color-app);
    color: white;
    scrollbar-width: none;
  }
  .snap::-webkit-scrollbar {
    display: none;
  }

  .panel {
    position: relative;
    height: 100svh;
    scroll-snap-align: start;
    scroll-snap-stop: always;
    overflow: hidden;
    display: flex;
  }

  /* Full-bleed dark "imagery": layered gradients per panel + slow drift (transform only) */
  .bg {
    position: absolute;
    inset: -12%;
    animation: drift 26s ease-in-out infinite alternate;
    will-change: transform;
  }
  .bg-hero { background: var(--bg-hero); }
  .bg-markets { background: var(--bg-markets); }
  .bg-flow { background: var(--bg-flow); }
  .bg-discipline { background: var(--bg-discipline); }
  .bg-soon { background: var(--bg-soon); }
  @keyframes drift {
    from { transform: translate3d(0, -1.6%, 0) scale(1.02); }
    to { transform: translate3d(0, 1.6%, 0) scale(1.07); }
  }

  .scrim {
    position: absolute;
    inset: 0;
    background: var(--scrim);
  }

  .content {
    position: relative;
    z-index: 1;
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 1.1rem;
    width: 100%;
    max-width: 80rem;
    margin-inline: auto;
    padding: 6rem 1.25rem 5rem;
  }
  .content-bottom {
    justify-content: flex-end;
  }
  @media (min-width: 768px) {
    .content { padding-inline: 3rem; }
  }

  .kicker {
    font-size: 0.8rem;
    font-weight: 700;
    letter-spacing: 0.42em;
    color: var(--color-neon-red);
  }

  .word {
    font-size: clamp(3.4rem, 17vw, 13rem);
    line-height: 0.92;
    font-weight: 800;
    letter-spacing: -0.02em;
    text-wrap: balance;
  }

  .line {
    font-size: clamp(1.15rem, 4.6vw, 2rem);
    font-weight: 300;
    line-height: 1.25;
    max-width: 34ch;
  }
  .dim { color: var(--color-muted); }

  .pill {
    align-self: flex-start;
    margin-top: 0.6rem;
    padding: 0.45rem 0.9rem;
    border: 1px solid var(--pill-border);
    border-radius: 999px;
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.28em;
    color: var(--pill-text);
    background: var(--pill-bg);
  }

  .markets { list-style: none; margin: 0.4rem 0; padding: 0; }
  .market {
    font-size: clamp(1.9rem, 8.5vw, 4.6rem);
    font-weight: 800;
    line-height: 1.04;
    letter-spacing: -0.01em;
    color: white;
  }
  .market:nth-child(even) { color: var(--color-muted); }

  .steps {
    list-style: none;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.55rem 0.8rem;
    margin: 0.4rem 0 0;
    padding: 0;
  }
  .step { display: inline-flex; align-items: center; gap: 0.8rem; }
  .step-name {
    padding: 0.5rem 0.85rem;
    border: 1px solid var(--step-border);
    border-radius: 0.75rem;
    background: var(--step-bg);
    font-size: 0.78rem;
    font-weight: 700;
    letter-spacing: 0.22em;
  }
  .step-arrow { color: var(--color-neon-red); font-weight: 700; }

  /* Badge stays reveal-free (clip-path reveals break replaced elements) */
  .badge {
    display: block;
    width: fit-content;
    margin-top: 1.4rem;
    opacity: 0.85;
    transition: opacity 300ms ease;
  }
  .badge:hover, .badge:focus-visible { opacity: 1; }
  .badge img { height: 2.6rem; width: auto; }
  @media (min-width: 1024px) {
    .badge img { height: 3.4rem; }
  }

  .fineprint {
    font-size: 0.72rem;
    letter-spacing: 0.14em;
    color: var(--color-muted);
  }

  .cue {
    position: absolute;
    right: 1.4rem;
    bottom: 1.4rem;
    z-index: 2;
    color: var(--color-neon-red);
    font-size: 1.3rem;
    animation: cue-drop 2.2s ease-in-out infinite;
  }
  @keyframes cue-drop {
    0%, 100% { transform: translateY(0); opacity: 0.55; }
    50% { transform: translateY(8px); opacity: 1; }
  }

  /* Reveal: observe the panel; clip only text elements */
  .rv {
    opacity: 0;
    transform: translateY(26px);
    clip-path: inset(0 0 100% 0);
    transition: opacity 700ms ease, transform 700ms cubic-bezier(0.22, 1, 0.36, 1), clip-path 700ms cubic-bezier(0.22, 1, 0.36, 1);
  }
  .panel:global(.in) .rv {
    opacity: 1;
    transform: translateY(0);
    clip-path: inset(0 0 0% 0);
  }
  .panel .rv:nth-child(2) { transition-delay: 90ms; }
  .panel .rv:nth-child(3) { transition-delay: 180ms; }
  .panel .rv:nth-child(4) { transition-delay: 270ms; }
  .panel .rv:nth-child(5) { transition-delay: 360ms; }

  @media (prefers-reduced-motion: reduce) {
    .bg, .cue { animation: none; }
    .rv { transition: none; opacity: 1; transform: none; clip-path: none; }
  }
</style>
