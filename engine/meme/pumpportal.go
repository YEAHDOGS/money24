package meme

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/YEAHDOGS/money24/engine/dexscreener"
)

const (
	PUMP_URL       = "https://pumpportal.fun/api/data" // https with an RFC 6455 upgrade is wss.
	PUMP_SOURCE    = "pumpportal"
	PUMP_LAUNCH    = "create"
	PUMP_MIGRATION = "migrate"
	PUMP_SUB_NEW   = `{"method":"subscribeNewToken"}`
	PUMP_SUB_MOVE  = `{"method":"subscribeMigration"}`
	RECONNECT_MIN  = 5 * time.Second
	RECONNECT_MAX  = 2 * time.Minute
)

type pumpEvent struct {
	Kind          string   `json:"txType"`
	Mint          string   `json:"mint"`
	Signature     string   `json:"signature"`
	Pool          string   `json:"pool"`
	PoolAddress   string   `json:"poolAddress"`
	Creator       string   `json:"traderPublicKey"`
	Name          string   `json:"name"`
	Symbol        string   `json:"symbol"`
	URI           string   `json:"uri"`
	Curve         string   `json:"bondingCurveKey"`
	InitialBuy    *float64 `json:"initialBuy"`
	SOLAmount     *float64 `json:"solAmount"`
	VirtualTokens *float64 `json:"vTokensInBondingCurve"`
	VirtualSOL    *float64 `json:"vSolInBondingCurve"`
	MarketCapSOL  *float64 `json:"marketCapSol"`
	Mayhem        *bool    `json:"is_mayhem_mode"`
	Error         string   `json:"error"`
}

func decodePump(data []byte, at time.Time) (Record, error) {
	var ev pumpEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return Record{}, fmt.Errorf("decode PumpPortal event: %w", err)
	}
	if ev.Error != "" {
		return Record{}, fmt.Errorf("PumpPortal subscription: %s", ev.Error)
	}
	if ev.Kind != PUMP_LAUNCH && ev.Kind != PUMP_MIGRATION {
		return Record{}, nil
	}
	if !dexscreener.ADDRESS.MatchString(ev.Mint) {
		return Record{}, fmt.Errorf("PumpPortal %s event has an invalid mint", ev.Kind)
	}
	row := Record{At: at.UTC(), Source: PUMP_SOURCE, Mint: ev.Mint, Signature: ev.Signature}
	if ev.Kind == PUMP_MIGRATION {
		row.Kind = KIND_GRADUATION
		row.Graduation = &Graduation{Venue: ev.Pool, Pool: ev.PoolAddress}
		return row, nil
	}
	row.Kind = KIND_LAUNCH
	row.Launch = &Launch{
		Creator: ev.Creator, Name: ev.Name, Symbol: ev.Symbol, MetadataURI: ev.URI, Venue: ev.Pool,
		Curve: ev.Curve, InitialBuy: ev.InitialBuy, InitialSOL: ev.SOLAmount, VirtualTokens: ev.VirtualTokens,
		VirtualSOL: ev.VirtualSOL, MarketCapSOL: ev.MarketCapSOL, Mayhem: ev.Mayhem,
	}
	if ev.VirtualSOL != nil && ev.VirtualTokens != nil && *ev.VirtualTokens > 0 {
		price := *ev.VirtualSOL / *ev.VirtualTokens
		row.Launch.PriceSOL = &price
	}
	return row, nil
}

type streamUpdate struct {
	row       Record
	connected bool
	err       error
}

func sendStream(ctx context.Context, updates chan<- streamUpdate, update streamUpdate) bool {
	select {
	case updates <- update:
		return true
	case <-ctx.Done():
		return false
	}
}

func pumpSession(ctx context.Context, client *http.Client, url string, updates chan<- streamUpdate) (bool, error) {
	session, cancel := context.WithCancel(ctx)
	defer cancel()
	handshake := time.AfterFunc(WS_CONNECT_LIMIT, cancel)
	ws, err := dialWebsocket(session, client, url)
	handshake.Stop()
	if err != nil {
		return false, err
	}
	defer ws.Close()
	stop := context.AfterFunc(session, func() { ws.Close() })
	defer stop()
	idle := time.AfterFunc(WS_IDLE_LIMIT, cancel)
	defer idle.Stop()
	for _, subscription := range []string{PUMP_SUB_NEW, PUMP_SUB_MOVE} {
		if err := ws.send(WS_TEXT, []byte(subscription)); err != nil {
			return false, err
		}
	}
	if !sendStream(ctx, updates, streamUpdate{connected: true}) {
		return false, ctx.Err()
	}
	received := false
	for {
		data, err := ws.receive(func() { idle.Reset(WS_IDLE_LIMIT) })
		if err != nil {
			return received, err
		}
		row, err := decodePump(data, time.Now())
		if err != nil {
			if !sendStream(ctx, updates, streamUpdate{connected: true, err: err}) {
				return received, ctx.Err()
			}
			continue
		}
		if row.Kind != "" {
			received = true
			if !sendStream(ctx, updates, streamUpdate{connected: true, row: row}) {
				return received, ctx.Err()
			}
		}
	}
}

func pumpLoop(ctx context.Context, client *http.Client, url string, updates chan<- streamUpdate) {
	delay := RECONNECT_MIN
	for ctx.Err() == nil {
		received, err := pumpSession(ctx, client, url, updates)
		if ctx.Err() != nil {
			return
		}
		if !sendStream(ctx, updates, streamUpdate{err: fmt.Errorf("PumpPortal: %w", err)}) {
			return
		}
		if received {
			delay = RECONNECT_MIN
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(delay*2, RECONNECT_MAX)
	}
}
