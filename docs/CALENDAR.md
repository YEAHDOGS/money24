# The trading calendar

Status: built (Nov 21, 2026). Engine: `engine/calendar/`. Wording: `engine/calendar/strings.json`.

Every day gets a label: which markets are open, what kind of day it is, which stretch of the year
it falls in, and how Money 24 treats a day like that. The engine works the label out from the date
alone, in New York time, by the exchanges' own rules. Nothing is looked up and no list of years
needs updating.

```
the date → session (open, half day, closed) → each market's state → tags → the day's playbook
```

## 1. Why a calendar

The same rule behaves differently on different days. A stop that holds on a busy Tuesday gets
jumped on a thin half day. A put bought on Friday loses two days of time value before anyone can
sell it. Money from a stock sold on Friday cannot buy anything until Monday. A coin bought on a
Saturday is trading on a thinner book than the same coin on a Wednesday.

A person learns these by losing money to them. A machine that runs with nobody watching has to be
told. The calendar is where it is told.

Two kinds of label, kept apart on purpose:

| Kind | Where it comes from | How far to trust it |
|---|---|---|
| **Structural** | The exchange calendar: sessions, holidays, half days, expiries, month and quarter ends | Exact. These are facts about the day |
| **Seasonal** | Patterns people have measured in past prices: earnings season, September, the Santa Claus rally | Weak. Tendencies, shown as context, **never traded on** |

Scheduled events (Fed decisions, CPI, jobs reports, each company's earnings date) are a third
thing and are not in this package: they change, so they come from the news service's calendar
(`newsd`, `GET /api/calendar`).

## 2. The week, day by day

| Day | What is open | How Money 24 treats it |
|---|---|---|
| **Sunday evening** | Forex opens 5 PM New York, CME futures 6 PM. Stocks and options are still shut | The first prices of the week. They show where Monday's open is leaning, on thin volume |
| **Monday** | Everything | The open prices in the whole weekend. Wait out the first 15 minutes. A gap is not a signal; volume after the first half hour is |
| **Tuesday to Thursday** | Everything | A normal session. The rulebook as written |
| **Friday** | Everything. Weekly options expire at the close; futures and forex close 5 PM | Nothing is carried into the weekend on hope: it has its stop and its time exit, or it is closed. What is sold today is Monday's money |
| **Saturday** | Coins and prediction markets only | Half size: books are thin. Use the quiet to read the week ahead |
| **Sunday** | Coins and prediction markets, then forex and futures in the evening | The same as Saturday until the evening |

The regular US stock and option session is 9:30 AM to 4 PM New York. The rulebook already keeps
plays out of the first 15 minutes and the last 10 (`opening_wait`, docs/PLAYS.md).

## 3. The month

| Day | What happens | How Money 24 treats it |
|---|---|---|
| **First session** | Monthly contributions arrive. The first session of a quarter also undoes last quarter's window dressing | Let volume confirm a move before following it |
| **VIX expiration** (a Wednesday, 30 days before the next month's third Friday) | VIX futures and options settle at the open | Do not read the first hour's volatility as news |
| **Monthly options expiry** (the third Friday; the Thursday before when that Friday is a holiday) | The month's standard options expire. Prices pin and unpin around strikes with a lot of open contracts | No new option plays that expire today. A play expiring today leaves on its time exit, never at the bell |
| **Triple witching** (the monthly expiry of March, June, September and December) | Stock options, index options and index futures expire together | The volume is real, but it says nothing about direction |
| **Last session** (and quarter end, year end) | Funds rebalance and dress their books into the close | A late move can be flows, not news |

In 2026 the June expiry is a Thursday (June 18): the third Friday is Juneteenth.

## 4. The year, by season

These are **tendencies**: averages over past decades, with plenty of years that went the other
way. Most are well documented and most have weakened since they were published. The engine shows
them as tags so a person reading the day knows the regime. No rule reads them, no play is sized
by them, and none of them is a forecast.

| When | Tag | The tendency | The catch |
|---|---|---|---|
| January | January effect | Small stocks sold for tax losses in December have often bounced | Weaker in recent decades |
| Mid-January, mid-April, mid-July, mid-October, about four weeks each | Earnings season | Most large companies report. Stocks gap on results; option prices deflate once a report is out (`event_crush`) | The dates here are approximate. Each company's own date is on the news calendar |
| May to October | "Sell in May" | The weaker half of the year historically | Weaker is not the same as down, and the effect has faded |
| August | Summer doldrums | The lightest volume of the year | Thin markets drift, then jump |
| September | September | Historically the weakest month on average | Plenty of Septembers rise |
| November and December | Tax-loss selling | The year's losers see extra selling as holders book losses | Everyone knows, so some of it is priced early |
| Thanksgiving week | Thanksgiving week | Thursday closed, Friday a half day, light volume all week | A move on a thin tape means less |
| Last 5 sessions of the year, first 2 of the next | Santa Claus rally | Up more often than not | Seven days, a small sample |
| Every year | Presidential cycle year | Midterm years (2026) have been the choppiest of the four, pre-election years (2027) the strongest | A few dozen cycles of data at most |
| November of a midterm year to the next June | Post-midterm window | A favorable stretch for the S&P 500 historically. November 2026 to June 2027 is one | Stocks drift up in most eight-month windows. It is context for risk posture, not a signal and not a trade (notes/strategy-evals.md) |

## 5. The cash account's clock

Money 24 is designed around a **cash account**: it only spends money it has, it never borrows, and
it never sells what it does not own. That makes settlement part of every plan.

| What was sold | When the money can be spent again |
|---|---|
| Stocks and ETFs | The next trading day (T+1) |
| Options | The next trading day (T+1) |
| Coins | At once |

- **Friday proceeds are Monday's money.** A stock or option sold on Friday settles at Monday's
  open, or Tuesday's when Monday is a holiday. Over Thanksgiving, Wednesday's sale settles Friday.
- **Buying with unsettled money and selling again before it settles** is a good-faith violation,
  and brokers restrict accounts that repeat it. The engine avoids the question: unsettled proceeds
  are not buying power.
- **So capital turns over once a day** in stocks and options. A cash account is not bound by the
  pattern-day-trader count, but each dollar can make one round trip per session. Plays are sized
  with that in mind: money spent in the morning is not coming back in the afternoon.
- **Coins are the exception.** They settle the moment they sell, any hour of any day, which is
  why the weekend desk trades them.

`calendar.NextSettlement(sold)` is that clock: 9:30 AM New York on the next trading day after the
sale, weekends and exchange holidays skipped.

## 6. What each market does on a closed day

| Market | Weekend | US market holiday | Half day (1 PM close) |
|---|---|---|---|
| **Stocks** | Closed | Closed | Close 1 PM New York |
| **Options** | Closed. Time value still drains | Closed. Time value still drains | Close 1 PM; index options 1:15 |
| **Futures** (CME) | Closed Saturday; reopen Sunday 6 PM New York | Shortened or paused: check the exchange | Shortened: check the exchange |
| **Forex** | Closed Saturday; reopens Sunday 5 PM New York | Trades, thin, wide spreads | Trades, thin |
| **Coins** | Open, on thinner books | Open, on thinner books | Open |
| **Prediction markets** | Open | Open | Open |

Prediction markets (Kalshi, Polymarket) never close, but each contract settles on its own event's
clock: a contract on Friday's jobs report is decided Friday morning, whatever day it was bought.

The full-day holidays, by rule: New Year's Day, Martin Luther King Jr. Day, Washington's Birthday,
Good Friday, Memorial Day, Juneteenth, Independence Day, Labor Day, Thanksgiving and Christmas. A
holiday on a Sunday is kept on the Monday. One on a Saturday is kept on the Friday before, except
New Year's Day, which is then not kept at all. Half days: the day after Thanksgiving, Christmas
Eve, and July 3 when July 4 is a weekday.

| | 2026 | 2027 |
|---|---|---|
| Closed | Jan 1, Jan 19, Feb 16, Apr 3, May 25, Jun 19, Jul 3, Sep 7, Nov 26, Dec 25 | Jan 1, Jan 18, Feb 15, Mar 26, May 31, Jun 18, Jul 5, Sep 6, Nov 25, Dec 24 |
| Closes 1 PM | Nov 27, Dec 24 | Nov 26 |

Checked against the NYSE Group holiday and early-closings calendar for 2026 to 2028.

## 7. How the engine uses it

| Piece | What it does |
|---|---|
| `calendar.On(t)` | The label for the day `t` falls on: session, each market's state and note, tags, playbook lines, the next open |
| `calendar.Ahead(t, n)` | That day and the ones after it, for a week-ahead strip |
| `calendar.StocksOpen(t)` | Whether the regular session is open right now, holidays and half days included. The stock finder only looks inside a real session |
| `calendar.NextSettlement(t)` | The cash account's T+1 clock for stock and option sales |
| `GET /api/day?days=N` | The same labels over HTTP for the app: today and up to 31 days, as `{"now", "stocks_open", "days"}` |
| The crypto desk | Reads the day's crypto state: `thin` (weekends and US holidays) is the cue for half size |

Each `Day` has a `label` (one short line for a header chip), `tags` (every label that applies, the
most important first, each with a one-line note) and a `playbook` (one to four lines: how Money 24
treats this kind of day). Every word is in `strings.json`, beside the code, so the philosophy can
be edited without touching the logic.

Today, Saturday November 21, 2026, reads: *Weekend: crypto only.* Stocks and options open Monday
9:30 AM New York. Thursday is Thanksgiving (closed), Friday is a half day.

## 8. Honest limits

- Seasonality is weak evidence. A tag says what has tended to happen, on small samples, in a
  market that has since read the same almanac. It is never a reason to take a play.
- Holidays follow today's exchange rules. The exchanges can close for a day of mourning or a
  storm, or add a holiday, and nothing here will know until the rules are changed.
- Futures and forex holiday hours differ by product and are not modelled: the label says to check
  the exchange instead of inventing hours.
- Settlement skips exchange holidays only. On the two bank holidays when stocks still trade
  (Columbus Day and Veterans Day) settlement can take a day longer than the clock here says.
- Earnings season dates are a rule of thumb, and the day's label knows nothing about the Fed, CPI
  or any single company. Those come from the news calendar.

*A calendar tells you what kind of day it is, not which way prices will go. Trading involves
substantial risk of loss; this is market-structure information, not financial advice.*

## Holidays on the news calendar (Nov 23, 2026)

The news calendar (`newsd`: Forex Factory, Nasdaq earnings, Treasury auctions) has no market closures, so the News page adds them itself: `src/feed/market-days.js` works out every US stock market holiday and half day (the day after Thanksgiving, Christmas Eve, July 3) for the year before to two years ahead, by the same rules as `engine/calendar/sessions.go`. Keep the two in step. They show under Holidays as all-day events, closures at high impact and half days at medium.
