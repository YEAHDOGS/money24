// The chart behind the Markets page: one price pane with overlays and volume, optional RSI and MACD
// panes below it, a crosshair legend and two drawing tools. Drawn by TradingView's open-source
// lightweight-charts; this module owns everything about it so the component only holds controls.

import {
  AreaSeries, BarSeries, CandlestickSeries, HistogramSeries, LineSeries, CrosshairMode, LineStyle, PriceScaleMode, createChart,
} from 'lightweight-charts';
import { bollinger, ema, macd, rsi, sma, vwap } from './indicators.js';

export const TYPE_AREA = 'area';
export const TYPE_CANDLES = 'candles';
export const TYPE_BARS = 'bars';
export const TYPE_LINE = 'line';
export const TOOL_LEVEL = 'level';
export const TOOL_TREND = 'trend';
export const SCALE_LINEAR = 'linear';
export const SCALE_LOG = 'log';
export const SCALE_PERCENT = 'percent';
/** Every study the chart can draw, in menu order. `pane` studies get their own pane under the price. */
export const STUDIES = [
  { id: 'volume', token: '--color-muted' },
  { id: 'sma20', token: '--color-kind-news', bars: 20 },
  { id: 'sma50', token: '--color-kind-filing', bars: 50 },
  { id: 'ema200', token: '--color-kind-insider', bars: 200 },
  { id: 'bollinger', token: '--color-kind-central-bank', bars: 20 },
  { id: 'vwap', token: '--color-kind-macro' },
  { id: 'rsi', token: '--color-kind-insider', bars: 14, pane: true },
  { id: 'macd', token: '--color-kind-news', pane: true },
];

const SCALE_MODES = { [SCALE_LINEAR]: PriceScaleMode.Normal, [SCALE_LOG]: PriceScaleMode.Logarithmic, [SCALE_PERCENT]: PriceScaleMode.Percentage };
const DAILY_INTERVALS = new Set(['1d', '1wk', '1mo']);
const SECONDS_PER_DAY = 86400;
const SECONDS_PER_HOUR = 3600;
const MS_PER_SECOND = 1000;
const HEX_COLOR = /^#[0-9a-f]{6}$/i;
const HEX_RADIX = 16;
const RED_SHIFT = 16;
const GREEN_SHIFT = 8;
const BYTE_MASK = 0xff;
const PERCENT = 100;
const AREA_TOP_ALPHA = 0.3;
const AREA_EXT_ALPHA = 0.1;
const VOLUME_ALPHA = 0.45;
const GAP_ALPHA = 0.6;
const BAND_FILL_ALPHA = 0.55;
const VOLUME_SCALE = 'volume';
const VOLUME_TOP = 0.82; // volume bars fill the bottom 18% of the price pane
const PRICE_BOTTOM_WITH_VOLUME = 0.22;
const PRICE_BOTTOM = 0.08;
const PRICE_TOP = 0.08;
const MAIN_STRETCH = 4; // the price pane is four times as tall as each study pane
const STUDY_STRETCH = 1;
const RIGHT_OFFSET_BARS = 3;
const BOLLINGER_WIDTH = 2;
const MACD_FAST = 12;
const MACD_SLOW = 26;
const MACD_SIGNAL = 9;
const RSI_HIGH = 70;
const RSI_LOW = 30;
const TREND_WIDTH = 2;
const ANCHOR_RADIUS = 3;
const FULL_TURN = Math.PI * 2;
const SMALL_PRICE = 1;
const TINY_PRICE = 0.01;
const DECIMALS = 2;
const SMALL_DECIMALS = 4;
const TINY_DECIMALS = 8;
const TOKENS = {
  up: '--color-ok', down: '--color-bad', muted: '--color-muted', line: '--color-line', accent: '--color-neon-red', draw: '--color-warn',
};

/** @typedef {{t:number, o:number, h:number, l:number, c:number, v:number}} Candle */
/** @typedef {{time:number, t:number, open:number, high:number, low:number, close:number, volume:number, ext:boolean}} Bar */
/** @typedef {{kind:'level', p:number} | {kind:'trend', a:{t:number,p:number}, b:{t:number,p:number}}} Drawing */

/** Reads a theme token, so the chart's colors come from src/styles/theme.css like everything else. */
function token(/** @type {string} */ name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

/** The same color, see-through. Tokens that are not plain hex come back unchanged. */
function alpha(/** @type {string} */ color, /** @type {number} */ a) {
  if (!HEX_COLOR.test(color)) return color;
  const n = parseInt(color.slice(1), HEX_RADIX);
  return `rgba(${n >> RED_SHIFT}, ${(n >> GREEN_SHIFT) & BYTE_MASK}, ${n & BYTE_MASK}, ${a})`;
}

/** Seconds the exchange's clock is ahead of UTC at each moment, so the axis reads in market time. */
function zoneShift(/** @type {string} */ zone) {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: zone || 'UTC', hourCycle: 'h23', year: 'numeric', month: 'numeric', day: 'numeric', hour: 'numeric', minute: 'numeric', second: 'numeric',
  });
  /** @type {Map<number, number>} the offset only changes on the hour, so one lookup per hour */
  const byHour = new Map();
  return (/** @type {number} */ t) => {
    const hour = Math.floor(t / SECONDS_PER_HOUR);
    let shift = byHour.get(hour);
    if (shift === undefined) {
      const p = Object.fromEntries(parts.formatToParts(t * MS_PER_SECOND).map((x) => [x.type, Number(x.value)]));
      shift = Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute, p.second) / MS_PER_SECOND - t;
      byHour.set(hour, shift);
    }
    return shift;
  };
}

export class ChartEngine {
  /** @type {Bar[]} */
  bars = [];
  #chart;
  #main = null;
  /** @type {{study:string, label:string, color:string, values:(number|null)[], series:any, histogram?:boolean}[]} */
  #lines = [];
  #levels = [];
  #type = TYPE_AREA;
  /** @type {Set<string>} */
  #studies = new Set();
  /** @type {Drawing[]} */
  #drawings = [];
  #tool = '';
  /** @type {{t:number, p:number}|null} first point of a trend line being drawn */
  #anchor = null;
  /** @type {{t:number, p:number}|null} where the pointer is while the second point is chosen */
  #preview = null;
  #previousClose = 0;
  #showBaseline = false;
  #colors;
  #events;
  #redraw = () => {};
  #overlay;

  /**
   * @param {HTMLElement} el
   * @param {{onLegend:(l:any)=>void, onDrawings:(d:Drawing[])=>void, onTool:(t:string)=>void}} events
   */
  constructor(el, events) {
    this.#events = events;
    this.#colors = Object.fromEntries(Object.entries(TOKENS).map(([k, v]) => [k, token(v)]));
    const c = this.#colors;
    this.#chart = createChart(el, {
      autoSize: true,
      layout: {
        background: { color: 'transparent' },
        textColor: c.muted,
        fontFamily: getComputedStyle(el).fontFamily,
        panes: { separatorColor: c.line, separatorHoverColor: c.line, enableResize: true },
      },
      grid: { vertLines: { color: c.line }, horzLines: { color: c.line } },
      crosshair: { mode: CrosshairMode.Normal },
      rightPriceScale: { borderColor: c.line },
      timeScale: { borderColor: c.line, rightOffset: RIGHT_OFFSET_BARS, secondsVisible: false },
    });
    this.#chart.subscribeCrosshairMove(this.#onMove);
    this.#chart.subscribeClick(this.#onClick);
    // Trend lines are drawn straight onto the price pane, in step with every pan and zoom.
    const view = { renderer: () => ({ draw: (/** @type {any} */ target) => target.useBitmapCoordinateSpace(this.#paint) }) };
    const views = [view];
    this.#overlay = {
      attached: (/** @type {any} */ p) => { this.#redraw = p.requestUpdate; },
      detached: () => { this.#redraw = () => {}; },
      paneViews: () => views,
    };
  }

  destroy() {
    this.#chart.remove();
  }

  /**
   * Loads candles. `fit` zooms out to show them all; leave it off for a live refresh so the
   * reader's pan and zoom are kept.
   * @param {Candle[]} candles
   * @param {{interval:string, timezone:string, previousClose:number, baseline:boolean, regular:{start:number,end:number}, fit:boolean}} opts
   */
  load(candles, opts) {
    const daily = DAILY_INTERVALS.has(opts.interval);
    const shift = zoneShift(opts.timezone);
    // A regular session shorter than a day means the exchange has extended hours to mark.
    const open = opts.regular.start + shift(opts.regular.start);
    const length = opts.regular.end - opts.regular.start;
    const sessions = !daily && length > 0 && length < SECONDS_PER_DAY - SECONDS_PER_HOUR;
    /** @type {Bar[]} */
    const bars = [];
    for (const k of candles) {
      let time = k.t + shift(k.t);
      if (daily) time -= time % SECONDS_PER_DAY;
      const sinceOpen = (((time - open) % SECONDS_PER_DAY) + SECONDS_PER_DAY) % SECONDS_PER_DAY;
      const bar = { time, t: k.t, open: k.o, high: k.h, low: k.l, close: k.c, volume: k.v, ext: sessions && sinceOpen >= length };
      // The newest daily bar can arrive twice (yesterday's close and today's live bar): keep the later one.
      if (bars.length && time <= bars[bars.length - 1].time) bars[bars.length - 1] = { ...bar, time: bars[bars.length - 1].time };
      else bars.push(bar);
    }
    this.bars = bars;
    this.#previousClose = opts.previousClose;
    this.#showBaseline = opts.baseline;
    this.#chart.applyOptions({ timeScale: { timeVisible: !daily } });
    this.#build();
    if (opts.fit) this.#chart.timeScale().fitContent();
    this.#legend(bars.length - 1);
  }

  setType(/** @type {string} */ type) {
    this.#type = type;
    this.#build();
  }

  setStudies(/** @type {string[]} */ ids) {
    this.#studies = new Set(ids);
    this.#build();
  }

  setScale(/** @type {string} */ scale) {
    this.#chart.priceScale('right').applyOptions({ mode: SCALE_MODES[scale] });
  }

  setDrawings(/** @type {Drawing[]} */ drawings) {
    this.#drawings = drawings;
    this.#build();
  }

  /** Arms a drawing tool; the next click (two for a trend line) places it. Empty disarms. */
  setTool(/** @type {string} */ tool) {
    this.#tool = tool;
    this.#anchor = null;
    this.#preview = null;
    this.#redraw();
  }

  fit() {
    this.#chart.timeScale().fitContent();
  }

  /** Decimals a price of this size needs: cents above a dollar, more below. */
  static decimals(/** @type {number} */ price) {
    if (price >= SMALL_PRICE || price <= 0) return DECIMALS;
    return price >= TINY_PRICE ? SMALL_DECIMALS : TINY_DECIMALS;
  }

  /** Removes every series and draws them again from the current bars, type, studies and drawings. */
  #build() {
    const chart = this.#chart;
    const c = this.#colors;
    const bars = this.bars;
    if (this.#main) chart.removeSeries(this.#main);
    for (const l of this.#lines) chart.removeSeries(l.series);
    this.#lines = [];
    this.#levels = [];
    this.#main = null;
    if (!bars.length) return;

    const last = bars[bars.length - 1].close;
    const decimals = ChartEngine.decimals(last);
    const priceFormat = { type: 'price', precision: decimals, minMove: 1 / 10 ** decimals };
    const trend = last >= (this.#previousClose || bars[0].open) ? c.up : c.down;
    const closes = bars.map((b) => b.close);
    const quiet = { priceLineVisible: false, lastValueVisible: false, crosshairMarkerVisible: false };

    if (this.#type === TYPE_CANDLES) {
      this.#main = chart.addSeries(CandlestickSeries, { upColor: c.up, downColor: c.down, wickUpColor: c.up, wickDownColor: c.down, borderVisible: false, priceFormat });
      this.#main.setData(bars);
    } else if (this.#type === TYPE_BARS) {
      this.#main = chart.addSeries(BarSeries, { upColor: c.up, downColor: c.down, priceFormat });
      this.#main.setData(bars);
    } else if (this.#type === TYPE_LINE) {
      this.#main = chart.addSeries(LineSeries, { color: trend, lineWidth: 2, priceFormat });
      this.#main.setData(bars.map((b) => ({ time: b.time, value: b.close, ...(b.ext ? { color: c.muted } : {}) })));
    } else {
      // The overview look: a filled line in the day's color, greyed outside regular trading hours.
      const ext = { lineColor: c.muted, topColor: alpha(c.muted, AREA_EXT_ALPHA), bottomColor: alpha(c.muted, 0) };
      this.#main = chart.addSeries(AreaSeries, { lineColor: trend, topColor: alpha(trend, AREA_TOP_ALPHA), bottomColor: alpha(trend, 0), lineWidth: 2, priceFormat });
      this.#main.setData(bars.map((b) => ({ time: b.time, value: b.close, ...(b.ext ? ext : {}) })));
    }
    const volumeOn = this.#studies.has('volume');
    this.#main.priceScale().applyOptions({ scaleMargins: { top: PRICE_TOP, bottom: volumeOn ? PRICE_BOTTOM_WITH_VOLUME : PRICE_BOTTOM } });
    if (this.#showBaseline && this.#previousClose > 0) {
      this.#main.createPriceLine({ price: this.#previousClose, color: c.muted, lineStyle: LineStyle.Dotted, lineWidth: 1, axisLabelVisible: true, title: '' });
    }

    /** Adds one study line (or histogram) and remembers its values for the legend. */
    const add = (study, label, values, color, pane = 0, extra = {}) => {
      const series = chart.addSeries(LineSeries, { color, lineWidth: 1, priceFormat, ...quiet, ...extra }, pane);
      series.setData(bars.flatMap((b, i) => (values[i] === null ? [] : [{ time: b.time, value: values[i] }])));
      this.#lines.push({ study, label, color, values, series });
      return series;
    };
    const tokenOf = (id) => token(STUDIES.find((s) => s.id === id)?.token ?? TOKENS.muted);
    const barsOf = (id) => STUDIES.find((s) => s.id === id)?.bars ?? 0;

    if (volumeOn) {
      const series = chart.addSeries(HistogramSeries, { priceFormat: { type: 'volume' }, priceScaleId: VOLUME_SCALE, ...quiet });
      chart.priceScale(VOLUME_SCALE).applyOptions({ scaleMargins: { top: VOLUME_TOP, bottom: 0 } });
      series.setData(bars.map((b) => ({ time: b.time, value: b.volume, color: alpha(b.close >= b.open ? c.up : c.down, VOLUME_ALPHA) })));
      this.#lines.push({ study: 'volume', label: '', color: c.muted, values: bars.map((b) => b.volume), series, histogram: true });
    }
    for (const id of ['sma20', 'sma50']) {
      if (this.#studies.has(id)) add(id, String(barsOf(id)), sma(closes, barsOf(id)), tokenOf(id));
    }
    if (this.#studies.has('ema200')) add('ema200', String(barsOf('ema200')), ema(closes, barsOf('ema200')), tokenOf('ema200'));
    if (this.#studies.has('bollinger')) {
      const band = bollinger(closes, barsOf('bollinger'), BOLLINGER_WIDTH);
      const color = tokenOf('bollinger');
      add('bollinger', 'U', band.upper, alpha(color, BAND_FILL_ALPHA));
      add('bollinger', 'M', band.mid, color, 0, { lineStyle: LineStyle.Dashed });
      add('bollinger', 'L', band.lower, alpha(color, BAND_FILL_ALPHA));
    }
    if (this.#studies.has('vwap')) add('vwap', '', vwap(bars), tokenOf('vwap'));

    let pane = 0;
    if (this.#studies.has('rsi')) {
      const format = { type: 'price', precision: 1, minMove: 0.1 };
      const series = add('rsi', String(barsOf('rsi')), rsi(closes, barsOf('rsi')), tokenOf('rsi'), ++pane, { priceFormat: format, lastValueVisible: true });
      for (const price of [RSI_HIGH, RSI_LOW]) series.createPriceLine({ price, color: c.muted, lineStyle: LineStyle.Dotted, lineWidth: 1, axisLabelVisible: false, title: '' });
    }
    if (this.#studies.has('macd')) {
      const m = macd(closes, MACD_FAST, MACD_SLOW, MACD_SIGNAL);
      pane += 1;
      const gap = chart.addSeries(HistogramSeries, { priceFormat, ...quiet }, pane);
      gap.setData(bars.flatMap((b, i) => (m.gap[i] === null ? [] : [{ time: b.time, value: m.gap[i], color: alpha(m.gap[i] >= 0 ? c.up : c.down, GAP_ALPHA) }])));
      this.#lines.push({ study: 'macd', label: 'H', color: c.muted, values: m.gap, series: gap, histogram: true });
      add('macd', 'M', m.line, tokenOf('macd'), pane);
      add('macd', 'S', m.signal, token('--color-kind-macro'), pane);
    }
    chart.panes().forEach((p, i) => p.setStretchFactor(i ? STUDY_STRETCH : MAIN_STRETCH));

    for (const d of this.#drawings) {
      if (d.kind !== TOOL_LEVEL) continue;
      this.#levels.push(this.#main.createPriceLine({ price: d.p, color: c.draw, lineStyle: LineStyle.Solid, lineWidth: 1, axisLabelVisible: true, title: '' }));
    }
    this.#main.attachPrimitive(this.#overlay);
  }

  /** Index of the bar at or just before a real Unix time. */
  #indexOf(/** @type {number} */ t) {
    let lo = 0;
    let hi = this.bars.length - 1;
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1;
      if (this.bars[mid].t <= t) lo = mid;
      else hi = mid - 1;
    }
    return lo;
  }

  /** Draws the trend lines, and the one being placed, in device pixels. */
  #paint = (/** @type {{context: CanvasRenderingContext2D, horizontalPixelRatio: number, verticalPixelRatio: number}} */ scope) => {
    if (!this.#main || !this.bars.length) return;
    const { context: ctx, horizontalPixelRatio: hr, verticalPixelRatio: vr } = scope;
    const scale = this.#chart.timeScale();
    const at = (/** @type {{t:number, p:number}} */ pt) => {
      const x = scale.logicalToCoordinate(/** @type {any} */ (this.#indexOf(pt.t)));
      const y = this.#main.priceToCoordinate(pt.p);
      return x === null || y === null ? null : [x * hr, y * vr];
    };
    const segments = this.#drawings.flatMap((d) => (d.kind === TOOL_TREND ? [[d.a, d.b]] : []));
    if (this.#anchor && this.#preview) segments.push([this.#anchor, this.#preview]);
    ctx.strokeStyle = this.#colors.draw;
    ctx.fillStyle = this.#colors.draw;
    ctx.lineWidth = TREND_WIDTH * hr;
    for (const [a, b] of segments) {
      const from = at(a);
      const to = at(b);
      if (!from || !to) continue;
      ctx.beginPath();
      ctx.moveTo(from[0], from[1]);
      ctx.lineTo(to[0], to[1]);
      ctx.stroke();
      for (const [x, y] of [from, to]) {
        ctx.beginPath();
        ctx.arc(x, y, ANCHOR_RADIUS * hr, 0, FULL_TURN);
        ctx.fill();
      }
    }
  };

  /** The bar and price under a pointer event on the price pane, or null anywhere else. */
  #point(/** @type {any} */ param) {
    if (!this.#main || !param.point || param.logical == null || (param.paneIndex ?? 0) !== 0) return null;
    const p = this.#main.coordinateToPrice(param.point.y);
    if (p === null) return null;
    const i = Math.min(this.bars.length - 1, Math.max(0, Math.round(param.logical)));
    return { t: this.bars[i].t, p };
  }

  #onClick = (/** @type {any} */ param) => {
    const pt = this.#tool ? this.#point(param) : null;
    if (!pt) return;
    if (this.#tool === TOOL_TREND && !this.#anchor) {
      this.#anchor = pt;
      return;
    }
    const drawing = this.#tool === TOOL_LEVEL ? { kind: TOOL_LEVEL, p: pt.p } : { kind: TOOL_TREND, a: this.#anchor, b: pt };
    this.#drawings = [...this.#drawings, /** @type {Drawing} */ (drawing)];
    this.setTool('');
    this.#build();
    this.#events.onTool('');
    this.#events.onDrawings(this.#drawings);
  };

  #onMove = (/** @type {any} */ param) => {
    if (this.#anchor) {
      this.#preview = this.#point(param) ?? this.#preview;
      this.#redraw();
    }
    const hovered = param.logical == null || !param.point ? this.bars.length - 1 : Math.round(param.logical);
    this.#legend(Math.min(this.bars.length - 1, Math.max(0, hovered)));
  };

  /** Reports the bar under the crosshair (or the newest one) with every study's value there. */
  #legend(/** @type {number} */ i) {
    const bar = this.bars[i];
    if (!bar) {
      this.#events.onLegend(null);
      return;
    }
    const before = i > 0 ? this.bars[i - 1].close : this.#previousClose || bar.open;
    this.#events.onLegend({
      bar,
      change: bar.close - before,
      changePct: before ? ((bar.close - before) / before) * PERCENT : 0,
      studies: this.#lines.filter((l) => !l.histogram || l.study === 'macd').map((l) => ({ study: l.study, label: l.label, color: l.color, value: l.values[i] })),
    });
  }
}
