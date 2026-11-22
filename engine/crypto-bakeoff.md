# Crypto rule bake-off

This protocol was fixed before the historical replay. The rulebook key is
`crypto.strategy`. An omitted key or `breakout` uses the existing rule.

- `breakout` requires an hourly close above its SMA20 and the previous 12-hour
  high, SMA20 above SMA50, at least 1.5 times the preceding 20-hour average
  volume, RSI(14) at most 75 and a close at most 8% above SMA20.
- `rsi-reversion` enters when hourly RSI(14) crosses from at or below 30 to
  above 30, with the hourly SMA20 above SMA50. Larger current RSI ranks first.
- `four-hour-trend` enters on a 6/24 SMA bullish crossover of completed
  UTC-aligned four-hour closes, evaluated only when a four-hour bar finishes.
  It requires 25 complete consecutive bars. Larger fast/slow SMA ratio ranks first.
- `weekend-fade` enters at Sunday 00:00 UTC after Saturday's close has fallen
  at least 3% from Friday's close. It requires 25 consecutive hourly closes.
  Larger declines rank first.

All entries share the existing default universe and risk settings. The 12 USD
pairs are BTC, ETH, SOL, XRP, DOGE, ADA, AVAX, LINK, LTC, BCH, DOT and SUI.
Altcoins require BTC above its hourly SMA50. The stop is two hourly ATR(14)
from entry, clamped to 1.5–6%. The ladder exits in thirds at 1.5R, 3R and 5R
with the default fixed stop. The time stop is 72 hours, with an early exit
after 12 hours if the open would fail to recover the entry cost after fees.
Position sizing starts with $1,000 cash and risks 1% of equity at the stop,
capped at 25% of equity per position with a $10 minimum and half sizing on
days the US stock calendar is closed. Limits are three open positions, eight
entries per New York day, a 12-hour cooldown after a loss in the same coin and
a 3% daily loss gate. The market entry chase ceiling is 1%.

Market orders enter at the next hourly open. The simulated resting buy limit
is 0.10% below the signal close and expires after that next hour. It fills
at the limit only when the open is above it and the low strictly below it.
The fill hour can stop out but cannot take profit. All exits use the taker fee,
including target exits. Tier 1 fees are 0.15% maker and 0.25% taker.

For each entry mode and rule, run trailing 90 and 365 day windows ending at
the same completed UTC hour. Replay each full window and the first two thirds
and final third separately, with fresh cash and a floor to a whole hour at
the split. Use up to 120 warm-up candles. Close any remaining positions at
the final available close in the segment, with fees. Drawdown uses hourly
closing equity marked at net liquidation value and includes starting equity.
R uses planned initial stop risk after fees. A rule and entry-mode pair must
have strictly positive net P&L in both chronological segments and both full
windows. No parameters are adjusted after seeing the results.

Fee source: https://docs.alpaca.markets/us/docs/crypto-fees

Candle source: https://docs.cdp.coinbase.com/api-reference/exchange-api/rest-api/products/get-product-candles

## Replay on 2026-11-22

From `engine/` in PowerShell:

```powershell
$replayDir = Join-Path ([IO.Path]::GetTempPath()) ("m24-crypto-" + [guid]::NewGuid().ToString("N"))
go run -buildvcs=false ./cmd/m24d --data $replayDir --crypto-backtest 365 --crypto-backtest-all --crypto-backtest-entry both --crypto-backtest-end 2026-11-22T04:00:00Z
```

The 90-day window started 2026-08-24 04:00 UTC and split at 2026-10-23
04:00 UTC. The 365-day window started 2025-11-22 04:00 UTC and split at
2026-07-23 12:00 UTC. Both ended at 2026-11-22 04:00 UTC. Coinbase returned
8,875 of 8,880 expected hours per pair, including the first window's warm-up.
The downloader requests at most 300 hourly candles at a time, filters to the
requested interval, and deduplicates the shared page boundary.

No strategy and entry mode passed. All eight combinations lost money in both
full windows. The 365-day market runs of breakout and four-hour trend lost
37.60% and 40.25%, respectively. RSI reversion had 19–25 trades over 365 days
and fell 4.40–4.54%. Weekend fade gained 0.63–0.79% in the first two thirds
of the 365-day window, then lost 1.39–1.44% in the final third. Weekend fade
made only one trade in the full 90-day window.

The 90-day window overlaps the 365-day window. The replay uses Coinbase
trades as a proxy for Alpaca fills and models neither spread, slippage beyond
opening gaps, nor queue priority. The live paper desk runs its usual play
checks, including liquidity and spread, and uses its existing execution and
fee assumptions. The entry-mode flag applies only to the offline replay.
