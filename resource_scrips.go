// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"context"

	"github.com/high-live/high-openapi/generated"
)

type (
	QuotesRequest      = generated.QuotesJSONRequestBody
	OhlcRequest        = generated.OhlcJSONRequestBody
	HistoricalRequest  = generated.HistoricalJSONRequestBody
	OptionChainRequest = generated.OptionChainJSONRequestBody
)

// ExpiryType is which derivative family to list expiries for. Taken from the
// generated path-parameter type, so widening it to a bare string would let a
// value the API rejects compile — see contract §8.
type ExpiryType = generated.ExpiriesParamsType

const (
	ExpiryTypeFutures = generated.Futures
	ExpiryTypeOptions = generated.Options
)

// ScripsResource covers quotes, depth, expiries, futures reference data,
// historical candles and option chains.
type ScripsResource struct {
	config *resolvedConfig
}

// Quotes returns the last traded price and session change, keyed by trading symbol.
func (r *ScripsResource) Quotes(ctx context.Context, body QuotesRequest) (generated.QuoteMap, error) {
	return doRequest[generated.QuoteMap](ctx, r.config, requestOptions{
		method: "POST", path: "/scrips/quotes", auth: authBearer, body: body,
	})
}

// Ohlc returns quotes plus the session OHLCV block, keyed by trading symbol.
func (r *ScripsResource) Ohlc(ctx context.Context, body OhlcRequest) (generated.QuoteMap, error) {
	return doRequest[generated.QuoteMap](ctx, r.config, requestOptions{
		method: "POST", path: "/scrips/ohlc", auth: authBearer, body: body,
	})
}

// Depth returns five levels of bids and asks.
func (r *ScripsResource) Depth(ctx context.Context, symbol string) (generated.MarketDepth, error) {
	path, err := pathOf("/scrips/{symbol}/depth", map[string]string{"symbol": symbol})
	if err != nil {
		return generated.MarketDepth{}, err
	}
	return doRequest[generated.MarketDepth](ctx, r.config, requestOptions{method: "GET", path: path, auth: authBearer})
}

// Expiries returns the available expiries for a derivative underlying.
func (r *ScripsResource) Expiries(ctx context.Context, symbol string, expiryType ExpiryType) ([]generated.Expiry, error) {
	path, err := pathOf("/scrips/{symbol}/{type}/expiries", map[string]string{"symbol": symbol, "type": string(expiryType)})
	if err != nil {
		return nil, err
	}
	return doRequest[[]generated.Expiry](ctx, r.config, requestOptions{method: "GET", path: path, auth: authBearer})
}

// FutureData returns the futures contracts on an underlying.
func (r *ScripsResource) FutureData(ctx context.Context, symbol string) ([]generated.ScripInfo, error) {
	path, err := pathOf("/scrips/{symbol}/future-data", map[string]string{"symbol": symbol})
	if err != nil {
		return nil, err
	}
	return doRequest[[]generated.ScripInfo](ctx, r.config, requestOptions{method: "GET", path: path, auth: authBearer})
}

// Historical returns candles, columnar: every array has the same length and
// index i across them describes one candle.
func (r *ScripsResource) Historical(ctx context.Context, body HistoricalRequest) (generated.HistoricalCandles, error) {
	return doRequest[generated.HistoricalCandles](ctx, r.config, requestOptions{
		method: "POST", path: "/scrips/historical", auth: authBearer, body: body,
	})
}

// OptionChain returns the option chain for one underlying and expiry.
func (r *ScripsResource) OptionChain(ctx context.Context, body OptionChainRequest) (generated.OptionChain, error) {
	return doRequest[generated.OptionChain](ctx, r.config, requestOptions{
		method: "POST", path: "/scrips/option-chain", auth: authBearer, body: body,
	})
}
