import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

// Oldest browsers that get the full app: the first releases with CSS cascade layers, which Tailwind v4 requires.
// Anything older (down to the Chrome 40 Potato Target) gets the static fallback page in index.html instead.
// CSS is lowered by Lightning CSS (oklch -> rgb, color-mix fallbacks, etc.), which costs nothing at runtime.
const APP_CSS_TARGET = ['chrome99', 'edge99', 'firefox97', 'safari15.4', 'ios15.4']
// JS is lowered by Oxc. Safari stays at 16.4: anything lower makes Oxc rewrite every private class field in
// Svelte's runtime into WeakMap helpers for ALL browsers. Safari 15.4-16.3 can still run it natively, and the
// index.html error net swaps in the fallback page if one of them can't.
const APP_JS_TARGET = ['chrome99', 'edge99', 'firefox97', 'safari16.4', 'ios16.4']
// The optional Tauri shell lives in src/tauri; its Rust build output must not trigger dev reloads.
const NATIVE_SHELL_GLOB = '**/src/tauri/**'
// Pages: the landing page, the live news feed and the trading desk (both are desktop app windows), the markets
// page (quotes and charts), the plays board, the wallet, Home (the launch screen: open positions, the paper
// account, quotes and news), and the Terms of Use (plain HTML, no JS).
const PAGES = {
  main: 'index.html',
  home: 'home.html',
  feed: 'feed.html',
  trading: 'trading.html',
  markets: 'markets.html',
  plays: 'plays.html',
  wallet: 'wallet.html',
  terms: 'terms.html',
}
// Tauri's dev window loads this port; it must not drift to another one.
const DEV_PORT = 5173
// The app pages and the terms hold their first paint until the page script has run (blocking="render" in
// their HTML), so moving between pages never shows an empty one. Vite rebuilds the entry script tag
// and drops that attribute; this puts it back. The landing page paints at once and is left alone.
const LANDING_PAGE = '/index.html'
const BUILT_ENTRY_TAG = '<script type="module" crossorigin'
const BLOCKING_ENTRY_TAG = '<script type="module" blocking="render" crossorigin'

/** @returns {import('vite').Plugin} */
function keepRenderBlocking() {
  return {
    name: 'm24-keep-render-blocking',
    apply: 'build',
    transformIndexHtml: {
      order: 'post',
      handler: (html, ctx) => (ctx.path === LANDING_PAGE ? html : html.replace(BUILT_ENTRY_TAG, BLOCKING_ENTRY_TAG)),
    },
  }
}

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    tailwindcss(),
    svelte(),
    keepRenderBlocking()
  ],
  server: {
    port: DEV_PORT,
    strictPort: true,
    watch: { ignored: [NATIVE_SHELL_GLOB] },
  },
  build: {
    target: APP_JS_TARGET,
    cssTarget: APP_CSS_TARGET,
    rolldownOptions: { input: PAGES },
  },
})
