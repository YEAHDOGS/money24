<script>
  /**
   * The Money 24 landing page: full-screen panels on one spine, one idea each. What it is, how a
   * play is made, the three power modes, the markets, the demo and its snapshot, the public
   * sources behind it, and how unfinished it all is. Copy lives in landing/landing.json and
   * landing/sources.json. The hero's lettering is the brand: its markup and styles do not change.
   */
  import { onMount } from "svelte";
  import ArrowRight from "@lucide/svelte/icons/arrow-right";
  import ArrowUpRight from "@lucide/svelte/icons/arrow-up-right";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import sticker from "./assets/made-by-dogs-sticker.webp";
  import logo from "./assets/dogs-logo.svg";
  import NoticeBar from "./lib/NoticeBar.svelte";
  import Waitlist from "./landing/Waitlist.svelte";
  import copy from "./landing/landing.json";
  import sourceList from "./landing/sources.json";

  const DOGS_URL = "https://dogs.red";
  const CURRENT_YEAR = new Date().getFullYear();

  const MARKETS = ["CRYPTO", "OPTIONS", "STOCKS", "FUTURES", "PREDICTION", "FOREX"];
  const POWER_BARS = [1, 2, 3];
  /** The spine: one node per panel, top to bottom. */
  const PANELS = [
    { id: "top", label: "Money 24" },
    { id: "loop", label: "How a play is made" },
    { id: "modes", label: "Power modes" },
    { id: "markets", label: "Markets" },
    { id: "demo", label: "The demo" },
    { id: "sources", label: "Public sources" },
    { id: "paper", label: "Paper first" },
    { id: "wip", label: "Not active yet" },
    { id: "soon", label: "Join the waitlist" },
  ];
  const REVEAL_THRESHOLD = 0.55;

  let current = $state(PANELS[0].id);
  /** Reveal panels as they enter; observe the panel, never the clipped text. */
  onMount(() => {
    const root = document.getElementById("snap-root");
    const panels = root.querySelectorAll(".panel");
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (!e.isIntersecting) continue;
          e.target.classList.add("in");
          current = e.target.id;
        }
      },
      { root, threshold: REVEAL_THRESHOLD }
    );
    panels.forEach((p) => io.observe(p));
    return () => io.disconnect();
  });
</script>

<NoticeBar overlay />

<!-- The DOGS logo (logo.dogs.red), top left on every panel. Fixed, so it moves nothing. -->
<a class="dogs-mark" href={DOGS_URL} target="_blank" rel="noopener noreferrer"><img src={logo} alt="DOGS" width="540" height="534" /></a>

<!-- Always within reach, on every panel: the way into the demo. Fixed, so it moves nothing. -->
<a class="demo-link" href={copy.cta.demoHref}>{copy.cta.demo}<ArrowRight size={14} strokeWidth={2.5} aria-hidden="true" /></a>

<!-- THE SPINE: one node per panel; the red one is where you are. -->
<nav class="spine" aria-label="Sections">
  <a href="#{PANELS[0].id}" class="to-top" class:hidden-top={current === PANELS[0].id} aria-label={copy.toTop} title={copy.toTop}><ArrowUp size={14} strokeWidth={3} aria-hidden="true" /></a>
  {#each PANELS as p (p.id)}
    <a href="#{p.id}" class:here={current === p.id} aria-current={current === p.id ? "true" : undefined} aria-label={p.label} title={p.label}></a>
  {/each}
</nav>

<main id="snap-root" class="snap">
  <!-- 1 · HERO -->
  <section class="panel" id="top">
    <div class="bg bg-hero"></div>
    <div class="scrim"></div>
    <div class="content content-bottom">
      <p class="rv kicker">MONEY 24 BY DOGS</p>
      <h2 class="rv word">MONEY<span class="text-neon-red">24</span></h2>
      <p class="rv line">An AI that trades every market it can reach.</p>
      <p class="rv line dim">Around the clock.</p>
      <p class="rv pill">IN DEVELOPMENT</p>
    </div>
    <!-- Out of the flex column on purpose: absolutely placed in the room under the pill, so the lettering never moves. -->
    <p class="hero-cta">
      <a class="cta cta-solid" href={copy.cta.demoHref}>{copy.cta.demo}<ArrowRight size={16} strokeWidth={2.5} aria-hidden="true" /></a>
      <a class="cta" href={copy.cta.tourHref}>{copy.cta.tour}</a>
    </p>
    <div class="cue" aria-hidden="true">↓</div>
  </section>

  <!-- 2 · HOW A PLAY IS MADE -->
  <section class="panel" id="loop">
    <div class="bg bg-flow"></div>
    <div class="scrim"></div>
    <div class="content content-bottom fit">
      <h2 class="rv word">{copy.loop.title}</h2>
      <p class="rv line">{copy.loop.line}</p>
      <ol class="steps">
        {#each copy.loop.steps as step, i (step)}
          <li class="rv step" style="transition-delay:{140 + i * 110}ms">
            <span class="step-name">{step}</span>
            {#if i < copy.loop.steps.length - 1}<span class="step-arrow" aria-hidden="true">→</span>{/if}
          </li>
        {/each}
      </ol>
      <p class="rv line dim">{copy.loop.foot}</p>
    </div>
  </section>

  <!-- 3 · POWER MODES -->
  <section class="panel" id="modes">
    <div class="bg bg-discipline"></div>
    <div class="scrim"></div>
    <div class="content fit">
      <div class="head">
        <h2 class="rv word-md">{copy.modes.title}</h2>
        <p class="rv pill pill-inline">{copy.modes.planned}</p>
      </div>
      <p class="rv line">{copy.modes.line}</p>
      <ul class="grid-list modes">
        {#each copy.modes.list as mode, i (mode.id)}
          <li class="rv mode mode-{mode.id}" style="transition-delay:{160 + i * 120}ms">
            <div class="mode-top">
              <span class="meter" aria-hidden="true">
                {#each POWER_BARS as bar (bar)}
                  <span class="bar" class:lit={bar <= mode.level} style="--h:{bar / POWER_BARS.length}; --d:{360 + i * 120 + bar * 90}ms"></span>
                {/each}
              </span>
              <h3 class="mode-name">{mode.name}</h3>
              <span class="risk">{copy.modes.riskLabel}: {mode.risk}</span>
            </div>
            <p class="mode-text">{mode.text}</p>
          </li>
        {/each}
      </ul>
      <p class="rv line dim small">{copy.modes.foot}</p>
    </div>
  </section>

  <!-- 4 · MARKETS -->
  <section class="panel" id="markets">
    <div class="bg bg-markets"></div>
    <div class="scrim"></div>
    <div class="content fit">
      <h2 class="rv word">6 MARKETS</h2>
      <ul class="markets">
        {#each MARKETS as market, i}
          <li class="rv market" style="transition-delay:{120 + i * 90}ms">{market}</li>
        {/each}
      </ul>
      <p class="rv line dim">One system. Every market. No sleep.</p>
    </div>
  </section>

  <!-- 5 · THE DEMO -->
  <section class="panel" id="demo">
    <div class="bg bg-hero"></div>
    <div class="scrim"></div>
    <div class="content fit">
      <h2 class="rv word-md">{copy.demo.title}</h2>
      <p class="rv line">{copy.demo.line}</p>
      <ul class="grid-list doors">
        {#each copy.demo.doors as door, i (door.id)}
          <li class="rv" style="transition-delay:{160 + i * 100}ms">
            <a class="door" href={door.href}>
              <span class="door-name">{door.name}<ArrowRight class="door-arrow" size={18} strokeWidth={2.5} aria-hidden="true" /></span>
              <span class="door-text">{door.text}</span>
            </a>
          </li>
        {/each}
      </ul>
    </div>
  </section>

  <!-- 6 · PUBLIC SOURCES -->
  <section class="panel" id="sources">
    <div class="bg bg-markets"></div>
    <div class="scrim"></div>
    <div class="content fit">
      <h2 class="rv word-md">{copy.sources.title}</h2>
      <p class="rv line">{copy.sources.line}</p>
      <ul class="grid-list source-groups">
        {#each sourceList.groups as group, i (group.id)}
          <li class="rv source-group" style="transition-delay:{160 + i * 100}ms">
            <h3 class="group-name">{group.name}</h3>
            <ul>
              {#each group.sources as source (source.url)}
                <li class="source">
                  <a href={source.url} target="_blank" rel="noopener noreferrer" title={copy.sources.visit}>{source.name}<ArrowUpRight size={12} strokeWidth={2.5} aria-hidden="true" /></a>
                  <span class="source-what">{source.what}</span>
                </li>
              {/each}
            </ul>
          </li>
        {/each}
      </ul>
    </div>
  </section>

  <!-- 7 · DISCIPLINE -->
  <section class="panel" id="paper">
    <div class="bg bg-discipline"></div>
    <div class="scrim"></div>
    <div class="content fit">
      <h2 class="rv word">PAPER FIRST</h2>
      <p class="rv line">It proves itself in paper before a real dollar moves.</p>
      <p class="rv line">Hard risk caps on every position.</p>
      <p class="rv line dim">And a kill switch that actually kills.</p>
    </div>
  </section>

  <!-- 8 · NOT ACTIVE YET -->
  <section class="panel" id="wip">
    <div class="bg bg-flow"></div>
    <div class="scrim"></div>
    <div class="content fit">
      <div class="hazard-rule rv" aria-hidden="true"></div>
      <h2 class="rv word wip-word">{copy.wip.title}</h2>
      <ul class="wip-lines">
        {#each copy.wip.lines as line, i (line)}
          <li class="rv line" class:dim={i > 0} style="transition-delay:{180 + i * 90}ms">{line}</li>
        {/each}
      </ul>
      <p class="rv"><a class="cta" href="terms.html">Terms of Use</a></p>
    </div>
  </section>

  <!-- 9 · STATUS -->
  <section class="panel" id="soon">
    <div class="bg bg-soon"></div>
    <div class="scrim"></div>
    <div class="content content-bottom">
      <h2 class="rv word">SOON</h2>
      <p class="rv line">In Development.</p>
      <Waitlist />
      <a class="badge" href={DOGS_URL} target="_blank" rel="noopener noreferrer">
        <img src={sticker} alt="This product was made by DOGS" width="360" height="119" />
      </a>
      <p class="fineprint">&copy; {CURRENT_YEAR} DOGS. Not financial advice.</p>
    </div>
    <!-- Out of the flex column on purpose: absolutely placed, so the wordmark and badge never move. -->
    <p class="legal"><a href="terms.html">Terms of Use</a></p>
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
    scroll-behavior: smooth;
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
    font-size: clamp(1.9rem, min(8.5vw, 6.6vh), 4.6rem); /* by height too, so six of them fit under the heading */
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

  /* Terms link: pinned under the last panel content, outside its flex column, so adding it reflows nothing */
  .legal {
    position: absolute;
    inset-inline: 0;
    bottom: 1.4rem;
    z-index: 1;
    max-width: 80rem;
    margin: 0 auto;
    padding-inline: 1.25rem;
    font-size: 0.72rem;
    letter-spacing: 0.14em;
  }
  @media (min-width: 768px) {
    .legal { padding-inline: 3rem; }
  }
  .legal a {
    color: var(--color-muted);
    text-decoration: underline;
    text-underline-offset: 0.25em;
    transition: color 150ms cubic-bezier(0.34, 1.56, 0.64, 1);
  }
  .legal a:hover, .legal a:focus-visible { color: white; }
  .legal a:focus-visible { outline: 2px solid var(--color-neon-red); outline-offset: 3px; }

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

  /* The DOGS logo: fixed under the notice strip, top left, on every panel. */
  .dogs-mark {
    position: fixed;
    top: 2.2rem;
    left: 1.25rem;
    z-index: 30;
    opacity: 0.9;
    transition: transform 150ms cubic-bezier(0.34, 1.56, 0.64, 1), opacity 150ms ease;
  }
  .dogs-mark img { height: 2.4rem; width: auto; }
  .dogs-mark:hover, .dogs-mark:focus-visible { opacity: 1; transform: scale(1.08); }
  .dogs-mark:focus-visible { outline: 2px solid white; outline-offset: 3px; }
  @media (min-width: 768px) {
    .dogs-mark { left: 3rem; }
    .dogs-mark img { height: 3rem; }
  }

  /* ── The way into the demo ──────────────────────────────────────────────── */
  /* Fixed under the notice strip, top right, on every panel. */
  .demo-link {
    position: fixed;
    top: 2.4rem;
    right: 1.25rem;
    z-index: 30;
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.4rem 0.75rem;
    border: 1px solid var(--pill-border);
    border-radius: 999px;
    background: var(--step-bg);
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.2em;
    text-transform: uppercase;
    color: white;
    transition: transform 150ms cubic-bezier(0.34, 1.56, 0.64, 1), background-color 150ms ease, border-color 150ms ease;
  }
  .demo-link:hover, .demo-link:focus-visible { background: var(--color-neon-red); border-color: var(--color-neon-red); }
  .demo-link:active { transform: scale(0.96); }
  .demo-link:focus-visible, .cta:focus-visible, .door:focus-visible, .source a:focus-visible {
    outline: 2px solid white;
    outline-offset: 3px;
  }
  @media (min-width: 768px) {
    .demo-link { right: 3rem; }
  }

  /* Hero buttons: in the 5rem of room under the pill, placed absolutely like .legal. */
  .hero-cta {
    position: absolute;
    inset-inline: 0;
    bottom: 1.15rem;
    z-index: 1;
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
    max-width: 80rem;
    margin: 0 auto;
    padding-inline: 1.25rem;
  }
  @media (min-width: 768px) {
    .hero-cta { padding-inline: 3rem; }
  }
  .cta {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.6rem 1rem;
    border: 1px solid var(--step-border);
    background: var(--step-bg);
    font-size: 0.74rem;
    font-weight: 700;
    letter-spacing: 0.2em;
    text-transform: uppercase;
    color: white;
    transition: transform 150ms cubic-bezier(0.34, 1.56, 0.64, 1), background-color 150ms ease, border-color 150ms ease, color 150ms ease;
  }
  .cta:hover, .cta:focus-visible { border-color: white; }
  .cta:active { transform: scale(0.96); }
  .cta-solid { border-color: var(--color-neon-red); background: var(--color-neon-red); }
  .cta-solid:hover, .cta-solid:focus-visible { background: white; border-color: white; color: var(--color-app); }

  /* ── The spine ──────────────────────────────────────────────────────────── */
  .spine {
    position: fixed;
    top: 50%;
    right: 0.3rem;
    z-index: 30;
    display: flex;
    flex-direction: column;
    transform: translateY(-50%);
  }
  /* The 1px line the nodes hang on. */
  .spine::before {
    content: "";
    position: absolute;
    top: 2.4rem;
    bottom: 0.5rem;
    left: 50%;
    width: 1px;
    background: var(--step-border);
  }
  .spine a {
    position: relative;
    display: grid;
    place-items: center;
    width: 1rem;
    height: 1rem;
  }
  .spine a::after {
    content: "";
    width: 0.3rem;
    height: 0.3rem;
    border-radius: 999px;
    background: var(--color-muted);
    transition: transform 220ms cubic-bezier(0.34, 1.56, 0.64, 1), background-color 150ms ease;
  }
  .spine a:hover::after, .spine a:focus-visible::after { background: white; transform: scale(1.5); }
  .spine a.here::after { background: var(--color-neon-red); transform: scale(2); }
  .spine a:focus-visible { outline: none; }
  /* Back to the top: an arrow above the first node. It fades out on the first panel, where it has nothing to do. */
  .spine a.to-top {
    width: 1.6rem;
    height: 1.6rem;
    margin: 0 -0.3rem 0.3rem;
    border: 1px solid var(--pill-border);
    border-radius: 999px;
    background: var(--step-bg);
    color: white;
    transition: transform 150ms cubic-bezier(0.34, 1.56, 0.64, 1), opacity 200ms ease, background-color 150ms ease;
  }
  .spine a.to-top::after { content: none; }
  .spine a.to-top:hover, .spine a.to-top:focus-visible { background: var(--color-neon-red); transform: translateY(-2px); }
  .spine a.to-top.hidden-top { opacity: 0; pointer-events: none; }
  @media (min-width: 768px) {
    .spine { right: 1rem; }
  }

  /* ── Panels that hold a list: tighter, and the list scrolls when a small screen runs out ── */
  .fit {
    padding-block: 5.8rem 2.6rem;
    gap: 0.85rem;
  }
  .word-md {
    font-size: clamp(2.2rem, 9.5vw, 6.5rem);
    line-height: 0.95;
    font-weight: 800;
    letter-spacing: -0.02em;
  }
  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.4rem 1rem;
  }
  .pill-inline { margin-top: 0; align-self: center; }
  .small { font-size: clamp(0.95rem, 3vw, 1.2rem); max-width: none; }

  /* Edge to edge on a 1px grid: cells share their borders, and the list is the only thing that scrolls. */
  .grid-list {
    list-style: none;
    display: grid;
    min-height: 0;
    margin: 0.2rem -1.25rem 0;
    padding: 0;
    overflow-y: auto;
    border-block: 1px solid var(--step-border);
    /* No scrollbar is ever drawn on the landing page. A list too long for a squat window still moves with
       the wheel, a drag or a swipe; and a row sliding in as it is revealed can no longer flash a bar. */
    scrollbar-width: none;
  }
  .grid-list::-webkit-scrollbar, .steps::-webkit-scrollbar { display: none; }
  .steps { scrollbar-width: none; }
  .grid-list > li {
    min-width: 0;
    border-bottom: 1px solid var(--step-border);
    background: var(--step-bg);
  }
  .grid-list > li:last-child { border-bottom: 0; }
  @media (min-width: 768px) {
    .grid-list {
      margin-inline: 0;
      border-inline: 1px solid var(--step-border);
    }
    .grid-list > li {
      border-bottom: 0;
      border-right: 1px solid var(--step-border);
    }
    .grid-list > li:last-child { border-right: 0; }
    .modes { grid-template-columns: repeat(3, minmax(0, 1fr)); }
    .doors { grid-template-columns: repeat(3, minmax(0, 1fr)); }
    .source-groups { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .source-groups > li:nth-child(2n) { border-right: 0; }
    .source-groups > li:nth-child(-n + 2) { border-bottom: 1px solid var(--step-border); }
  }
  @media (min-width: 1100px) {
    .source-groups { grid-template-columns: repeat(4, minmax(0, 1fr)); }
    .source-groups > li:nth-child(2n) { border-right: 1px solid var(--step-border); }
    .source-groups > li:nth-child(-n + 2) { border-bottom: 0; }
    .source-groups > li:last-child { border-right: 0; }
  }

  /* Power modes */
  .mode { padding: 0.9rem 1.25rem; }
  .mode-top {
    display: flex;
    align-items: flex-end;
    gap: 0.7rem;
  }
  .mode-name {
    font-size: clamp(1.5rem, 5vw, 2.6rem);
    font-weight: 800;
    line-height: 0.9;
    letter-spacing: -0.01em;
  }
  .risk {
    margin-left: auto;
    padding: 0.2rem 0.5rem;
    border: 1px solid var(--step-border);
    font-size: 0.62rem;
    font-weight: 700;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    white-space: nowrap;
    color: var(--color-muted);
  }
  .mode-high .risk { border-color: var(--color-neon-red); color: var(--pill-text); }
  .mode-medium .risk { border-color: var(--color-warn); color: var(--color-warn); }
  /* The meter: three bars; the lit ones rise when the panel arrives. */
  .meter {
    display: flex;
    align-items: flex-end;
    gap: 3px;
    height: 1.9rem;
  }
  .bar {
    width: 0.4rem;
    height: calc(var(--h) * 100%);
    background: var(--step-border);
    transform: scaleY(0.25);
    transform-origin: bottom;
    transition: transform 520ms cubic-bezier(0.34, 1.56, 0.64, 1) var(--d), background-color 300ms ease var(--d);
  }
  .panel:global(.in) .bar { transform: scaleY(1); }
  .panel:global(.in) .bar.lit { background: white; }
  .panel:global(.in) .mode-medium .bar.lit { background: var(--color-warn); }
  .panel:global(.in) .mode-high .bar.lit { background: var(--color-neon-red); }
  .mode-text {
    margin-top: 0.6rem;
    font-size: 0.92rem;
    line-height: 1.4;
    color: var(--color-muted);
  }

  /* Demo doors */
  .door {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    height: 100%;
    padding: 0.9rem 1.25rem;
    transition: background-color 150ms ease;
  }
  .door:hover, .door:focus-visible { background: var(--pill-bg); }
  .door-name {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    font-size: clamp(1.3rem, 4.4vw, 2rem);
    font-weight: 800;
    line-height: 1;
    letter-spacing: -0.01em;
  }
  .door :global(.door-arrow) {
    flex: none;
    color: var(--color-neon-red);
    transition: transform 150ms cubic-bezier(0.34, 1.56, 0.64, 1);
  }
  .door:hover :global(.door-arrow), .door:focus-visible :global(.door-arrow) { transform: translateX(5px); }
  .door-text {
    font-size: 0.9rem;
    line-height: 1.35;
    color: var(--color-muted);
  }

  /* Sources */
  .source-group { padding: 0.8rem 1.25rem; }
  .group-name {
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.24em;
    text-transform: uppercase;
    color: var(--color-neon-red);
  }
  .source-group ul { list-style: none; margin: 0.4rem 0 0; padding: 0; }
  .source { padding: 0.28rem 0; }
  .source a {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    font-size: 0.95rem;
    font-weight: 700;
    text-decoration: underline;
    text-decoration-color: var(--step-border);
    text-underline-offset: 0.25em;
    transition: color 150ms ease, text-decoration-color 150ms ease;
  }
  .source a:hover, .source a:focus-visible { color: var(--pill-text); text-decoration-color: var(--color-neon-red); }
  .source-what {
    display: block;
    font-size: 0.78rem;
    line-height: 1.3;
    color: var(--color-muted);
  }

  /* Not active yet. Nothing in this panel scrolls, ever: its lettering, lines and spacing take their size
     from the window's height as well as its width, so the whole notice fits whatever the window's shape. */
  #wip .content {
    padding-block: max(3.6rem, min(5.8rem, 11vh)) min(2.6rem, 6vh);
    gap: min(0.85rem, 1.6vh);
  }
  #wip .word { font-size: clamp(1.5rem, min(17vw, 15vh), 13rem); }
  #wip .line { font-size: clamp(0.75rem, min(4.6vw, 2.7vh), 2rem); }
  .wip-lines {
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: min(0.85rem, 1.6vh);
    margin: 0;
    padding: 0;
  }
  .hazard-rule {
    width: min(100%, 28rem);
    height: 0.6rem;
    background: var(--bg-hazard);
  }
  .wip-word { color: var(--color-warn); }

  /* ── Phones: the browser's own bar covers the bottom of the screen, so every panel keeps clear of it. ── */
  @media (max-width: 767px) {
    .content { padding-bottom: 9rem; }
    .fit { padding-bottom: 4.5rem; }
    .hero-cta { bottom: 4rem; }
    .legal { bottom: 3.4rem; }
    .cue { display: none; }
    /* Edge-to-edge cells keep clear of the spine on the right edge. */
    .mode, .door, .source-group { padding-right: 2.75rem; }
    /* Sources are names only, side by side: four groups fit one screen with nothing to scroll. */
    .source-group { padding-block: 0.55rem; }
    .source-group ul { display: flex; flex-wrap: wrap; gap: 0 1rem; margin-top: 0.2rem; }
    .source { padding: 0.12rem 0; }
    .source a { font-size: 0.9rem; }
    .source-what { display: none; }
  }

  /* ── The narrowest phones (320 wide): the two hero buttons stay on one row, clear of the pill above,
     and the list headings stay on one line so their lists get the room. ── */
  @media (max-width: 359px) {
    .hero-cta { flex-wrap: nowrap; gap: 0.4rem; }
    .hero-cta .cta { padding: 0.55rem 0.6rem; font-size: 0.64rem; letter-spacing: 0.12em; white-space: nowrap; }
    .word-md { font-size: 1.9rem; }
    .mode { padding-block: 0.6rem; }
    .mode-text { margin-top: 0.3rem; }
  }

  /* ── Short screens (phones on their side, squat windows down to 320 high): everything tightens so
     nothing is cut off. The hero's lettering only changes here, where it used to be cropped. ── */
  @media (max-height: 560px) {
    .content, .fit { padding-block: 3.6rem 2.6rem; gap: 0.45rem; }
    .word { font-size: clamp(1.9rem, 13vh, 5rem); }
    .word-md { font-size: clamp(1.5rem, 10vh, 3rem); }
    .line { font-size: clamp(0.9rem, 4.4vh, 1.3rem); }
    .kicker { font-size: 0.64rem; }
    .pill { margin-top: 0.1rem; padding: 0.25rem 0.6rem; font-size: 0.6rem; }
    .market { font-size: clamp(1rem, 7vh, 2rem); }
    .markets { display: flex; flex-wrap: wrap; gap: 0 0.8rem; margin: 0; }
    .step-name { padding: 0.25rem 0.5rem; font-size: 0.6rem; }
    .steps { gap: 0.3rem 0.5rem; margin: 0; min-height: 0; overflow-y: auto; }
    .hazard-rule { height: 0.3rem; flex: none; }
    .hero-cta { bottom: 0.4rem; }
    .cta { padding: 0.3rem 0.6rem; font-size: 0.62rem; }
    .demo-link { top: 1.9rem; padding: 0.2rem 0.55rem; font-size: 0.58rem; }
    .dogs-mark { top: 1.8rem; }
    .dogs-mark img { height: 1.5rem; }
    .mode, .door, .source-group { padding-block: 0.45rem; }
    .mode-text, .door-text { font-size: 0.78rem; }
    .badge { margin-top: 0.3rem; }
    .legal { bottom: 0.5rem; }
    .cue { display: none; }
  }

  @media (prefers-reduced-motion: reduce) {
    .snap { scroll-behavior: auto; }
    .bg, .cue { animation: none; }
    .rv { transition: none; opacity: 1; transform: none; clip-path: none; }
    .bar { transition: none; transform: scaleY(1); }
    .spine a::after, .demo-link, .dogs-mark, .cta, .door, .door :global(.door-arrow) { transition: none; }
  }
</style>
