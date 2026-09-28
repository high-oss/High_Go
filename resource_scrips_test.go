// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi_test

import (
	"context"
	"net/http"
	"testing"

	highopenapi "github.com/high-live/high-openapi"
)

func TestScripsQuotesReturnsAMapKeyedByTradingSymbol(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"RELIANCE-EQ": map[string]any{"scrip": "NSE@2885", "LTP": 1412.3, "prevClose": 1405.8, "change": 6.5, "changePer": 0.46},
		}))
	})
	defer ts.Close()

	quotes, err := clientFor(t, ts, nil).Scrips.Quotes(context.Background(), highopenapi.QuotesRequest{
		Symbols: highopenapi.Ptr([]string{"RELIANCE-EQ"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	q, ok := quotes["RELIANCE-EQ"]
	if !ok || q.LTP != 1412.3 {
		t.Fatalf("quotes[RELIANCE-EQ] = %+v, ok=%v", q, ok)
	}
	if ts.Req(0).URL != "/v1/scrips/quotes" {
		t.Errorf("URL = %q", ts.Req(0).URL)
	}
}

func TestScripsOhlcReturnsTheSameMapShapeWithAnOhlcvBlock(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"RELIANCE-EQ": map[string]any{
				"scrip": "NSE@2885", "LTP": 1412.3, "prevClose": 1405.8, "change": 6.5, "changePer": 0.46,
				"ohlcv": map[string]any{"open": 1407, "high": 1418.6, "low": 1403.2, "close": 1412.3, "volume": 6843210},
			},
		}))
	})
	defer ts.Close()

	ohlc, err := clientFor(t, ts, nil).Scrips.Ohlc(context.Background(), highopenapi.OhlcRequest{
		Symbols: highopenapi.Ptr([]string{"RELIANCE-EQ"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if ohlc["RELIANCE-EQ"].Ohlcv == nil || ohlc["RELIANCE-EQ"].Ohlcv.Volume != 6843210 {
		t.Fatalf("ohlcv = %+v", ohlc["RELIANCE-EQ"].Ohlcv)
	}
}

func TestScripsDepthEncodesTheSymbolIntoThePath(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"tradingSymbol": "M&M-EQ", "bids": []any{}, "asks": []any{},
			"totalBidQuantity": 0, "totalAskQuantity": 0, "totalAskPercentage": 0, "totalBidsPercentage": 0,
		}))
	})
	defer ts.Close()

	if _, err := clientFor(t, ts, nil).Scrips.Depth(context.Background(), "M&M-EQ"); err != nil {
		t.Fatal(err)
	}
	if ts.Req(0).URL != "/v1/scrips/M%26M-EQ/depth" {
		t.Errorf("URL = %q", ts.Req(0).URL)
	}
}

func TestScripsExpiriesReturnsAnArrayAndEncodesBothParameters(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", []any{map[string]any{"expiry": "2026-09-29", "type": "M"}}))
	})
	defer ts.Close()

	expiries, err := clientFor(t, ts, nil).Scrips.Expiries(context.Background(), "NIFTY 50", highopenapi.ExpiryTypeOptions)
	if err != nil {
		t.Fatal(err)
	}
	if len(expiries) != 1 || string(expiries[0].Type) != "M" {
		t.Fatalf("expiries = %+v", expiries)
	}
	if ts.Req(0).URL != "/v1/scrips/NIFTY%2050/options/expiries" {
		t.Errorf("URL = %q", ts.Req(0).URL)
	}
}

func TestScripsFutureDataReturnsAnArrayOfScrips(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", []any{}))
	})
	defer ts.Close()

	futures, err := clientFor(t, ts, nil).Scrips.FutureData(context.Background(), "RELIANCE-EQ")
	if err != nil {
		t.Fatal(err)
	}
	if len(futures) != 0 {
		t.Errorf("futures = %v", futures)
	}
	if ts.Req(0).URL != "/v1/scrips/RELIANCE-EQ/future-data" {
		t.Errorf("URL = %q", ts.Req(0).URL)
	}
}

// historical returns columnar parallel arrays — index i across them is one
// candle — not an array of row objects.
func TestScripsHistoricalReturnsParallelColumnsOfEqualLength(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"tradingSymbol": "NSE@2885", "interval": "1D",
			"timestamp": []int{1789929000, 1790015400}, "open": []float64{1398.5, 1402}, "high": []float64{1406.2, 1409.8},
			"low": []float64{1394.1, 1399.3}, "close": []float64{1401.7, 1407.4}, "volume": []int{7215430, 6532180},
		}))
	})
	defer ts.Close()

	candles, err := clientFor(t, ts, nil).Scrips.Historical(context.Background(), highopenapi.HistoricalRequest{
		TradingSymbol: "RELIANCE-EQ", Interval: "1D", FromTime: 1789929000, ToTime: 1790330400,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(candles.Timestamp) != 2 {
		t.Fatalf("Timestamp = %v", candles.Timestamp)
	}
	if len(candles.Close) != len(candles.Timestamp) {
		t.Fatalf("Close length %d != Timestamp length %d", len(candles.Close), len(candles.Timestamp))
	}
}

func TestScripsOptionChainReturnsRowsWithCallAndPutLegs(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"baseStockInfo": map[string]any{
				"tradingSymbol": "RELIANCE-EQ", "scripCode": 2885, "exchange": "NSE", "segment": "ES",
				"scripKey": "NSE@2885", "symbol": "RELIANCE", "scripName": "Reliance", "marketLot": 1, "priceTick": 10,
			},
			"optionChain": []any{map[string]any{"strikePrice": 1400, "call": map[string]any{}, "put": map[string]any{}}},
			"marketLot":   500, "maxOrderLots": 36,
		}))
	})
	defer ts.Close()

	chain, err := clientFor(t, ts, nil).Scrips.OptionChain(context.Background(), highopenapi.OptionChainRequest{
		TradingSymbol: "RELIANCE-EQ", Expiry: highopenapi.Ptr("2026-09-29"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chain.OptionChain) != 1 || chain.OptionChain[0].StrikePrice != 1400 {
		t.Fatalf("OptionChain = %+v", chain.OptionChain)
	}
}

// quotes is a POST, so it is never retried even on a 503.
func TestScripsQuotesIsNeverRetried(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 503, map[string]any{"code": "SERVICE_UNAVAILABLE"})
	})
	defer ts.Close()

	_, err := clientFor(t, ts, nil).Scrips.Quotes(context.Background(), highopenapi.QuotesRequest{Symbols: highopenapi.Ptr([]string{"RELIANCE-EQ"})})
	if err == nil {
		t.Fatal("expected an error")
	}
	if ts.Count() != 1 {
		t.Fatalf("expected exactly 1 request, got %d", ts.Count())
	}
}

// The type parameter must reject anything the spec's enum does not declare.
// Go has no closed union type, so this is enforced by ExpiryType being the
// generated path-parameter type (contract §8's "never widen a spec enum" —
// see the report for the Go-specific ruling this required).
func TestScripsExpiryTypeIsTheGeneratedEnumType(t *testing.T) {
	var _ highopenapi.ExpiryType = highopenapi.ExpiryTypeFutures
	var _ highopenapi.ExpiryType = highopenapi.ExpiryTypeOptions
}
