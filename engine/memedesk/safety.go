package memedesk

import (
	"bytes"
	"strings"

	"github.com/YEAHDOGS/money24/engine/meme"
)

const (
	REASON_GRADUATION_ONLY = "graduation_only"
	REASON_MISSING_SAFETY  = "missing_safety_report"
	REASON_MINT_UNKNOWN    = "mint_authority_unknown"
	REASON_MINT_ACTIVE     = "mint_authority_active"
	REASON_FREEZE_UNKNOWN  = "freeze_authority_unknown"
	REASON_FREEZE_ACTIVE   = "freeze_authority_active"
	REASON_RUGGED          = "reported_rugged"
	REASON_RISK_FLAG       = "rugcheck_danger"
	REASON_HOLDERS_UNKNOWN = "holder_concentration_unknown"
	REASON_TOP_HOLDER      = "top_holder_concentration"
	REASON_TOP_HOLDERS     = "top_holders_concentration"
	REASON_INSIDER         = "reported_insider"
	REASON_LP_UNKNOWN      = "pool_lock_unknown"
	REASON_LP_LOW          = "pool_lock_below_minimum"
	REASON_BUYERS_UNKNOWN  = "unique_buyers_unknown"
	REASON_BUYERS_LOW      = "too_few_unique_buyers"
	REASON_BLOCK_UNKNOWN   = "creation_block_match_unknown"
	REASON_SAME_BLOCK      = "bonded_in_creation_block"
	REASON_CREATOR_UNKNOWN = "creator_sells_unknown"
	REASON_CREATOR_SOLD    = "creator_sold"
	REASON_FUNDING_UNKNOWN = "deployer_funding_unknown"
	REASON_DEPLOYER_FUNDED = "deployer_funded_buyers"
	REASON_LOW_LIQUIDITY   = "liquidity_below_minimum"
	REASON_NO_SPIKE        = "no_observed_spike"
	RUGCHECK_DANGER        = "danger"
	JSON_NULL              = "null"
)

func safetyReasons(o Observation, entry Entry) []string {
	report := o.Safety
	if report == nil {
		return []string{REASON_MISSING_SAFETY}
	}
	var reasons []string
	reasons = authorityReasons(reasons, report.MintAuthority, REASON_MINT_UNKNOWN, REASON_MINT_ACTIVE)
	reasons = authorityReasons(reasons, report.FreezeAuthority, REASON_FREEZE_UNKNOWN, REASON_FREEZE_ACTIVE)
	if report.Rugged != nil && *report.Rugged {
		reasons = append(reasons, REASON_RUGGED)
	}
	for _, risk := range report.Risks {
		if strings.EqualFold(risk.Level, RUGCHECK_DANGER) {
			reasons = append(reasons, REASON_RISK_FLAG)
			break
		}
	}
	reasons = holderReasons(reasons, o, entry)
	if o.Event.Kind != meme.KIND_GRADUATION {
		return reasons
	}
	pool := ""
	if o.Event.Graduation != nil {
		pool = o.Event.Graduation.Pool
	}
	if pool == "" && o.Market != nil {
		pool = o.Market.Pool
	}
	for _, market := range report.Markets {
		if pool == "" || market.Pool != pool || market.LP == nil || market.LP.LockedPct == nil {
			continue
		}
		if *market.LP.LockedPct < entry.MinLPLockedPct {
			return append(reasons, REASON_LP_LOW)
		}
		return reasons
	}
	return append(reasons, REASON_LP_UNKNOWN)
}

func authorityReasons(reasons []string, raw []byte, unknown, active string) []string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return append(reasons, unknown)
	}
	if !bytes.Equal(bytes.TrimSpace(raw), []byte(JSON_NULL)) {
		return append(reasons, active)
	}
	return reasons
}

func holderReasons(reasons []string, o Observation, entry Entry) []string {
	excluded := map[string]bool{}
	if o.Launch != nil && o.Launch.Curve != "" {
		excluded[o.Launch.Curve] = true
	}
	for _, market := range o.Safety.Markets {
		if market.Pool != "" {
			excluded[market.Pool] = true
		}
	}
	var largest, total float64
	known, missing, insider := false, false, false
	for _, holder := range o.Safety.TopHolders {
		if excluded[holder.Address] || excluded[holder.Owner] {
			continue
		}
		if holder.Percent == nil || *holder.Percent < 0 || *holder.Percent > MAX_PERCENT {
			missing = true
			continue
		}
		known = true
		largest = max(largest, *holder.Percent)
		total += *holder.Percent
		insider = insider || (holder.Insider != nil && *holder.Insider)
	}
	if !known || missing {
		reasons = append(reasons, REASON_HOLDERS_UNKNOWN)
	}
	if largest > entry.MaxTopHolderPct {
		reasons = append(reasons, REASON_TOP_HOLDER)
	}
	if total > entry.MaxTopHoldersPct {
		reasons = append(reasons, REASON_TOP_HOLDERS)
	}
	if insider {
		reasons = append(reasons, REASON_INSIDER)
	}
	return reasons
}
