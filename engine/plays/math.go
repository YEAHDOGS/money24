package plays

import (
	"math"
	"time"
)

const (
	RISK_FREE        = 0.04 // annual rate used in option pricing; small next to the volatility term
	HOURS_PER_YEAR   = 365 * 24
	EXPIRY_HOUR      = 16 // US options stop trading at 4 PM New York on expiry day
	SOLVE_STEPS      = 100
	SOLVE_LOW_FACTOR = 0.01 // search the stock from 1% to 500% of the strike
	SOLVE_HIGH       = 5.0
	TRADING_DAYS     = 252
	RSI_PERIOD       = 14
	MA_SHORT         = 20
	MA_LONG          = 50
	HV_PERIOD        = 20
	PERCENT          = 100.0
	CENT             = 100.0
	SQRT2            = math.Sqrt2
	TOUCH_FACTOR     = 2 // reflection principle: the chance of touching a level is twice the chance of ending past it
)

// MARKET_TZ is where US option expiries are clocked.
var MARKET_TZ = func() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.UTC
	}
	return loc
}()

func normCDF(x float64) float64 { return 0.5 * (1 + math.Erf(x/SQRT2)) }

// optionPrice is the Black-Scholes value of one share's worth of a call or put. years <= 0 or no
// volatility gives the value at expiry: what is in the money and nothing else.
func optionPrice(spot, strike, years, vol float64, call bool) float64 {
	if years <= 0 || vol <= 0 || spot <= 0 {
		if call {
			return max(0, spot-strike)
		}
		return max(0, strike-spot)
	}
	sd := vol * math.Sqrt(years)
	d1 := (math.Log(spot/strike) + (RISK_FREE+vol*vol/2)*years) / sd
	d2 := d1 - sd
	disc := strike * math.Exp(-RISK_FREE*years)
	if call {
		return spot*normCDF(d1) - disc*normCDF(d2)
	}
	return disc*normCDF(-d2) - spot*normCDF(-d1)
}

// spotFor finds the stock price at which the option is worth target with years left. ok is false
// when no stock price gets it there (a put can never be worth more than its strike).
func spotFor(target, strike, years, vol float64, call bool) (float64, bool) {
	lo, hi := strike*SOLVE_LOW_FACTOR, strike*SOLVE_HIGH
	value := func(s float64) float64 { return optionPrice(s, strike, years, vol, call) }
	// Calls gain as the stock rises, puts as it falls; flip the put so both search the same way.
	if !call {
		lo, hi = hi, lo
	}
	if value(hi) < target || value(lo) > target {
		return 0, false
	}
	for range SOLVE_STEPS {
		mid := (lo + hi) / 2
		if value(mid) < target {
			lo = mid
		} else {
			hi = mid
		}
	}
	return math.Round((lo+hi)/2*CENT) / CENT, true
}

// touchChance is the chance the stock trades at level at some point in the next years, from its
// volatility alone (no drift): the market's own odds, before any evidence.
func touchChance(spot, level, years, vol float64) float64 {
	if spot <= 0 || level <= 0 {
		return 0
	}
	if years <= 0 || vol <= 0 {
		return 0
	}
	distance := math.Abs(math.Log(level / spot))
	return min(1, TOUCH_FACTOR*normCDF(-distance/(vol*math.Sqrt(years))))
}

// yearsUntil is the time from now to t, in years, never negative.
func yearsUntil(now, t time.Time) float64 { return max(0, t.Sub(now).Hours()/HOURS_PER_YEAR) }

// expiryClose is 4 PM New York on the contract's expiry day.
func expiryClose(expiry string) (time.Time, bool) {
	day, err := time.ParseInLocation(EXPIRY_LAYOUT, expiry, MARKET_TZ)
	if err != nil {
		return time.Time{}, false
	}
	return day.Add(EXPIRY_HOUR * time.Hour), true
}

// sma is the average of the last n closes, or 0 with too few.
func sma(closes []float64, n int) float64 {
	if len(closes) < n || n <= 0 {
		return 0
	}
	sum := 0.0
	for _, c := range closes[len(closes)-n:] {
		sum += c
	}
	return sum / float64(n)
}

// rsi is Wilder's relative strength index over the closes, or 0 with too few.
func rsi(closes []float64, period int) float64 {
	if len(closes) <= period {
		return 0
	}
	gain, loss := 0.0, 0.0
	for i := 1; i <= period; i++ {
		d := closes[i] - closes[i-1]
		gain, loss = gain+max(0, d), loss+max(0, -d)
	}
	gain, loss = gain/float64(period), loss/float64(period)
	for i := period + 1; i < len(closes); i++ {
		d := closes[i] - closes[i-1]
		gain = (gain*float64(period-1) + max(0, d)) / float64(period)
		loss = (loss*float64(period-1) + max(0, -d)) / float64(period)
	}
	if loss == 0 {
		return PERCENT
	}
	return PERCENT - PERCENT/(1+gain/loss)
}

// historicalVol is the annualised standard deviation of daily log returns over the last n days.
func historicalVol(closes []float64, n int) float64 {
	if len(closes) <= n {
		return 0
	}
	window := closes[len(closes)-n-1:]
	rets := make([]float64, 0, n)
	mean := 0.0
	for i := 1; i < len(window); i++ {
		r := math.Log(window[i] / window[i-1])
		rets, mean = append(rets, r), mean+r
	}
	mean /= float64(len(rets))
	variance := 0.0
	for _, r := range rets {
		variance += (r - mean) * (r - mean)
	}
	return math.Sqrt(variance/float64(len(rets)-1)) * math.Sqrt(TRADING_DAYS)
}
