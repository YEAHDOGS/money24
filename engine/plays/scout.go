package plays

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/YEAHDOGS/money24/engine/calendar"
	"github.com/YEAHDOGS/money24/engine/internal/diskusage"
	"github.com/YEAHDOGS/money24/engine/market"
)

const (
	// The scout looks for short-term option plays every minute of the session: same-day contracts on
	// the index ETFs, the nearest expiry on the big single names. Every setup that fires becomes a whole
	// play (contract, entry, entry window, stop, two targets, time exit, expected hold, liquidity, the
	// market's odds) and every play it finds is followed to its outcome whether it is traded or not, so
	// each setup's real record builds up every session. It trades nothing itself yet: the board and the
	// record come first.
	SCOUT_EVERY        = time.Minute
	SCOUT_TIMEOUT      = 50 * time.Second
	SCOUT_GAP          = 120 * time.Millisecond // between requests
	SCOUT_FILE         = "scout.jsonl"          // every play found and how it ended, one line each
	SCOUT_KEPT         = 300
	SCOUT_DEDUPE       = 30 * time.Minute // one play per setup, name and side in this long
	SCOUT_ENTRY_WINDOW = 5 * time.Minute  // the entry is good for this long after it is found
	SCOUT_STORIES      = 5 * time.Minute  // the news board is read this often
	CHART_TODAY        = "1d"
	CHART_MINUTE       = "1m"
	TICK               = 0.01
	MIN_HOLD_LEFT      = 30 * time.Minute // an expiry needs this much time left past the play's hold
	SCOUT_LOG          = "scout: "

	OUTCOME_OPEN   = "open"
	OUTCOME_TARGET = "target"
	OUTCOME_STOP   = "stop"
	OUTCOME_TIME   = "time"
	MARK_CHAIN     = "chain" // the contract's own quote
	MARK_MODEL     = "model" // worked out from the stock's price at the contract's volatility

	WHY_SCOUT_CLOSED = "the stock session is closed"
	WHY_NO_CONTRACT  = "no contract with a two-sided quote, %d+ contracts traded and a spread under %.0f%%"
)

// ScoutRules is the scout's part of the rulebook.
type ScoutRules struct {
	On             bool     `json:"on"`
	Indexes        []string `json:"indexes"`         // read every pass: same-day expiries every weekday
	Stocks         []string `json:"stocks"`          // read a few a pass, in turn
	StocksPerPass  int      `json:"stocks_per_pass"` // keeps the requests to the data source flat
	StopPct        float64  `json:"stop_pct"`        // below the entry
	Target1Pct     float64  `json:"target1_pct"`     // half the position
	Target2Pct     float64  `json:"target2_pct"`     // the rest
	FlatMinutes    int      `json:"flat_minutes"`    // everything out this long before the close
	MaxSpreadPct   float64  `json:"max_spread_pct"`
	MinVolume      int      `json:"min_volume"` // contracts traded today on the strike
	MinRelVolume   float64  `json:"min_rel_volume"`
	RiskPct        float64  `json:"risk_pct"`         // of the account at the stop, for the suggested size
	MaxPositionPct float64  `json:"max_position_pct"` // the most of the account one play may cost; at least one contract when it fits
	MaxSharePct    float64  `json:"max_share_pct"`    // of the strike's volume or open interest one play may take
	// Trade is how the scout trades its best play each pass. Nil in an older rulebook: the defaults apply.
	Trade *ScoutTrade `json:"trade,omitempty"`
}

// DefaultScout is how the scout runs until the rulebook says otherwise.
func DefaultScout() *ScoutRules {
	return &ScoutRules{On: true, Indexes: []string{"SPY", "QQQ", "IWM"},
		Stocks: []string{"NVDA", "TSLA", "AAPL", "AMD", "META", "AMZN", "MSFT", "GOOGL", "PLTR"}, StocksPerPass: 3,
		StopPct: 35, Target1Pct: 40, Target2Pct: 100, FlatMinutes: 10, MaxSpreadPct: 12, MinVolume: 100, MinRelVolume: 1.2,
		RiskPct: 10, MaxPositionPct: 25, MaxSharePct: 5, Trade: DefaultScoutTrade()}
}

// Candidate is one play the scout found: everything needed to make it, and how it ended.
type Candidate struct {
	ID        string    `json:"id"`
	FoundAt   time.Time `json:"found_at"`
	Setup     string    `json:"setup"`
	Symbol    string    `json:"symbol"`
	Direction string    `json:"direction"` // bullish or bearish
	Contract  Contract  `json:"contract"`
	OCC       string    `json:"occ"`
	Spot      float64   `json:"spot"`
	Entry     float64   `json:"entry"` // limit, per share: the mid when found
	EntryBy   time.Time `json:"entry_by"`
	Stop      float64   `json:"stop"`
	Target1   float64   `json:"target1"`
	Target2   float64   `json:"target2"`
	ExitBy    time.Time `json:"exit_by"`
	HoldMin   int       `json:"hold_min"` // how long a trade like this usually takes
	Qty       int       `json:"qty"`      // suggested contracts, from the account and the market
	CostUSD   float64   `json:"cost_usd"`
	Liquidity struct {
		Bid          float64 `json:"bid"`
		Ask          float64 `json:"ask"`
		SpreadPct    float64 `json:"spread_pct"`
		Volume       int     `json:"volume"`
		OpenInterest int     `json:"open_interest"`
		MaxQty       int     `json:"max_qty"` // contracts the market can take without being the market
	} `json:"liquidity"`
	IV         float64  `json:"iv"`
	OddsTarget float64  `json:"odds_target_pct"` // the market's odds the stock reaches the target 1 price within the hold
	OddsStop   float64  `json:"odds_stop_pct"`
	EdgeUSD    float64  `json:"edge_usd"` // rough expected value per contract from those odds
	Strength   float64  `json:"strength"`
	Score      float64  `json:"score"`
	Reasons    []string `json:"reasons"`

	Outcome   string    `json:"outcome"` // open, target, stop or time
	Mark      float64   `json:"mark"`
	MarkBy    string    `json:"mark_by,omitempty"` // chain or model
	ResultPct float64   `json:"result_pct"`
	ClosedAt  time.Time `json:"closed_at,omitzero"`
	PlayID    string    `json:"play_id,omitempty"` // the play it became, when the scout traded it
	Traded    string    `json:"traded,omitempty"`  // why a pick was not traded, when it was tried
}

// SetupStats is one setup's record so far.
type SetupStats struct {
	Setup    string  `json:"setup"`
	Found    int     `json:"found"`
	Closed   int     `json:"closed"`
	Wins     int     `json:"wins"`
	WinPct   float64 `json:"win_pct"`
	AvgPct   float64 `json:"avg_pct"` // average result per play, percent of the premium
	TotalPct float64 `json:"total_pct"`
}

// ScoutStatus is the board: the scout's state, the open plays best first, the latest closed ones and each setup's record.
type ScoutStatus struct {
	BytesOnDisk int64        `json:"bytes_on_disk"`
	At          time.Time    `json:"at"`
	State       string       `json:"state"`
	Why         string       `json:"why,omitempty"`
	Scanned     []string     `json:"scanned"`
	Open        []Candidate  `json:"open"`
	Closed      []Candidate  `json:"closed"`
	Stats       []SetupStats `json:"stats"`
	NextAt      time.Time    `json:"next_at"`
	EveryMs     int64        `json:"every_ms"`
}

// scoutDesk is the scout's memory between passes.
type scoutDesk struct {
	mu        sync.Mutex
	last      ScoutStatus
	all       []Candidate                   // newest last
	volumes   map[string]map[string]float64 // last pass's premium traded per contract, by symbol
	turn      int                           // which stocks this pass reads
	stories   []foundStory
	storiesAt time.Time
	loaded    bool
	lastMu    sync.Mutex // guards last alone, so the board can be read while a pass is running
}

// ScoutStatus returns the board.
func (s *Service) ScoutStatus() ScoutStatus {
	s.scout.lastMu.Lock()
	status := s.scout.last
	s.scout.lastMu.Unlock()
	if s.Store != nil {
		status.BytesOnDisk = s.Store.bytesOnDisk() + diskusage.Files(filepath.Join(s.Store.dir, SCOUT_FILE))
	}
	return status
}

// loadScout reads the record back once, so the stats survive a restart.
func (s *Service) loadScout() {
	if s.scout.loaded {
		return
	}
	s.scout.loaded = true
	b, err := os.ReadFile(filepath.Join(s.Store.dir, SCOUT_FILE))
	if err != nil {
		return
	}
	latest := map[string]Candidate{}
	order := []string{}
	for _, line := range strings.Split(string(b), "\n") {
		var c Candidate
		if json.Unmarshal([]byte(line), &c) != nil || c.ID == "" {
			continue
		}
		if _, ok := latest[c.ID]; !ok {
			order = append(order, c.ID)
		}
		latest[c.ID] = c
	}
	for _, id := range order {
		s.scout.all = append(s.scout.all, latest[id])
	}
}

// saveScout appends one candidate's current state to the record. A dry run keeps no record.
func (s *Service) saveScout(c Candidate) {
	if s.ScoutDry {
		return
	}
	f, err := os.OpenFile(filepath.Join(s.Store.dir, SCOUT_FILE), os.O_CREATE|os.O_APPEND|os.O_WRONLY, FILE_PERMS)
	if err != nil {
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(c)
}

// pickExpiry is the nearest expiry with room for the hold, from a chain that knows every expiry.
func pickExpiry(expiries []int64, now time.Time, hold time.Duration) (int64, string) {
	for _, e := range expiries {
		day := time.Unix(e, 0).UTC().Format(EXPIRY_LAYOUT)
		closes, ok := expiryClose(day)
		if ok && now.Add(hold+MIN_HOLD_LEFT).Before(closes) {
			return e, day
		}
	}
	return 0, ""
}

// pickStrike is the first strike just out of the money that is liquid enough, walking outward.
func pickStrike(rows []market.OptionRow, spot float64, call bool, f ScoutRules) (market.OptionRow, bool) {
	sorted := append([]market.OptionRow(nil), rows...)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].Strike < sorted[b].Strike })
	if !call {
		sort.Slice(sorted, func(a, b int) bool { return sorted[a].Strike > sorted[b].Strike })
	}
	for _, r := range sorted {
		outOfMoney := (call && r.Strike >= spot) || (!call && r.Strike <= spot)
		mid := (r.Bid + r.Ask) / 2
		if !outOfMoney || r.Bid <= 0 || r.Ask < r.Bid || mid <= 0 {
			continue
		}
		if (r.Ask-r.Bid)/mid*PERCENT <= f.MaxSpreadPct && r.Volume >= f.MinVolume {
			return r, true
		}
	}
	return market.OptionRow{}, false
}

// candidate turns a setup that fired into a whole play on one contract.
func (s *Service) candidate(hit setupHit, sess session, chain market.Chain, expiry string, f ScoutRules, funds Funds, now, flat time.Time) (Candidate, error) {
	call := hit.Direction == DIR_UP
	rows := chain.Puts
	if call {
		rows = chain.Calls
	}
	row, ok := pickStrike(rows, sess.Last, call, f)
	if !ok {
		return Candidate{}, fmt.Errorf(WHY_NO_CONTRACT, f.MinVolume, f.MaxSpreadPct)
	}
	right := RIGHT_PUT
	if call {
		right = RIGHT_CALL
	}
	mid := math.Round((row.Bid+row.Ask)/2*CENT) / CENT
	c := Candidate{ID: NewID(now), FoundAt: now.UTC(), Setup: hit.Setup, Symbol: sess.Symbol, Spot: sess.Last, Reasons: hit.Reasons,
		Contract: Contract{Symbol: sess.Symbol, Market: MARKET_OPTION, Right: right, Strike: row.Strike, Expiry: expiry}, OCC: row.Contract,
		Entry: mid, EntryBy: now.Add(SCOUT_ENTRY_WINDOW).UTC(), Strength: hit.Strength, IV: row.IV, Outcome: OUTCOME_OPEN, Mark: mid}
	c.Direction = ANGLE_BEARISH
	if call {
		c.Direction = ANGLE_BULLISH
	}
	c.Stop = math.Max(TICK, math.Round(mid*(1-f.StopPct/PERCENT)*CENT)/CENT)
	c.Target1 = math.Round(mid*(1+f.Target1Pct/PERCENT)*CENT) / CENT
	c.Target2 = math.Round(mid*(1+f.Target2Pct/PERCENT)*CENT) / CENT
	exit := now.Add(hit.Hold)
	if closes, ok := expiryClose(expiry); ok && closes.Add(-time.Duration(f.FlatMinutes)*time.Minute).Before(exit) {
		exit = closes.Add(-time.Duration(f.FlatMinutes) * time.Minute)
	}
	if flat.Before(exit) {
		exit = flat
	}
	c.ExitBy, c.HoldMin = exit.UTC(), int(exit.Sub(now).Minutes())
	c.Liquidity.Bid, c.Liquidity.Ask, c.Liquidity.Volume, c.Liquidity.OpenInterest = row.Bid, row.Ask, row.Volume, row.OpenInterest
	c.Liquidity.SpreadPct = math.Round((row.Ask-row.Bid)/mid*PERCENT*10) / 10
	c.Liquidity.MaxQty = int(math.Floor(float64(min(row.Volume, max(row.OpenInterest, row.Volume))) * f.MaxSharePct / PERCENT))
	// The market's own odds: the stock price that puts the contract at target 1 (or the stop) by the exit,
	// and the chance the stock touches it within the hold at the contract's implied volatility.
	expiryAt, _ := expiryClose(expiry)
	holdYears := yearsUntil(now, exit)
	if spotT1, ok := spotFor(c.Target1, row.Strike, yearsUntil(exit, expiryAt), row.IV, call); ok && row.IV > 0 {
		c.OddsTarget = math.Round(touchChance(sess.Last, spotT1, holdYears, row.IV)*PERCENT*10) / 10
	}
	if spotStop, ok := spotFor(c.Stop, row.Strike, yearsUntil(exit, expiryAt), row.IV, call); ok && row.IV > 0 {
		c.OddsStop = math.Round(touchChance(sess.Last, spotStop, holdYears, row.IV)*PERCENT*10) / 10
	}
	c.EdgeUSD = math.Round((c.OddsTarget/PERCENT*(c.Target1-mid)-c.OddsStop/PERCENT*(mid-c.Stop))*OPTION_MULT*CENT) / CENT
	perContractRisk := (mid - c.Stop) * OPTION_MULT
	if perContractRisk > 0 {
		c.Qty = int(math.Floor(funds.EquityUSD * f.RiskPct / PERCENT / perContractRisk))
	}
	unit := mid * OPTION_MULT
	most := int(math.Floor(math.Min(funds.EquityUSD*f.MaxPositionPct/PERCENT, funds.BuyingPowerUSD) / unit))
	// A small account still gets one contract when one fits inside the position cap.
	c.Qty = max(0, min(max(c.Qty, min(1, most)), most, c.Liquidity.MaxQty))
	c.CostUSD = math.Round(float64(c.Qty)*mid*OPTION_MULT*CENT) / CENT
	c.Score = math.Round(hit.Strength*(PERCENT+c.EdgeUSD)*10) / 10
	return c, nil
}

// follow marks every open play and closes the ones that reached the stop, target 1 or their time.
// A contract's own quote is used when its chain was read this pass; otherwise the price is worked out
// from the stock at the contract's volatility.
func (s *Service) follow(now time.Time, spots map[string]float64, quotes map[string]float64) {
	for i := range s.scout.all {
		c := &s.scout.all[i]
		if c.Outcome != OUTCOME_OPEN {
			continue
		}
		mark, by := quotes[c.OCC], MARK_CHAIN
		if mark <= 0 {
			spot, ok := spots[c.Symbol]
			if !ok {
				continue
			}
			expiryAt, _ := expiryClose(c.Contract.Expiry)
			mark, by = optionPrice(spot, c.Contract.Strike, yearsUntil(now, expiryAt), c.IV, c.Contract.Right == RIGHT_CALL), MARK_MODEL
		}
		c.Mark, c.MarkBy = math.Round(mark*CENT)/CENT, by
		switch {
		case mark <= c.Stop:
			c.Outcome = OUTCOME_STOP
		case mark >= c.Target1:
			c.Outcome = OUTCOME_TARGET
		case !now.Before(c.ExitBy):
			c.Outcome = OUTCOME_TIME
		default:
			continue
		}
		exit := map[string]float64{OUTCOME_STOP: c.Stop, OUTCOME_TARGET: c.Target1, OUTCOME_TIME: c.Mark}[c.Outcome]
		c.ResultPct = math.Round((exit/c.Entry-1)*PERCENT*10) / 10
		c.ClosedAt = now.UTC()
		s.saveScout(*c)
	}
}

// stats adds up each setup's record.
func stats(all []Candidate) []SetupStats {
	by := map[string]*SetupStats{}
	for _, c := range all {
		st := by[c.Setup]
		if st == nil {
			st = &SetupStats{Setup: c.Setup}
			by[c.Setup] = st
		}
		st.Found++
		if c.Outcome == OUTCOME_OPEN {
			continue
		}
		st.Closed++
		st.TotalPct += c.ResultPct
		if c.ResultPct > 0 {
			st.Wins++
		}
	}
	out := []SetupStats{}
	for _, st := range by {
		if st.Closed > 0 {
			st.WinPct = math.Round(float64(st.Wins)/float64(st.Closed)*PERCENT*10) / 10
			st.AvgPct = math.Round(st.TotalPct/float64(st.Closed)*10) / 10
		}
		st.TotalPct = math.Round(st.TotalPct*10) / 10
		out = append(out, *st)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].AvgPct > out[b].AvgPct })
	return out
}

// readName loads one name's minute candles and the chains a play would use, and works out its session.
func (s *Service) readName(ctx context.Context, symbol string, opens, asOf time.Time) (session, market.Chain, error) {
	chart, err := s.Resolver.Market.Chart(ctx, symbol, CHART_TODAY, CHART_MINUTE)
	if err != nil {
		return session{}, market.Chain{}, err
	}
	candles := []market.Candle{}
	for _, c := range chart.Candles {
		if c.T <= asOf.Unix() {
			candles = append(candles, c)
		}
	}
	sess, ok := readSession(symbol, candles, opens)
	if !ok {
		return sess, market.Chain{}, fmt.Errorf("%s: too few minutes of today's session yet", symbol)
	}
	time.Sleep(SCOUT_GAP)
	chain, err := s.Resolver.Market.Chain(ctx, symbol, 0)
	return sess, chain, err
}

// ScanAt runs one pass at asOf: now, or for a dry run a past minute of the latest session, whose
// candles then stop at asOf while the chains are the latest.
func (s *Service) ScanAt(ctx context.Context, now, asOf time.Time) ScoutStatus {
	s.scout.mu.Lock()
	defer s.scout.mu.Unlock()
	s.loadScout()
	f := s.Store.Rules().Scout
	st := ScoutStatus{At: now.UTC(), State: FINDER_LOOKING, Scanned: []string{}, NextAt: now.Add(SCOUT_EVERY).UTC(), EveryMs: SCOUT_EVERY.Milliseconds()}
	day := calendar.On(asOf)
	switch {
	case f == nil || !f.On || s.Resolver.Market == nil:
		st.State = FINDER_OFF
	case day.Session == calendar.SESSION_CLOSED || asOf.Before(day.Opens) || !asOf.Before(day.Closes):
		st.State, st.Why = FINDER_CLOSED, WHY_SCOUT_CLOSED
	}
	if st.State != FINDER_LOOKING {
		s.board(&st)
		return st
	}
	flat := day.Closes.Add(-time.Duration(f.FlatMinutes) * time.Minute)
	if s.scout.volumes == nil {
		s.scout.volumes = map[string]map[string]float64{}
	}
	names := append([]string{}, f.Indexes...)
	for i := range f.StocksPerPass {
		if len(f.Stocks) > 0 {
			names = append(names, f.Stocks[(s.scout.turn*f.StocksPerPass+i)%len(f.Stocks)])
		}
	}
	s.scout.turn++
	if now.Sub(s.scout.storiesAt) > SCOUT_STORIES && s.NewsURL != "" {
		if board, err := s.stories(ctx); err == nil {
			s.scout.stories, s.scout.storiesAt = board, now
		}
	}
	funds := Funds{EquityUSD: s.Store.Rules().BankrollUSD, BuyingPowerUSD: s.Store.Rules().BankrollUSD}
	if s.Funds != nil {
		funds = s.Funds(now)
	}
	spots, quotes, fresh := map[string]float64{}, map[string]float64{}, []int{}
	for _, symbol := range names {
		sess, chain, err := s.readName(ctx, symbol, day.Opens, asOf)
		if err != nil {
			log.Printf(SCOUT_LOG+"%v", err)
			continue
		}
		st.Scanned = append(st.Scanned, symbol)
		spots[symbol] = sess.Last
		// Premium traded per contract since the last pass: the flow of the last minute.
		callUSD, putUSD, seen := 0.0, 0.0, map[string]float64{}
		for _, side := range [][]market.OptionRow{chain.Calls, chain.Puts} {
			for _, r := range side {
				quotes[r.Contract] = r.Bid
				usd := float64(r.Volume) * (r.Bid + r.Ask) / 2 * OPTION_MULT
				seen[r.Contract] = usd
				before, ok := s.scout.volumes[symbol][r.Contract]
				if !ok || usd <= before {
					continue
				}
				if strings.Contains(r.Contract[len(symbol):], RIGHT_CALL) {
					callUSD += usd - before
				} else {
					putUSD += usd - before
				}
			}
		}
		firstPass := s.scout.volumes[symbol] == nil
		s.scout.volumes[symbol] = seen
		hits := []setupHit{}
		if h, ok := orbHit(sess, *f); ok {
			hits = append(hits, h)
		}
		if h, ok := vwapHit(sess, *f); ok {
			hits = append(hits, h)
		}
		if h, ok := flowHit(symbol, callUSD, putUSD); ok && !firstPass {
			hits = append(hits, h)
		}
		if h, ok := lastHourHit(sess, asOf, day.Closes); ok {
			hits = append(hits, h)
		}
		for _, story := range s.scout.stories {
			for _, l := range story.Links {
				if l.Symbol == symbol && l.Relation == RELATION_DIRECT && (l.Angle == ANGLE_BULLISH || l.Angle == ANGLE_BEARISH) {
					if h, ok := newsHit(sess, story, l.Angle); ok {
						hits = append(hits, h)
					}
				}
			}
		}
		for _, hit := range hits {
			if s.recent(hit, symbol, now) {
				continue
			}
			expiry, expiryDay := pickExpiry(chain.Expiries, asOf, hit.Hold)
			if expiry == 0 {
				continue
			}
			use := chain
			if expiry != chain.Expiry {
				time.Sleep(SCOUT_GAP)
				if use, err = s.Resolver.Market.Chain(ctx, symbol, expiry); err != nil {
					continue
				}
			}
			c, err := s.candidate(hit, sess, use, expiryDay, *f, funds, asOf, flat)
			if err != nil {
				log.Printf(SCOUT_LOG+"%s %s: %v", symbol, hit.Setup, err)
				continue
			}
			s.scout.all = append(s.scout.all, c)
			fresh = append(fresh, len(s.scout.all)-1)
			s.saveScout(c)
			log.Printf(SCOUT_LOG+"%s %s %s %g%s %s: entry %.2f, stop %.2f, target %.2f, out by %s, odds %.0f%%",
				hit.Setup, symbol, c.Direction, c.Contract.Strike, c.Contract.Right, c.Contract.Expiry, c.Entry, c.Stop, c.Target1,
				c.ExitBy.In(MARKET_TZ).Format("15:04"), c.OddsTarget)
		}
		time.Sleep(SCOUT_GAP)
	}
	s.follow(asOf, spots, quotes)
	if !s.ScoutDry {
		trade := f.Trade
		if trade == nil {
			trade = DefaultScoutTrade()
		}
		st.Why = s.tradeBest(fresh, trade, now)
	}
	if len(s.scout.all) > SCOUT_KEPT {
		s.scout.all = s.scout.all[len(s.scout.all)-SCOUT_KEPT:]
	}
	s.board(&st)
	return st
}

// recent says whether this setup already found a play on this name and side lately.
func (s *Service) recent(hit setupHit, symbol string, now time.Time) bool {
	dir := map[int]string{DIR_UP: ANGLE_BULLISH, DIR_DOWN: ANGLE_BEARISH}[hit.Direction]
	for _, c := range s.scout.all {
		if c.Setup == hit.Setup && c.Symbol == symbol && c.Direction == dir && now.Sub(c.FoundAt) < SCOUT_DEDUPE {
			return true
		}
	}
	return false
}

// board fills the status's lists from the record: open plays best first, the newest closed ones.
func (s *Service) board(st *ScoutStatus) {
	st.Open, st.Closed = []Candidate{}, []Candidate{}
	for i := len(s.scout.all) - 1; i >= 0; i-- {
		c := s.scout.all[i]
		if c.Outcome == OUTCOME_OPEN {
			st.Open = append(st.Open, c)
		} else if len(st.Closed) < SCOUT_KEPT/10 {
			st.Closed = append(st.Closed, c)
		}
	}
	sort.Slice(st.Open, func(a, b int) bool { return st.Open[a].Score > st.Open[b].Score })
	st.Stats = stats(s.scout.all)
}

// RunScout looks every minute until ctx ends.
func (s *Service) RunScout(ctx context.Context) {
	tick := time.NewTicker(SCOUT_EVERY)
	defer tick.Stop()
	for {
		pass, cancel := context.WithTimeout(ctx, SCOUT_TIMEOUT)
		now := time.Now()
		st := s.ScanAt(pass, now, now)
		cancel()
		s.scout.lastMu.Lock()
		s.scout.last = st
		s.scout.lastMu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
