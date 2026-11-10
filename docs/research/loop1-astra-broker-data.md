# Loop 1 — Broker + market data

Research date: **2026-10-30**. Scope: planning for Money 24 by DOGS. Official documentation and company-issued releases checked on the live web. No authenticated account, paid entitlement, order submission, or latency benchmark was available. **Documented support is not an execution test. UNVERIFIED means a claim needs further evidence before implementation or product promises.** Prices are USD unless specified; personal-use prices do not imply SaaS redistribution rights.

## Executive findings

- Webull is a viable primary integration candidate: current US documentation covers equities/ETFs, options including multi-leg strategies, futures, spot crypto and event contracts. An actual API sandbox and OAuth partner integration exist. Old claims that Webull has no official API, no OAuth or no API paper trading are obsolete. [API overview](https://developer.webull.com/apis/docs/)
- “Log in with Webull” requires an approved **Connect API** application, not merely a retail API key. Commercial approval and sandbox acceptance are launch dependencies. [Connect](https://developer.webull.com/apis/docs/connect-api/about-connect-api/)
- Free infrastructure does not mean free consolidated real-time data or redistribution. Webull’s app subscriptions and API subscriptions are separate. [Data permissions](https://developer.webull.com/apis/docs/market-data-api/overview/)
- Webull can remain the authoritative brokerage ledger, but cannot replace the application's durable execution journal, strategy state and audit trail. Its history is more useful than older documentation suggests, but differs by API product.
- Build a small capability-driven broker adapter and separate data adapter now. Start with personal paper trading; defer discretionary trading for paying customers until legal and broker approvals are settled.

## 1. Webull tradability and regions

### US: documented API capability

US stocks and ETFs are supported, with fractional quantities, short selling subject to eligibility, extended sessions and a distinct overnight session. The stock API has `CORE`, `ALL` and `NIGHT`; `ALL` means premarket/regular/after-hours, not an instruction to include the overnight session. Eligible instruments and order types still matter. Do not label every stock as continuously tradable. [Stock trading](https://developer.webull.com/apis/docs/trade-api/stock/)

Options support single-leg and multi-leg strategies; futures cover several commodity and financial categories; crypto means spot crypto; event contracts are binary outcomes. Equities support OTO/OCO/OTOCO, whereas options strategies and futures have different capabilities. Crypto is documented as 24/7, futures as near 24/6; neither implies US equity markets trade weekends. [Product and feature matrix](https://developer.webull.com/apis/docs/trade-api/overview/)

The API product page explicitly advertises 24/5. Treat this as availability for eligible instruments/sessions, not continuous liquidity or a fill guarantee. Fractional overnight compatibility, every supported option strategy, and every event venue require instrument/account-specific validation. [US OpenAPI](https://www.webull.com/open-api)

There is a documentation conflict: the overview marks options as supporting market/best-price execution, while the Trading FAQ explicitly disallows `MARKET` options orders. Until resolved, use documented limit orders and validate against the product-specific contract. Futures do not permit physical delivery: expiry/roll risk must be handled. [Trading FAQ](https://developer.webull.com/apis/docs/trade-api/faq/)

### Regional findings — broker entity is not the market

- **US:** the US developer portal describes US-market access, not a universal global-market account. Foreign-company ADRs/US-listed ETFs are not direct access to their home exchanges. Global direct trading from a US Webull API account is **UNVERIFIED**. [US portal](https://developer.webull.com/apis/docs/)
- **HK:** regional docs support US equities/options, HK stocks and mainland A-shares through relevant arrangements. The about page excludes US index options. Separate futures documentation supports US and HK futures. Do not infer crypto/event contracts. [HK overview](https://developer.webull.hk/apis/docs/about-open-api/), [HK futures](https://developer.webull.hk/apis/docs/trade-api/futures/), [HK data](https://developer.webull.hk/apis/docs/market-data-api/overview/)
- **JP:** the official SDK skill matrix lists US/JP stock trading, but not option/futures/crypto/event trading. Option *data* availability is not trading permission. There is a regional approval flow and regional service host. [Official regional matrix](https://github.com/webull-inc/webull-openapi-skills/blob/main/SKILL.md), [JP application](https://developer.webull.co.jp/apis/docs/authentication/individual-application/), [JP data service](https://developer.webull.co.jp/apis/docs/market-data-api/data-api/)
- **SG:** regional developer portal says US-market trading; official integration matrix specifies US stocks/ETFs and no derivative/crypto/event order support. SG stocks and US options shown in the consumer app do not establish API availability. [SG portal](https://developer.webull.com.sg/apis/docs/), [SG consumer product list](https://www.webull.com.sg/)
- **UK:** API landing page lists US stocks and options and a minimum account size of **£1,000**. The official SDK skill matrix marks UK options unsupported. **Conflict: US stocks are established; API options are UNVERIFIED pending regional confirmation.** No evidence here of LSE trading. [UK API page](https://www.webull-uk.com/open-api), [UK portal](https://developer.webull-uk.com/apis/docs/)
- **AU:** dedicated API page explicitly lists **US stocks and ETFs**, with expanded offerings under development. AU/HK shares, options and crypto in website navigation are not proof of API access. ASX orders via API are **UNVERIFIED**. [AU API page](https://www.webull.com.au/open-api)
- **CA:** Canadian official agentic page advertises OAuth-connected agents and US-stock order instructions. This establishes a consumer integration offering, not a documented self-service Canadian developer API or Money 24 partner approval. A Canadian `/open-api` page could not be retrieved; direct Canadian OpenAPI onboarding, TSX orders, derivatives, fractional and overnight support remain **UNVERIFIED**. [Canadian agentic page](https://www.webull.ca/fr/agentic)
- **TH, MY, MX, BR, ZA:** the official integration matrix lists US stock trading, with no options, futures, crypto or events. Treat these as supported SDK capabilities subject to local onboarding, not evidence of local-exchange trading. [Official regional matrix](https://github.com/webull-inc/webull-openapi-skills/blob/main/SKILL.md)

The same matrix documents fractional shares only in the US region; HK mainland access requires enablement. Overnight and fractional support elsewhere are **UNVERIFIED**, not automatically absent. A region selector cannot bypass residency, account eligibility, agreements or exchange permissions. Money 24 should store broker entity, account region, instrument market and currency separately.

**Global markets answer:** some regional Webull APIs reach non-US markets, notably HK/CN and JP; one US account does not inherit all of them. For broad international coverage, investigate IBKR rather than promise global Webull access.

## 2. Authentication and third-party authorization

**Personal Trading API:** application-level App Key/App Secret sign requests. If 2FA is enabled, a reusable token verified in the Webull app is also needed. The secret is used for signing, not sent as a header. Follow the exact endpoint/version signing rules rather than copying an old SDK signature algorithm. Keep secrets in a server/local secure process, never Vite variables or native bundles. [Authentication](https://developer.webull.com/apis/docs/authentication/overview/)

US retail help says an active brokerage account and at least **$100 net account value** are required for API eligibility, with risk/trading-history review and typically 1–2 business days for approval. Sandbox application documentation separately describes automatic approval within minutes; whether the $100 requirement applies to a sandbox-only applicant is **UNVERIFIED**. [Eligibility FAQ](https://www.webull.com/help/faq/10512), [Application procedures](https://developer.webull.com/apis/docs/authentication/IndividualApplicationAPI/)

**Connect API:** real OAuth 2.0 redirect, authorization code, server-side token exchange and delegated account/trade access. Submit company name and callback URL; Webull business staff review the application, issue Client ID/Secret, then approve sandbox testing before production. Supported asset permissions are application-specific. The documented aggregate limit is **less than 90 requests/minute per authorized user**. This differs from individual Trading API endpoint quotas. Public partner price, minimum company scale and acceptance criteria are **UNVERIFIED**. [Connect onboarding and limits](https://developer.webull.com/apis/docs/connect-api/about-connect-api/)

**Institutional Trading API:** proprietary institutional accounts and authorized traders, rather than consumer OAuth linking. Docs warn that authorized traders' keys can access all portal accounts; finer permission alignment is forthcoming. **Broker API:** enterprise account opening, funding and brokerage infrastructure. It is a separate commercial/compliance integration, not a free substitute for Connect. [Institutional application](https://developer.webull.com/apis/docs/authentication/IndividualApplicationAPI/), [Broker introduction](https://developer.webull.com/apis/docs/broker-api/getting-started/)

Recommendation: personal credentials only for the owner's own initial installation. For SaaS, use Connect; do not ask customers to disclose their Webull password or treat shared retail keys as an approved commercial integration. OAuth authorization itself does not authorize Money 24 to operate as an investment adviser.

## 3. API paper trading

**Yes, it exists.** Webull's July 21, 2026 release announces API access to paperTrade, plus simulation across stocks, options, crypto, futures, bonds and events, rolling out later that month. Current individual application docs provide sandbox key creation and explicit production isolation. [Company-issued release](https://www.prnewswire.com/news-releases/webull-unveils-enhanced-paper-trading-experience-with-professional-grade-and-openapi-multi-asset-simulation-302830191.html), [Sandbox application](https://developer.webull.com/apis/docs/authentication/IndividualApplicationAPI/)

Distinguish the six-asset *paperTrade product announcement* from the five-asset Trading API documentation: a public bond-order API was not verified. Stocks/ETFs, options, futures, crypto and event-contract sandbox workflows are the documented target, but exact multi-leg, fractional, overnight, exercise/assignment, borrowing, margin and expiry simulation parity remains **UNVERIFIED** until acceptance tests pass. Do not claim a realistic queue/market-impact simulator.

Trading sandbox uses `api.sandbox.webull.com`; market stream TCP uses `data-api.sandbox.webull.com:1883`; trade events use `events-api.sandbox.webull.com`. Paid-data categories are **15 minutes delayed by default**; production real-time subscriptions also upgrade sandbox data. Sandbox capital/reset rules, retention guarantees and daily order-count caps were not established. [Environment changes](https://developer.webull.com/apis/docs/changelog/), [Sandbox data policy](https://developer.webull.com/apis/docs/market-data-api/overview/)

A paper strategy must consume the same temporal data as its simulated fills. Using current external prices to trade against delayed simulated fills can produce meaningless performance.

## 4. Throttles, streaming and Cloudflare

### US REST limits

The current central rate-limit reference states independent counters per endpoint, mostly per App Key; excess returns 429, and repeated excess can cause IP blocking. Production place/replace/cancel: **600/60s each**; batch placement **150/60s**; preview **150/10s**; history/open/detail and balance/positions **2/2s each**; account list **10/30s**. Corresponding sandbox trading endpoints generally **30/60s**. Production non-display quote endpoints **300/60s**, display endpoints commonly **60/60s**, sandbox **30/60s**. Authentication token create/check **10/10s**. Apply the exact endpoint's limit, not a global number copied from an older SDK. [Rate reference](https://developer.webull.com/apis/docs/rate-limits/)

Documentation conflict: some official integration examples still say 600/min market data and FAQ text calls subscribe/unsubscribe unlimited. The centralized reference has finite display-stream subscription quotas. Budget conservatively until the specific entitlement/path is verified. [Data FAQ](https://developer.webull.com/apis/docs/market-data-api/faq/)

### Market-data stream

MQTT 3.1.1 over TCP or WebSocket. Production WebSocket: `wss://data-api.webull.com:8883/mqtt`; production TCP: port 1883. Sandbox documents TCP, not a sandbox WebSocket URL. Use a unique session ID and matching HTTP subscription session; reusing the ID disconnects the old connection. Maximum **5 connections/App Key**, retained state about **1 minute** after disconnection, maximum **3 pushes/second/connection**. These are message delivery constraints, not evidence that only three underlying trades exist per second. Payloads may batch records. Numerical end-to-end latency/SLA and comprehensive option-stream parity are **UNVERIFIED**. [Streaming protocol](https://developer.webull.com/apis/docs/market-data-api/data-streaming-api/)

### Order/fill events

Trading events use persistent **gRPC server streaming**, including partial/full fills, placement failures, modification and cancellation results. Market MQTT is not the documented order-event channel. Event ordering, replay retention and exactly-once delivery guarantees were not established. Always reconcile after reconnect or uncertain submission. [Trade events and protocol](https://developer.webull.com/apis/docs/reference/custom/subscribe-trade-events/)

### Workers feasibility

- **HTTPS REST:** suitable for Workers. Signing must match Webull's precise contract.
- **MQTT over outbound WebSocket:** technically compatible; Workers and Durable Objects can act as WebSocket clients. An MQTT packet implementation/library and payload parser are still needed; opening a WebSocket alone is insufficient. Production port reachability needs a deployed proof of concept. [Cloudflare WebSockets](https://developers.cloudflare.com/durable-objects/best-practices/websockets/)
- **MQTT TCP:** `connect()` supports outbound TCP; Cloudflare explicitly mentions MQTT as a use case. Test DNS/IP restrictions and provider connectivity; outbound connections to Cloudflare IP ranges are blocked. Do not assume an arbitrary TCP feed is reachable. [TCP sockets](https://developers.cloudflare.com/workers/runtime-apis/tcp-sockets/)
- **Native gRPC:** not a drop-in fit. `node:http2` is a nonfunctional stub in Workers; enabling Node compatibility does not make standard gRPC clients work. Cloudflare proxy support for gRPC is not a Worker gRPC client API. A bespoke HTTP/2 implementation would be a separate, unvalidated engineering project. [Node compatibility](https://developers.cloudflare.com/workers/runtime-apis/nodejs/)
- **Practical choice:** REST order-status polling for the paper MVP; a local companion running the supported gRPC client if persistent event delivery is needed. A managed relay/container would change the strictly free/serverless constraint. Do not claim gRPC-Web compatibility without a Webull endpoint supporting it.
- **Persistence:** outbound WebSockets cannot use DO hibernation. Restarts/disconnections must be normal recovery paths, with durable sequence/reconciliation state. [Hibernation limitations](https://developers.cloudflare.com/durable-objects/best-practices/websockets/)

The DO free tier includes **13,000 GB-s/day** and **100,000 requests/day**; duration is metered while active/non-hibernatable. At the documented 128 MB allocation, one continuously active object is approximately **10,800 GB-s/day** (calculation: 0.125 × 86,400), potentially within duration allowance, but leaves little headroom. Thirty continuously active agent objects are not free. Message accounting, storage and other app work still consume quotas. Exceeding free limits fails operations rather than guaranteeing service. [DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/)

Use one shared authorized feed cache and one serialized execution coordinator per account; agents read snapshots rather than each opening streams or polling. Sharing data across users requires licensing permission. This is a proposed architecture, not a measured implementation.

### Webull data cost and rights

API access itself has no extra charge; trading fees still apply. Advanced subscriptions must be purchased specifically for OpenAPI. [API fees](https://developer.webull.com/apis/docs/faq/), [Subscription workflow](https://developer.webull.com/apis/docs/market-data-api/subscribe-quotes/)

The marketing page advertises free US Level 1 streaming. Detailed permissions call for Nasdaq Basic/TotalView non-display subscriptions, OPRA for options, individual CME-group exchanges or bundle for futures, and an overnight depth subscription. Crypto/event data require no additional subscription. **Exact current US OpenAPI prices, professional/nonprofessional classification and free-L1 eligibility: UNVERIFIED without the subscription console/quote.** Do not substitute mobile-app pricing or describe Nasdaq Basic as complete consolidated NBBO. [Marketing offer](https://www.webull.com/open-api), [Entitlements](https://developer.webull.com/apis/docs/market-data-api/overview/)

## 5. How far can “Webull is our database” go?

**Orders:** individual Trading API history defaults to seven days but explicitly permits full history as early as **2018-05-21**. The July 25, 2026 changelog says the two-year limit was removed. History/open-order lists can lag processing; query detail by `client_order_id` for freshest status. The Broker API's separate history page still says **two-year lookback**. Connect history retention and regional parity are **UNVERIFIED**. Do not generalize individual history retention to every API. [Individual history](https://developer.webull.com/apis/docs/reference/order-history/), [Change log](https://developer.webull.com/apis/docs/changelog/), [Broker history](https://developer.webull.com/apis/docs/reference/broker-fd-api/order-history/)

**Positions and balances:** current holdings/buying power are queryable. This is not evidence of arbitrary historical holdings snapshots. **Fills:** order details and fill events support reconciliation, but a guaranteed independently queryable lifetime execution archive, correction/bust coverage and per-fill retention were not established. **P&L:** arbitrary historical portfolio equity curves, tax-lot accounting and strategy-level realized P&L are **UNVERIFIED**; do not infer API access from app charts. [Positions](https://developer.webull.com/apis/docs/reference/account-position/), [Order detail](https://developer.webull.com/apis/docs/reference/order-detail/), [Fill events](https://developer.webull.com/apis/docs/reference/custom/subscribe-trade-events/)

**Recommended ownership boundary (design judgment):** Webull owns custody, execution truth, official balances/positions and broker records. Money 24 owns user authorization, risk limits, strategies, AI decisions, input timestamps, approvals, client-order IDs, submission attempts, responses, uncertain-order state, fill reconciliation, checkpoints and computed performance. Broker history cannot reconstruct a decision never submitted or explain why a bot issued an order.

Keep a minimal durable journal before submission, not a huge duplicate brokerage database. Reconcile broker state into it; archive older audit records compactly. A JSON payload inside a small D1/DO schema can reduce schema churn, but avoiding persistence entirely makes duplicate prevention and incident recovery unreliable. Browser storage alone cannot coordinate a server-side bot across devices. Do not use watchlists or order tags as a general application database.

## 6. Alternative/complementary data sources

### Alpaca

Basic **$0**: real-time equities from **IEX only**, 30 streaming equity symbols, indicative options, 200 historical calls/min; recent consolidated historical data is restricted by 15 minutes. Algo Trader Plus **$99/month**: consolidated US equities and OPRA options, unlimited equity subscriptions and 10,000 historical calls/min; options stream quotas differ. REST plus WebSocket fits the proposed stack. Best affordable paid consolidated-stock-and-option candidate here, subject to personal/commercial licensing. Free indicative options are not executable OPRA prices. [Official plan comparison](https://docs.alpaca.markets/us/v1.1/docs/about-market-data-api), [Feed distinctions](https://docs.alpaca.markets/us/docs/market-data-faq)

### Polygon / Massive

Current branding is Massive. Stocks: **$0** end-of-day/5 calls per minute; **$29/month** Starter and **$79/month** Developer are 15-minute delayed; **$199/month** Advanced supplies real-time trades/quotes. Options are a separate product with **$0/$29/$79/$199** tiers and real time at Advanced. Therefore personal real-time stocks plus options can total **$398/month**, before other products. Business stock pricing advertises **$2,499/month**, with exact redistribution terms to confirm. REST/WebSocket/flat files are useful, but the free tier is not live trading data. [Stocks](https://www.massive.com/stocks), [Options](https://www.massive.com/options)

### Databento

Strong candidate for exchange-native futures/depth and timestamped research rather than a zero-budget MVP. Historical data is usage-priced with **$125 introductory credits**, not an ongoing free live tier. CME Standard changed to **$199/month** on June 22, 2026; existing continuously subscribed $179 customers retain that rate for 12 months. Exchange licensing is additional/eligibility-dependent. Live transport uses raw TCP and binary encoding; HTTP handles historical access. Budget exchange fees and a client bridge/decoder before comparing against WebSocket products. [Pricing](https://databento.com/pricing), [2026 price change](https://databento.com/blog/updates-to-subscription-pricing)

### Tiingo / IEX

Tiingo Starter **$0**: 500 unique symbols/month, 50 requests/hour, 1,000/day, 1 GB/month. Personal Power **$30/month or $300/year**; internal commercial **$50/month or $499/year**. Published personal paid limits include 10,000/hour, 100,000/day and 40 GB/month. IEX/crypto feeds are listed; precise free streaming entitlements must be confirmed because the rendered comparison loses checkmark distinctions. Redistribution needs separate permission/pricing. IEX is a venue feed, not SIP. Do not confuse access to IEX exchange data through Tiingo/Alpaca with the former IEX Cloud service; current IEX Cloud availability/pricing was **UNVERIFIED** in this pass. [Tiingo pricing and usage rights](https://www.tiingo.com/about/pricing)

### Finnhub

Official homepage/pricing returned no extractable plan text in this research environment. **UNVERIFIED current free quotas, WebSocket symbol caps, paid prices, consolidated coverage and SaaS rights.** Frequently repeated figures such as 60 calls/minute or older $49.99 plans should not enter the budget as verified facts. It remains a candidate for supplementary fundamentals/news or quotes after an entitlement check, not the selected execution feed. [Official pricing](https://finnhub.io/pricing), [Official service](https://finnhub.io/)

### Direct crypto exchanges

- **Coinbase Advanced:** direct market WebSocket and authenticated user channel; good venue-specific live input. Public feed access is offered without a separate listed subscription price; trading fees are separate. Sandbox is static mocked account/order responses, **not a fill simulator**. [WebSocket docs](https://docs.cdp.coinbase.com/coinbase-app/advanced-trade-apis/websocket/websocket-overview), [Sandbox](https://docs.cdp.coinbase.com/coinbase-app/advanced-trade-apis/sandbox)
- **Kraken:** public WebSocket v2 book provides snapshots/deltas, selectable depth and checksum validation; no separate public-feed subscription price is listed in the reference. Suitable for a local paper simulator; enforce sequencing/checksums before treating the book as current. [Book protocol](https://docs.kraken.com/api/docs/websocket-v2/book/)
- **Binance.US:** US-specific REST, WebSocket API and market streams, including trades and depth; no separate market-stream fee listed. This is not Binance.com access or a way to access offshore derivatives. Verify state availability and instrument liquidity before using it as a trading venue. [US API documentation](https://docs.binance.us/)

“Free public feed” above means no separate fee advertised for the documented public connection, not unrestricted commercial redistribution or free trades. Exchange-local prices can diverge; a Coinbase price is not necessarily a fillable Webull quote.

### Prediction markets

**Kalshi:** REST plus WebSocket, with a demo using mock funds and separate credentials; demo prices/behavior need not match production. Current token-bucket documentation assigns Basic prediction budgets 200 read/100 write tokens per second; default endpoint cost is 10 tokens, corresponding to roughly 20 reads/10 ordinary writes per second, with exceptions. API access is not sold as a separate data subscription in these docs; trading fee schedules and commercial data rights remain separate. [Demo](https://docs.kalshi.com/getting_started/demo_env), [Limits](https://docs.kalshi.com/getting_started/rate_limits)

**Polymarket:** public market-data APIs and market/user WebSocket channels are documented; public-data use does not establish trading eligibility. Current international API geographic documentation places the US in **close-only on frontend and API**. Polymarket US must be assessed as a separate regulated product, not silently mapped to international CLOB credentials. Current commercial pricing, US-specific developer access and a realistic paper sandbox are **UNVERIFIED**. [API and geographic restrictions](https://docs.polymarket.com/api-reference/geoblock)

### CME futures costs are easy to misread

The official June 2026 schedule lists nonprofessional display top-of-book **$1.55/exchange/month or $4.65 bundle**, depth **$12.10/exchange or $36.50 bundle**. These are exchange display fees, **not an all-in bot API subscription**. It separately lists real-time feed fees and non-display principal-trading fees (Basic one application **$609/month per exchange**, subject to classification/exemptions). A retail qualifying package may differ; obtain vendor confirmation for automated/non-display usage. Do not automatically apply an enterprise category to every hobby user, or assume display pricing licenses SaaS automation. [CME fee schedule](https://www.cmegroup.com/market-data/files/june-2026-market-data-fee-list.pdf)

### “Fastest affordable” conclusion

No apples-to-apples benchmark was available; a universal fastest provider is **UNVERIFIED**. For free paper development, use a small IEX universe and direct crypto public streams. For consolidated equities/options, evaluate Alpaca's $99 plan. For serious futures depth, evaluate Databento plus licensing. Webull's 3-push/sec ceiling makes it a different delivery model from every-event feeds; it may suffice for second-level decisions but is not evidence of HFT suitability. Compare event-to-receive age and p50/p95/p99, reconnect gaps, completeness and stale-data behavior from the actual deployment region. An LLM's seconds-long decision path cannot exploit microsecond feed differences.

## 7. Backup brokers and adapter design

- **Alpaca:** preferred first backup for US equities/ETFs, options and crypto. Free real-time paper environment, paper-only signup and separate production credentials; options enabled in paper. REST/WebSocket integration is simpler for Workers than a desktop gateway. Commercial linking/embedded brokerage must use the appropriate partner product. [Trading/paper API](https://docs.alpaca.markets/us/docs/trading-api), [Options](https://docs.alpaca.markets/us/docs/options-trading)
- **IBKR:** strongest global/multi-asset candidate. Web API and TWS API support paper accounts, but simulated fills use top-of-book and some complex orders/combinations are limited. Retail Client Portal Gateway uses a local process; standard TWS integration also requires a host. Do not budget an always-on gateway as a free Worker. Approved direct OAuth routes are distinct. [Paper limitations](https://www.interactivebrokers.com/campus/glossary-terms/paper-trading-account/), [Gateway/API endpoints](https://www.interactivebrokers.com/docs/web-api/v1/endpoints/introduction)
- **Tradier:** US equities/ETFs/options including multileg; account includes API and sandbox. Sandbox fills use 15-minute-delayed data; no delayed sandbox streaming. Partner status and OAuth required for public apps. This documented brokerage API has no futures. [Overview](https://docs.tradier.com/), [FAQ](https://docs.tradier.com/docs/faq)
- **tastytrade:** useful options/futures alternative. OAuth is required; legacy sessions were retired in February 2026. Sandbox has synthetic fills and clears trades/transactions/positions every 24 hours; it tests workflows, not profitability. REST sandbox market-data routes do not function; some delayed streaming observations appear in newer docs, so verify separately. [OAuth](https://developer.tastytrade.com/docs/authentication/oauth2/), [Sandbox](https://developer.tastytrade.com/sandbox)
- **Schwab:** developer app approval and authorization framework exist. A generic portal “Sandbox App” status is **not proof of a Trader API paper-execution environment**. Public API paper trading, supported trading asset matrix and current production refresh-token operational requirements remain **UNVERIFIED** here. Do not equate thinkorswim paperMoney with API paper support. Lower priority until these are confirmed. [Official app approval guide](https://developer.schwab.com/user-guides/apis-and-apps/create-an-app)
- **Kalshi:** focused event-contract adapter with separate demo; demo market realism is limited. [Demo](https://docs.kalshi.com/getting_started/demo_env)
- **Coinbase Advanced:** focused crypto execution/data adapter, but static sandbox needs a separate simulator for strategy evaluation. [Sandbox](https://docs.cdp.coinbase.com/coinbase-app/advanced-trade-apis/sandbox)

**Yes, design adapters.** Keep a narrow common contract: discover capabilities/accounts/instruments; get positions/balance; preview/submit/cancel/replace; reconcile order/fills; receive events. Declare capabilities per entity, account, asset and environment. Preserve native IDs, decimals, multipliers, currency, session, expiry and provider error payloads. Do not force multi-leg options, futures and binary event contracts into a stock-only model. Separate quote providers from execution providers.

**No automatic broker failover for orders.** If a request times out, its fill state is unknown; submitting at another broker may double exposure. Reconcile first. An adapter enables deliberate migration and explicit account selection, not portable collateral or interchangeable fills. These are engineering recommendations.

## 8. US hobby/SaaS regulatory realities

### PDT status on this research date

FINRA's replacement of PDT provisions with intraday margin requirements became effective **June 4, 2026**, with firms permitted to transition through **October 20, 2027**. Migrated firms no longer use the trade-count PDT designation/$25,000 day-trading minimum; intraday margin still applies. Minimum equity for leveraged margin is $2,000 and firms may impose higher requirements. Firms still transitioning can retain the old regime. **Webull's specific account rollout was not verified.** Read account capabilities and get broker confirmation; do not hardcode “PDT abolished for everyone” or “$25k always required.” [FINRA adopted-rule explanation](https://syndication.finra.org/content/understanding-new-intraday-margin-requirements)

Legacy PDT generally meant four or more day trades in five business days and over 6% of margin-account trades, subject to broker designation. Cash accounts still require properly paid/settled funds; switching to cash does not remove settlement/free-riding constraints. [Legacy FINRA explanation](https://syndication.finra.org/content/am-i-pattern-day-trader), [Current cash/margin discussion](https://syndication.finra.org/content/frequent-intraday-trading-understanding-basics)

### Own money versus other people's accounts

Ordinary personal trading through a broker differs materially from operating a paid discretionary service. Automated advice remains advisory activity; calling it an AI tool or putting execution behind OAuth does not create an exemption. SEC versus state adviser registration, exemptions, fiduciary duties, disclosures and supervision depend on the actual business. The internet-adviser rule concerns eligibility for SEC registration, not blanket exemption from regulation. [SEC automated-advice guidance](https://www.sec.gov/about/divisions-offices/division-investment-management/electronic-filing-investment-advisers-iard/frequently-asked-questions-form-adv-iard), [Internet-adviser final rule](https://www.sec.gov/files/rules/final/2024/ia-6578.pdf)

Solicitation, participation in execution, handling funds or transaction-related compensation can raise broker-dealer registration issues. A flat SaaS fee does not automatically resolve advisory status. Avoid custody, pooled funds, per-trade compensation or performance fees before specialist review. [SEC broker-dealer guide](https://www.sec.gov/about/divisions-offices/division-trading-markets/division-trading-markets-compliance-guides/guide-broker-dealer-registration)

For futures/commodity interests, compensated advice or trading authority can trigger **CTA** issues; pooling funds can trigger **CPO** issues. Do not restrict the legal review to securities. [NFA registration categories](https://www.nfa.futures.org/registration-membership/who-has-to-register/), [CTA trading authority](https://www.nfa.futures.org/members/cta/index.html)

### Prediction markets by state

Do not promise “legal in all 50 states.” Nevada's regulator announced a preliminary injunction against Kalshi in April 2026; its current proceedings index also records actions involving Polymarket and Coinbase. These establish material state/federal conflict, not a final nationwide outcome. The precise live enforceability of every order, appeal and contract class on October 30 is **UNVERIFIED**. A complete 50-state eligibility matrix was not established. Sports, political and economic contracts cannot be treated as one undifferentiated permission. [Nevada injunction announcement](https://www.gaming.nv.gov/siteassets/content/about/press-release/ngcb-granted-preliminary-injunction-against-kalshi.pdf), [Current regulator proceedings](https://www.gaming.nv.gov/about-us/press-releases-public-statements)

Enforce provider eligibility, age, residency/location and contract restrictions; recheck on order entry. Cloudflare egress location is not the customer's legal location. Never route around a geographic block. International Polymarket's current US close-only policy independently blocks using that API for new US positions. [Polymarket restrictions](https://docs.polymarket.com/api-reference/geoblock)

### Crypto

API access and 24/7 trading do not make custody, money transmission, derivatives or state licensing requirements disappear. FinCEN distinguishes personal users from administrators/exchangers and analyzes business functions rather than labels. Handling/transmitting customer value can trigger obligations; noncustodial software design reduces some exposure but is not a universal exemption. Use provider-managed custody and trade-only scoped access where possible. [FinCEN CVC guidance](https://www.fincen.gov/system/files/2019-05/FinCEN%20CVC%20Guidance%20FINAL.pdf), [FinCEN participant definitions](https://www.fincen.gov/resources/statutes-regulations/guidance/application-fincens-regulations-persons-administering)

Current state-by-state crypto eligibility, asset-specific securities treatment and applicability of any 2026 legislative changes to this business are **UNVERIFIED**. Obtain a dated legal review before paid autonomous execution. These are launch constraints, not a conclusion that all trading bots are illegal.

## Recommendations for Money 24

1. Start with **one personal Webull US sandbox**, one account and a small liquid stock/ETF universe. Validate partial fills, cancels, duplicate submissions, reconnects and delayed-data behavior before derivatives.
2. Apply for **Connect partner access** early if SaaS account linking is a requirement. Do not build a commercial launch schedule around assumed approval or free partner terms.
3. Build separate **broker and market-data adapters**; add Alpaca paper as the first fallback, Kalshi demo only when events are in scope.
4. Keep Webull as brokerage truth and retain a **small durable execution/audit journal**. Full order history reduces duplication but does not replace application state.
5. Use shared REST snapshots/polling first. Prove MQTT/WebSocket on a deployed DO before promising continuous streams; use a local gRPC companion if needed. Thirty reasoning agents should share data and execution coordination.
6. Keep initial data spend at zero with explicitly labeled delayed/IEX/venue-specific feeds. For real consolidated equities/options, budget the verified **$99/month Alpaca** candidate; futures and SaaS display licensing need separate quotes.
7. Separate personal paper research from paid autonomous execution. Secure broker approval, data rights and legal review before customer-account discretion.

## Open risks

- Webull account/API approval, sandbox-only minimum balance and Connect partner commercial terms.
- Region conflicts, especially UK options; Canadian developer access; fractional/overnight parity outside US.
- Exact Webull OpenAPI quote prices, free Level 1 qualification, symbol limits and redistribution/non-display classification.
- Bonds announced in paperTrade but no verified public bond-order API; multi-leg/assignment/expiry simulator fidelity.
- gRPC incompatibility with native Workers, sandbox MQTT reachability, reconnect behavior and free-tier exhaustion.
- History differences across Trading/Broker/Connect, execution-correction retention and missing historical P&L guarantees.
- No measured provider latency ranking; timestamps, feed coverage and venue differences can invalidate comparisons.
- Finnhub pricing/free tier and Schwab API paper support remain unresolved; do not build budgets around remembered figures.
- Broker-specific PDT transition, unsettled-funds restrictions, options permissions and futures delivery controls.
- State prediction-market litigation, separate Polymarket US access, crypto eligibility and changing law.
- Paid advice/execution could require adviser, broker-dealer or CTA compliance; OAuth does not resolve it.
- No real-money or authenticated sandbox tests were performed in this research pass.
