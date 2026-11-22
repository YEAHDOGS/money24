package predict

// Record is the honest test of one strategy: every market it found, followed to its result, traded or
// not, as if one contract of each had been bought at the price then with Kalshi's fee.
type Record struct {
	Watched int     `json:"watched"`
	Settled int     `json:"settled"`
	Won     int     `json:"won"`
	WinPct  float64 `json:"win_pct"`
	// AvgCost is the average price paid with the fee: the win rate, as a share, that breaks even.
	AvgCost float64 `json:"avg_cost"`
	// EdgePct is what a dollar in every settled one made or lost.
	EdgePct float64 `json:"edge_pct"`
	// AvgProb is the strategy's own average chance for the settled ones it gave a chance for: beside
	// WinPct it shows whether its chances can be trusted (calibration).
	AvgProb float64 `json:"avg_prob,omitempty"`
	Bands   []Band  `json:"bands,omitempty"`
}

// Band is a strategy's record in one price range.
type Band struct {
	From    float64 `json:"from"`
	To      float64 `json:"to"`
	Watched int     `json:"watched"`
	Settled int     `json:"settled"`
	Won     int     `json:"won"`
	WinPct  float64 `json:"win_pct"`
	AvgCost float64 `json:"avg_cost"`
	EdgePct float64 `json:"edge_pct"`
}

// tally adds up settled watches.
type tally struct {
	watched, settled, won, probs int
	paid, made, prob             float64
}

func (t *tally) add(w *watch) {
	t.watched++
	if !w.settled {
		return
	}
	cost := w.line.Price + Fee(1, w.line.Price)
	t.settled++
	t.paid += cost
	t.made += w.payout - cost
	if w.payout > cost {
		t.won++
	}
	if w.line.Prob > 0 {
		t.probs++
		t.prob += w.line.Prob
	}
}

// rates is the win rate, the average cost and the edge, each 0 until something has settled.
func (t tally) rates() (winPct, avgCost, edgePct float64) {
	if t.settled == 0 {
		return 0, 0, 0
	}
	n := float64(t.settled)
	if t.paid > 0 {
		edgePct = round(t.made/t.paid*PERCENT, USD_DIGITS)
	}
	return round(float64(t.won)/n*PERCENT, 1), round(t.paid/n, PRICE_DIGITS), edgePct
}

// Record adds up one strategy's watches, and its bands when it has any.
func (b *Book) Record(strategy string, bands []float64) Record {
	all := tally{}
	per := make([]tally, max(0, len(bands)-1))
	for _, w := range b.watches {
		if w.line.Strategy != strategy {
			continue
		}
		all.add(w)
		if i := bandOf(bands, w.line.Price); i >= 0 {
			per[i].add(w)
		}
	}
	r := Record{Watched: all.watched, Settled: all.settled, Won: all.won}
	r.WinPct, r.AvgCost, r.EdgePct = all.rates()
	if all.probs > 0 {
		r.AvgProb = round(all.prob/float64(all.probs)*PERCENT, 1)
	}
	for i, t := range per {
		band := Band{From: bands[i], To: bands[i+1], Watched: t.watched, Settled: t.settled, Won: t.won}
		band.WinPct, band.AvgCost, band.EdgePct = t.rates()
		r.Bands = append(r.Bands, band)
	}
	return r
}

// bandOf is the band a price falls in: each includes its bottom, and the last its top too.
func bandOf(bands []float64, price float64) int {
	for i := 0; i+1 < len(bands); i++ {
		last := i+2 == len(bands)
		if price >= bands[i] && (price < bands[i+1] || last && price <= bands[i+1]) {
			return i
		}
	}
	return -1
}
