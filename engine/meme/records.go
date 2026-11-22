package meme

import "time"

const (
	KIND_LAUNCH       = "launch"
	KIND_GRADUATION   = "graduation"
	KIND_SNAPSHOT     = "snapshot"
	KIND_SAFETY       = "safety"
	KIND_CONNECTED    = "stream_connected"
	KIND_DISCONNECTED = "stream_disconnected"
	DAY_LAYOUT        = "2006-01-02"
	FILE_CLOCK_LAYOUT = "150405.000000000"
	FILE_PREFIX       = "records-"
	FILE_SUFFIX       = ".jsonl"
	DIRECTORY_MODE    = 0o700
	TRACK_ADMITTED    = "admitted"
	TRACK_CAPACITY    = "capacity"
	TRACK_UNLISTED    = "unlisted"
)

// Record is one observed event or lookup. At is UTC ingestion time. Signature
// identifies an on-chain event and can be used to deduplicate across recorders.
type Record struct {
	At          time.Time   `json:"at"`
	RequestedAt time.Time   `json:"requested_at,omitzero"`
	Kind        string      `json:"kind"`
	Source      string      `json:"source"`
	Mint        string      `json:"mint,omitempty"`
	Signature   string      `json:"signature,omitempty"`
	Launch      *Launch     `json:"launch,omitempty"`
	Graduation  *Graduation `json:"graduation,omitempty"`
	Market      *Market     `json:"market,omitempty"`
	Safety      *Safety     `json:"safety,omitempty"`
	Tracking    string      `json:"tracking,omitempty"`
	Evicted     string      `json:"evicted_mint,omitempty"`
	Error       string      `json:"error,omitempty"`
}

// Launch describes the bonding curve and launch account in PumpPortal's create event.
type Launch struct {
	Creator       string   `json:"creator,omitempty"`
	Name          string   `json:"name,omitempty"`
	Symbol        string   `json:"symbol,omitempty"`
	MetadataURI   string   `json:"metadata_uri,omitempty"`
	Venue         string   `json:"venue,omitempty"`
	Curve         string   `json:"curve,omitempty"`
	InitialBuy    *float64 `json:"initial_buy_tokens,omitempty"`
	InitialSOL    *float64 `json:"initial_sol,omitempty"`
	VirtualTokens *float64 `json:"virtual_tokens,omitempty"`
	VirtualSOL    *float64 `json:"virtual_sol,omitempty"`
	MarketCapSOL  *float64 `json:"market_cap_sol,omitempty"`
	PriceSOL      *float64 `json:"price_sol,omitempty"`
	Mayhem        *bool    `json:"mayhem,omitempty"`
}

// Graduation identifies the destination venue and pool address when supplied.
type Graduation struct {
	Venue          string `json:"venue,omitempty"`
	Pool           string `json:"pool,omitempty"`
	UniqueBuyers   *int   `json:"unique_buyers,omitempty"`
	SameBlock      *bool  `json:"same_block,omitempty"`
	CreatorSold    *bool  `json:"creator_sold,omitempty"`
	DeployerFunded *bool  `json:"deployer_funded,omitempty"`
}
