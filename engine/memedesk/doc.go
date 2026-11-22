// Package memedesk evaluates Pump.fun observations and simulates USD fills.
// The runner feeds it newly written recorder rows in receipt order. A local,
// append-only journal stores the observations, rulebook changes and clock
// ticks needed to reconstruct decisions, fills and price paths. The status
// endpoint exposes totals, open positions and recent candidates and exits.
// Edit meme-strategies.json in the runner data folder or use the protected
// PUT /api/memedesk/strategies endpoint to update the rulebook.
//
// Each strategy sees every observed launch and graduation. Candidate paths
// follow available USD quotes for up to 48 hours. Decisions use only facts
// available when the entry quote arrives. Organic graduates require buyer,
// block and creator evidence, which the current recorder leaves unknown.
// The shadow and fade benchmarks use the same model and exit rules without
// drawing from the desk's capital. Unavailable and unfinished benchmarks
// have their own totals. Outcomes by refusal reason can overlap when several
// checks fail.
//
// The model uses half the reported pool liquidity as its quote reserve,
// constant-product size impact, 0.5% additional adverse movement, and a 3%
// entry slippage limit. It charges 1.25% pump.fun and 0.5% PumpPortal on each
// fill. Signals wait at least three seconds for an eligible observation.
// The shadow book first looks for a quote after 15 seconds to give a scheduled
// RugCheck lookup time to arrive. Entry windows are five minutes. Empty
// successful lookups and explicitly zero liquidity write down remaining units
// to zero. Missing price and unknown liquidity leave exits waiting for quotes.
// Pool reserves and extra adverse movement are modeled estimates. Polling
// cadence limits the prices that can be observed.
//
// m24d --meme-replay YYYY-MM-DD uses the current rulebook on one UTC day's
// candidates. It reads the previous two days for context and the following
// two days for outcomes. With partial coverage, the replay retains pending
// decisions and open positions as of the final recorded observation.
package memedesk
