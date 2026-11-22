# News sources

`engine/news/sources.json` went from 56 to 313 sources: 258 added, 1 dead one removed (MarketWatch Pulse).
Every added source was fetched live through the real newsd fetcher and parser and returned titled, linked,
dated items with a recent newest item (November 2026). The check is `engine/news/live_test.go`:

```
cd engine
M24D_LIVE=1 go test -buildvcs=false ./news -run TestLiveSources -v -parallel 10
```

It is skipped unless `M24D_LIVE=1`. `M24D_SOURCES=<file>` checks another sources file, `M24D_ONLY=<id prefix>`
checks a few, `M24D_REVALIDATE=1` asks each feed a second time to see whether it answers 304. SEC and BLS answer 403
to the default User-Agent, so their entries are skipped unless `USER_AGENT` carries a contact (the same rule that
already applies to the EDGAR and `bls-latest` feeds). A GDELT answer of HTTP 429 is reported as a skip.

## Cost

| | |
|---|---|
| Requests a day added | about 18,800 (0.22 a second), at the `min_every` floors below |
| Bytes a day added, worst case | about 266 MB a day: every poll downloads the whole feed |
| Bytes a day added, expected | about 145 to 205 MB: 108 of the 249 feeds measured answer 304 Not Modified (about 1 KB) when nothing changed; the rest always send the whole feed |
| Typical feed | 16 KB compressed; the heaviest are Axios 216 KB, TheStreet 152 KB, Fox News Tech 99 KB, Fox News World 72 KB |
| Request bytes sent | a few hundred bytes each, under 10 MB a day |

Floors (`min_every`): 5 minutes for the two wire queries (Reuters, AP), 10 minutes for about 35 top feeds, 15 minutes
for 80 general feeds, 30 minutes for 101 section and international feeds, hourly for 24 quiet or heavy ones, and
3 to 6 hours for regulators, data releases and the Economist and Kiplinger. A floor only has effect above the global
poll interval (one minute by default), and it is a floor: raising the slider raises every source's interval.

One thing worth knowing: the 25 pre-existing sources with no floor poll every minute, about 36,000 requests a day,
twice what this change adds. A `min_every` of 5 minutes on them would cut that to about 7,200. Left alone here.

## Sources by group

Counts are the sources added in each group. "Polls" lists the `min_every` values used by that outlet's feeds.

### US television and wires (60)

| Outlet | Feeds | Polls | Covers | Reputation |
|---|---|---|---|---|
| CNN | 2 | 10m | General news and business, from Google News queries scoped to cnn.com. | Large US cable network with a big newsroom; broad and fast. Its own RSS froze years ago, so the Google News route is the only live one. |
| CNBC | 18 | 10m, 15m, 30m | Sections: finance, business, investing, earnings, economy, world, tech, US, politics, Europe, Asia, energy, real estate, health, media, autos, retail, personal finance (the top-news feed was already there). | NBCUniversal's business network and the default US source for market and earnings headlines; free headlines. |
| Fox News | 5 | 15m, 10m, 30m | Latest, politics, world, US, tech. | US cable network with the largest cable audience; news reporting sits beside opinion programming and a right-leaning editorial line. |
| Fox Business | 4 | 10m, 15m, 30m | Markets, economy, tech, real estate (the latest feed was already there). | Fox's markets network; competent market and economy desk, with the same editorial lean as Fox News. |
| Reuters | 4 | 5m, 10m | General, business, markets, world, from Google News queries scoped to reuters.com. | Global wire service and the usual neutral baseline for market-moving headlines. Its public RSS was discontinued; see the Google News note below. |
| Associated Press | 2 | 5m, 10m | General and business, from Google News queries scoped to apnews.com. | Not-for-profit wire cooperative with strict neutrality rules; fast and widely syndicated. It has no public RSS. |
| CBS News | 5 | 15m, 30m | Main, MoneyWatch, politics, world, tech. | US broadcast network news division; mainstream general reporting with a small business desk. |
| NBC News | 5 | 15m, 30m | Top, business, politics, world, tech. | US broadcast network news division; mainstream general reporting. |
| ABC News | 5 | 15m, 30m | Top, US, international, politics, tech (business was already there). | US broadcast network news division; mainstream general reporting. |
| NPR | 6 | 15m, 30m | News, business, economy, politics, world, tech. | US public radio; careful and well sourced, slower and light on markets. |
| PBS NewsHour | 3 | 30m | Headlines, economy, politics. | US public broadcaster nightly news; careful and slow. |
| USA Today | 1 | 10m | General news, from a Google News query scoped to usatoday.com (its own feeds answer HTTP 402). | Gannett national daily; broad general-interest news, light on depth, heavy on sports and lifestyle. |

### US newspapers and political press (24)

| Outlet | Feeds | Polls | Covers | Reputation |
|---|---|---|---|---|
| The New York Times | 8 | 15m, 30m | World, US, tech, economy, DealBook, Asia Pacific, Europe, Middle East (Business and Politics were already there). | Newspaper of record; paywalled, but headlines and summaries are free. |
| The Washington Post | 6 | 30m, 10m | Business, politics, world, national, tech, plus a Google News query. | Major national daily; its RSS feeds are short (a dozen or two items), so a Google News query backs them up. |
| Politico | 4 | 15m, 30m | Politics, Congress, energy, and Politico Europe. | Washington policy and politics trade press; strong on legislation, regulation and Brussels. |
| The Hill | 2 | 15m, 30m | Main feed and business. | Congress- and campaign-focused outlet; mainstream, quick on Capitol Hill news. |
| Axios | 1 | 1h | Main feed (a large file, so it polls hourly). | Short-form newsroom; good on tech and deal scoops, thin on detail. |
| New York Post | 1 | 15m | Business. | Tabloid with real New York finance scoops; treat single-source claims with care. |
| Los Angeles Times | 1 | 30m | Business. | Major regional daily with solid business and entertainment-industry coverage. |
| The Boston Globe | 1 | 30m | Business. | Major regional daily; useful on biotech and New England finance. |

### Financial press and market news (63)

| Outlet | Feeds | Polls | Covers | Reputation |
|---|---|---|---|---|
| Bloomberg | 7 | 10m, 15m, 30m | Markets, business, politics, tech, economics, industries, plus a Google News query. | The premier markets news service; paywalled, so the native feeds give headlines and a short lead only. |
| The Wall Street Journal | 6 | 10m, 15m | Markets, business, world, tech, US, plus a Google News query. | Top US business daily (Dow Jones); paywalled, headline and teaser only. |
| MarketWatch | 1 | 10m | Bulletins (Top Stories was already there; Pulse was dropped as dead). | Dow Jones retail-markets site; quick, short items. |
| Barron's | 1 | 15m | A Google News query scoped to barrons.com (its own RSS answers 403). | Dow Jones investing weekly; analytical, with picks and market commentary. |
| Financial Times | 8 | 10m, 15m, 30m, 1h | Markets, companies, world, US, UK, tech, Alphaville, plus a Google News query. | Global financial daily, widely treated as the benchmark for finance reporting; paywalled, headline and teaser only. |
| The Economist | 5 | 6h | Finance and economics, business, leaders, United States, international. Each feed lists about 300 items, so they poll every 6 hours. | Weekly magazine; long-form analysis, not breaking news. |
| Business Insider | 2 | 30m, 10m | All content and markets. | High-volume business site; real scoops mixed with listicles and clickbait. |
| Fortune | 1 | 30m | Main feed. | Established business magazine; corporate and CEO focus. |
| Forbes | 2 | 30m | Business and innovation. | Business magazine; a large share of posts are contributor pieces that are not held to staff-editor standards. |
| Kiplinger | 1 | 6h | Main feed (a large file, so it polls every 6 hours). | Personal-finance magazine; advice and explainers rather than news. |
| The Motley Fool | 1 | 15m | The public article feed. | CAVEAT: a stock-picking subscription publisher (Stock Advisor, Rule Breakers). Its articles are opinion and promotion as much as news; treat them as sentiment, never as a source of fact. |
| Seeking Alpha | 2 | 10m | Articles and Market Currents. | Crowd-sourced investor commentary: contributors are not vetted staff and often hold positions. Market Currents is the news-like part. |
| Benzinga | 1 | 15m | Main feed. | Retail-trader news and movers; fast, with some promotional content. |
| Investor's Business Daily | 1 | 30m | Main feed. | Growth-stock and technical-analysis newspaper; coverage follows its own stock-selection method. |
| TheStreet | 1 | 1h | Main feed (a large file, so it polls hourly). | Retail finance site; mixed quality, some promotional pieces. |
| Investing.com | 6 | 10m, 15m | News, stocks, commodities, crypto, economy, forex. | Big retail aggregator and calendar site; many items are syndicated or analyst notes. |
| Nasdaq | 6 | 10m, 15m, 30m | Markets, stocks, earnings, IPOs, ETFs, commodities. | The exchange operator's editorial feed; mostly syndicated and earnings-driven. |
| Yahoo Finance | 5 | 30m | Headline feeds for the Dow, Nasdaq, Bitcoin, crude oil and gold (the S&P 500 feed was already there). | Largest US retail finance portal; a mix of wire copy, partner posts and its own reporters. |
| PR Newswire | 1 | 30m | Financial-services press releases. | Company-issued releases: promotional by nature, but the primary source for announcements. |
| TechCrunch | 2 | 15m, 30m | Main feed and venture. | Startup and venture news; quick on funding rounds. |
| The Verge | 1 | 30m | Main feed. | Consumer-tech news site; good on product and platform stories. |
| Wired | 2 | 30m | Main feed and business. | Technology magazine; analysis and long-form. |

### Crypto press (4)

| Outlet | Feeds | Polls | Covers | Reputation |
|---|---|---|---|---|
| The Block | 1 | 15m | Main feed. | Institutional crypto news and research; one of the better-sourced crypto outlets. |
| CryptoSlate | 1 | 15m | Main feed. | Established crypto news site; mixed with some sponsored content. |
| BeInCrypto | 1 | 15m | Main feed. | Crypto news and price analysis; includes sponsored content. |
| Bitcoin Magazine | 1 | 30m | Main feed. | Bitcoin-focused outlet with a bitcoin-maximalist outlook. |

### UK, Ireland and Europe (30)

| Outlet | Feeds | Polls | Covers | Reputation |
|---|---|---|---|---|
| BBC News | 9 | 10m, 15m, 30m | Top, politics, US and Canada, Asia, Europe, Middle East, Africa, Latin America, tech (Business and World were already there). | UK public broadcaster; the most trusted general English-language news brand in most surveys. |
| The Guardian | 4 | 15m, 30m | World, US, US business, tech (UK business was already there). | UK national daily, free to read, left-leaning editorial line; strong on investigations and climate. |
| The Independent | 1 | 30m | World. | UK online-only national; mainstream, quick, lighter sourcing on some pieces. |
| The Telegraph | 1 | 1h | Business. | UK national daily, right-leaning; paywalled, strong on City and UK business. |
| Euronews | 2 | 15m | General and business. | Pan-European broadcaster; short, EU-centred items. |
| Deutsche Welle | 2 | 15m, 1h | All news and business. | German public international broadcaster; reliable, EU- and Germany-centred. |
| France 24 | 1 | 15m | General news (Business was already there). | French public international broadcaster. |
| Le Monde | 2 | 30m | Main and economy, in French. | France's newspaper of record. |
| Tagesschau | 2 | 15m, 30m | Main and economy, in German. | ARD's flagship news programme; German public broadcaster, careful and dry. |
| Der Spiegel | 1 | 30m | Economy, in German. | Germany's leading news magazine; strong investigative tradition. |
| Handelsblatt | 1 | 15m | Top themes, in German. | Germany's leading financial daily. |
| FAZ | 1 | 30m | Finance, in German. | Frankfurter Allgemeine Zeitung; conservative German newspaper of record with a strong business desk. |
| ANSA | 1 | 15m | Main feed, in Italian. | Italy's main news agency (a cooperative); fast wire. |
| Il Sole 24 Ore | 1 | 1h | Finance, in Italian. | Italy's financial daily; Confindustria-linked. |
| La Repubblica | 1 | 30m | Economy, in Italian. | Major Italian daily, centre-left. |

### Middle East, Asia, Africa and Oceania (48)

| Outlet | Feeds | Polls | Covers | Reputation |
|---|---|---|---|---|
| Al Jazeera | 1 | 10m | Main English feed. | Qatar-funded broadcaster; deep Middle East reach, with a state-funder perspective on Gulf politics. |
| The Times of Israel | 1 | 15m | Main feed. | Israeli English-language news site; fast and detailed on regional security. |
| Haaretz | 1 | 15m | Latest headlines. | Israeli daily, liberal; strong on politics and business. |
| The Jerusalem Post | 1 | 30m | Front page. | Israeli English daily, right-of-centre. |
| South China Morning Post | 5 | 15m, 30m | Business, China, Asia, Hong Kong, tech. | Hong Kong's English daily and the main English source on China business. |
| CNA | 3 | 15m, 30m | Asia, world, business. | Singapore's Mediacorp broadcaster (state-owned parent); solid regional business coverage. |
| The Straits Times | 3 | 15m, 30m | Asia, world, business. | Singapore's national daily; solid on South-East Asia, government-friendly. |
| The Japan Times | 1 | 15m | Main feed. | Japan's oldest English-language daily. |
| NHK | 1 | 15m | Google News query scoped to NHK (its English RSS is gone and the Japanese one froze months ago). | Japan's public broadcaster. |
| Nikkei Asia | 1 | 15m | Google News query scoped to asia.nikkei.com (its own RSS carries no dates). | Nikkei's English edition; the best English source on Asian business. |
| Yonhap | 1 | 30m | Main English feed. | South Korea's main news agency; fast wire, state-subsidised. |
| Taipei Times | 1 | 30m | Main feed. | Taiwan's English daily; pro-independence editorial line. |
| Bangkok Post | 1 | 1h | Business. | Thailand's leading English daily. |
| The Times of India | 3 | 15m, 30m | Top, business, world. | India's largest English daily; mass-market, uneven on sourcing. |
| The Economic Times | 5 | 15m, 30m | Top, markets, economy, industry, stocks. | India's leading business daily. |
| Mint | 3 | 15m, 30m | Markets, economy, companies. | Indian business daily (HT Media). |
| The Hindu | 2 | 30m | Business and BusinessLine. | Respected Indian national daily and its business sister paper. |
| Daily Maverick | 1 | 30m | Main feed. | South African independent online daily; investigative strength. |
| Nation Kenya | 1 | 30m | Kenya feed. | Kenya's largest media group. |
| The Punch | 1 | 30m | Main feed. | Nigeria's most-read daily. |
| CBC | 4 | 15m, 30m | Top, business, world, politics. | Canada's public broadcaster; mainstream and careful. |
| The Globe and Mail | 1 | 30m | Business. | Canada's national newspaper; paywalled, strong on Bay Street. |
| ABC News Australia | 2 | 15m, 30m | Top stories and business. | Australia's public broadcaster. |
| Australian Financial Review | 1 | 15m | Main feed. | Australia's financial daily; paywalled, headline and teaser. |
| Sydney Morning Herald | 1 | 30m | Business. | Major Australian daily. |
| NZ Herald | 1 | 30m | Business. | New Zealand's largest daily. |
| RNZ | 1 | 1h | Business. | New Zealand's public radio broadcaster. |

### Regulators, central banks and economic data (29)

| Outlet | Feeds | Polls | Covers | Reputation |
|---|---|---|---|---|
| SEC | 3 | 30m, 1h | Press releases, litigation releases, speeches and statements (the four EDGAR filing feeds were already there). | The US securities regulator; primary source. Needs the contact User-Agent, like the EDGAR feeds. |
| CFTC | 2 | 1h | Press releases and enforcement. | US derivatives regulator; primary source, sporadic. |
| FDIC | 1 | 3h | News. | US bank deposit insurer; primary source, sporadic. |
| OCC | 1 | 3h | News releases. | US national bank regulator; primary source, sporadic. |
| US Treasury | 2 | 1h | Press and OFAC sanctions actions: a Google News query scoped to home.treasury.gov, plus Treasury documents in the Federal Register. | Primary source. Treasury's own press RSS no longer exists, hence the two indirect routes. |
| Department of Justice | 1 | 1h | Press releases. | Primary source for fraud, antitrust and sanctions cases; mostly not market-relevant, so it polls hourly. |
| US Trade Representative | 1 | 1h | Press and tariff actions. | Primary source on tariffs and trade deals. |
| European Commission | 1 | 1h | Press corner. | Primary source for EU regulation, competition and trade. |
| World Trade Organization | 1 | 3h | News. | Primary source on trade disputes; slow. |
| IMF | 1 | 1h | A Google News query scoped to imf.org (its own feed answers 403). | Primary source for the IMF; global macro forecasts and programmes. |
| UN News | 1 | 30m | News. | United Nations news service; official, world affairs. |
| Federal Reserve | 1 | 1h | Speeches and testimony (all press releases were already there). | The US central bank; primary source. |
| ECB | 1 | 3h | Publications (press releases were already there). | The euro-area central bank; primary source. |
| BIS | 1 | 3h | Central bank speeches. | The central banks' bank; primary source, slow. |
| Bank of Japan | 1 | 1h | What is new. | Japan's central bank; primary source. |
| Bank of Canada | 1 | 3h | Press releases. | Canada's central bank; primary source. |
| BEA | 1 | 3h | Releases. | US GDP and income statistician; primary source. |
| Census Bureau | 1 | 3h | Economic indicators calendar. | US retail, housing and trade statistics; primary source. |
| EIA | 1 | 1h | Energy data and analysis, from a Google News query scoped to eia.gov (its own feeds need parser work). | US energy statistics agency; primary source. |
| BLS | 6 | 1h, 3h | CPI, employment situation, PPI, JOLTS, employment cost index, productivity (the "latest numbers" feed was already there). | US labour statistics agency; primary source. The feeds carry the headline sentence of each monthly release. Needs the contact User-Agent. |

## How Reuters, AP, CNN and other closed feeds are covered

Reuters discontinued its public RSS in 2020 (`feeds.reuters.com` no longer resolves) and AP has no public RSS today
(`apnews.com` answers a bot challenge, HTTP 403). CNN's own RSS (`rss.cnn.com`) still answers but has not carried
a new item for years. What legitimately works is Google News search RSS, scoped to the outlet with `site:`. It is a
public Google feed, the same one the existing `gnews-*` sources use. Barron's, USA Today, the US Treasury, the IMF,
NHK, Nikkei Asia and EIA are covered the same way, and Bloomberg, the Wall Street Journal, the Financial Times,
The Washington Post and CNN also get a general query beside their native feeds.

What to expect from those feeds: they are marked `bundle`, so each item names its own outlet; links go through
Google's redirect page; Google returns at most 100 items per query, ranked by recency and relevance, so it is a
sample of the outlet's output, not a complete firehose. That is why the busiest ones poll every 5 to 10 minutes.
The only complete alternatives are paid (Reuters Connect, AP's licensed feed, Bloomberg's terminal API).

## The Motley Fool

Added as `motley-fool` from its public article feed. It is a stock-picking subscription publisher (Stock Advisor,
Rule Breakers): its articles are opinion and promotion as much as news. Use it as a sentiment signal and never as a
source of fact. Its `foolwatch` feeds answer 401 (they need an API key), so only the public feed is used.

## Dropped

Candidates that were fetched and did not make it, so nobody tries them again. Ages are the newest item's age on
22 November 2026.

**Abandoned, the feed answers but never updates**
- CNN native RSS (`rss.cnn.com`: top stories, world, latest, money, economy): newest items 2.2 to 8 years old.
  `edition.rss` fails TLS. A path-scoped Google News query (`site:cnn.com/business`) also returned only old items.
- MarketWatch Pulse (an existing source, removed) and MarketWatch Real-time Headlines: about 500 days old.
- WSJ `feeds.a.dj.com` feeds: 1.8 years old (the `feeds.content.dowjones.io` ones are live and used).
- Blockworks (319 days), Nasdaq cryptocurrencies (257 days), Telegraph News (52 days),
  Expansion (17 days), DL News (24 days), Times of India Tech (46 days), Fox News Science (56 days), NHK's Japanese
  feeds (about 105 days), Bank of England speeches and publications (30 days), SNB (23 days), CFPB (39 days),
  SEC trading suspensions (38 days), Fed bank-regulation press (8 days, already in `fed-press`), Energy Department
  (6 years), SF Fed (7 years), US Treasury `rss.xml` (20 years).
- Bloomberg Crypto (one item), Bloomberg Wealth (no items), Politico Economy (two items, a month old).

**Blocked or paywalled with nothing usable**
- HTTP 403 (bot walls): Barron's native feeds, AP hub RSS, Arab News, Benzinga `/news/feed`, Chicago Tribune,
  Commerce Department, Labor Department, GlobeNewswire, IMF, Les Echos, Morningstar, Nasdaq IR, OECD, OPEC,
  Yahoo News (US), MarketWatch Stocks to Watch.
- HTTP 402 "Access Restricted": every USA Today feed (a Google News query is used instead).
- HTTP 401, API key needed: Motley Fool `foolwatch` and `investing`.

**Moved, gone or never existed at the URL tried** (HTTP 404, 400, 500, or DNS failure)
- 404: Al Jazeera economy, Accesswire, BEA `bea.gov/rss.xml`, Business Day (ZA), CFTC STP, Dallas, Cleveland
  and Atlanta Feds, FDIC press releases, FHFA, FINRA, Forbes investing, money and real-time, FTC (three URLs),
  Gulf News, IEA, ILO, Investopedia, Irish Times, Kiplinger investing, NHK World English, Nikkei Asia `/rss`, NYSE, Cboe,
  OCC `occ-news-releases`, Politico Picks, RTE, Seeking Alpha `news/all`, SF Chronicle, Swissinfo, Treasury press
  releases (five URL variants), TreasuryDirect, The Verge tech, White House, Yahoo Finance `rssindex`, `topstories`
  and `/rss`, Yonhap economy, Bloomberg AI, EIA Weekly Petroleum and Natural Gas.
- 400: Fox Business energy, media, politics and retail (`moxie` has no such sections).
- 500: ABC Australia world. No DNS: `feeds.reuters.com`, the feedburner hosts (Zero Hedge, NDTV Profit, World Bank),
  `feeds.24.com` (News24), `xml2.corrieredellasera.it`.
- Empty or unusable: Business Wire (no items), US Fiscal Data, Census newsroom, EIA press releases (no items),
  Treasury Auctions (HTTP 406), CME (empty body), Kansas City and St. Louis Fed (timeout, stream error).

**Duplicates, wrong fit or too heavy for the benefit**
- Fox News and Fox Business `feeds.*` feedburner copies (same items as `moxie`), Fox Business personal finance
  (4.7 days quiet) and lifestyle, Fox News health and opinion, CNBC Pro, Travel and Small Business, WaPo Economy
  (same as Business), ABC Money (same as the existing Business), Sky World, Politics and Tech (two to seven items),
  Independent Business (two items), Hindustan Times business (four items), `fomc-minutes` and `fed-monetary`
  (already in `fed-press`), ECB FX reference rates, ICE press releases, NY Fed (parser, see below).
- Large for what they carry: Yahoo News (165 KB for 15 items), Semafor (350 KB), The Atlantic (119 KB), Time (100 KB),
  Eurostat (420 KB for 1,377 items), The National (248 KB), El Pais (163 KB), Dawn (84 KB), Roll Call (294 KB),
  Government Executive (199 KB), Vox, Slate.
- Reputation: Zero Hedge, Newsweek, Daily Mail, Middle East Eye, U.Today, NewsBTC, Bitcoinist.
- Query variants that returned noise or duplicates: USA Today money query (sports), SEC and other agencies
  in the Federal Register (hundreds of routine notices; only Treasury, for OFAC actions, is kept).

## Feeds that need parser work (for later)

Each of these is a valid, current feed that `parse.go` cannot read in full. Nothing was changed there.

| Feed | URL | Problem |
|---|---|---|
| Korea Herald business | `https://www.koreaherald.com/rss/kh_Business` | pubDate `Sun, 22 Nov 2026 14:46:28 +09:00`: an offset with a colon, no layout matches, so no item is dated |
| Financial Post | `https://financialpost.com/feed` | XML syntax error: the non-strict decoder stops at line 69 |
| Japan Times business | `https://www.japantimes.co.jp/news/business/feed/` | XML syntax error at line 13 (the main Japan Times feed parses) |
| NY Fed news and markets | `https://www.newyorkfed.org/rss/feeds/newsevents`, `.../markets` | XML syntax error at line 47 |
| EIA Today in Energy | `https://www.eia.gov/rss/todayinenergy.xml` | declared ISO-8859-1; the charset reader passes bytes through, so decoding fails with invalid UTF-8. EIA press releases (`.../press_rss.xml`) parse to zero items. EIA is covered through Google News meanwhile |
| FCA | `https://www.fca.org.uk/news/rss.xml` | pubDate `Thursday, November 19, 2026 - 17:19` |
| RBA media releases | `https://www.rba.gov.au/rss/rss-cb-media-releases.xml` | `dc:date` as `2026-11-06T10:45+11:00`: RFC 3339 without seconds |
| ESMA | `https://www.esma.europa.eu/rss.xml` | no pubDate; the date is only inside the description's `<time datetime>` |
| WHO news | `https://www.who.int/rss-feeds/news-english.xml` | pubDate ends in `Z` as a zone letter (`Sat, 10 Oct 2026 07:49:45 Z`) |
| Nikkei Asia | `https://asia.nikkei.com/rss/feed/nar` | RSS 1.0 with titles and links but no dates at all (Google News covers it) |
| Investing.com analysis | `https://www.investing.com/rss/market_overview.rss` | pubDate `Nov 21, 2026 08:08 GMT` |
| Bank of England speeches, publications | `https://www.bankofengland.co.uk/rss/speeches`, `.../publications` | many items undated (20 of 50 in publications); also quiet |

## Notes for whoever runs it

- SEC and BLS need a contact in the User-Agent (`USER_AGENT` or `--user-agent`), as already documented in
  `engine/README.md`. Without it all 9 new SEC and BLS sources, and the 5 existing ones, answer 403 and log an error
  each poll. They were verified with a contact User-Agent.
- BLS indicator feeds carry one item per monthly release, so they look idle between releases by design.
- The Economist feeds list about 300 items each on the first poll, so the first poll stores a long archive, once.
- Google News links are redirect pages, as with the existing `gnews-*` sources.
- The GDELT sources failed during testing with HTTP 429 after repeated runs from one address; that is GDELT's rate
  limit and is not related to this change.
