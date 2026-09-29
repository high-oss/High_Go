// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"encoding/json"
	"testing"
)

func rawFrameFrom(t *testing.T, obj map[string]any) rawFrame {
	t.Helper()
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	var f rawFrame
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// cng is the absolute change and nc is the percentage change -- the reverse
// of what the vendor's own React sample README claims. The apiDoc's own
// worked example (ltp 1905.65, c 1859.05, cng 46.60, nc 2.51 -- 46.60 /
// 1859.05 is 2.51%) is the authority (datafeed plan, Phase 0).
func TestMergeQuotePinsChangeAndChangePercent(t *testing.T) {
	frame := rawFrameFrom(t, map[string]any{
		"e": "nse_cm", "tk": "2885", "name": "sf",
		"ltp": "1905.65", "c": "1859.05", "cng": "46.60", "nc": "2.51",
	})
	q := &Quote{}
	mergeQuote(q, frame)

	if q.Change != "46.60" {
		t.Errorf("Change = %q, want the absolute move 46.60 (from cng)", q.Change)
	}
	if q.ChangePercent != "2.51" {
		t.Errorf("ChangePercent = %q, want the percentage move 2.51 (from nc)", q.ChangePercent)
	}
}

// Prices are decimal strings, kept exact -- never routed through float64.
func TestMergeQuoteKeepsDecimalPricesExact(t *testing.T) {
	frame := rawFrameFrom(t, map[string]any{
		"e": "nse_cm", "tk": "2885", "name": "sf",
		"ltp": "1905.65", "op": "1900.00", "yl": "1000.10",
	})
	q := &Quote{}
	mergeQuote(q, frame)
	if q.LastTradedPrice != "1905.65" {
		t.Errorf("LastTradedPrice = %q, want exact 1905.65", q.LastTradedPrice)
	}
	if q.Open != "1900.00" {
		t.Errorf("Open = %q, want exact 1900.00 (trailing zero preserved)", q.Open)
	}
}

// A tick is a delta: a field that was not sent this time must keep its
// previous value, and Changed must list only what this specific delta
// actually touched.
func TestMergeQuoteDeltaAcrossTicks(t *testing.T) {
	q := &Quote{}

	first := rawFrameFrom(t, map[string]any{"e": "nse_cm", "tk": "2885", "name": "sf", "ltp": "100.00", "v": "1000"})
	changed1 := mergeQuote(q, first)
	if q.LastTradedPrice != "100.00" || q.Volume != 1000 {
		t.Fatalf("after first tick: ltp=%q v=%d", q.LastTradedPrice, q.Volume)
	}
	if !containsExact(changed1, "LastTradedPrice") || !containsExact(changed1, "Volume") {
		t.Fatalf("changed1 = %v, want LastTradedPrice and Volume", changed1)
	}

	// Second tick only moves ltp -- v must be untouched, and Changed must
	// name only LastTradedPrice, not Volume (which merely carried forward).
	second := rawFrameFrom(t, map[string]any{"e": "nse_cm", "tk": "2885", "name": "sf", "ltp": "101.50"})
	changed2 := mergeQuote(q, second)
	if q.LastTradedPrice != "101.50" {
		t.Errorf("LastTradedPrice = %q, want 101.50", q.LastTradedPrice)
	}
	if q.Volume != 1000 {
		t.Errorf("Volume = %d, want 1000 (unchanged: the delta never carried it)", q.Volume)
	}
	if len(changed2) != 1 || changed2[0] != "LastTradedPrice" {
		t.Errorf("changed2 = %v, want exactly [LastTradedPrice]", changed2)
	}
}

// A field this package has never seen must survive into Extra and into
// Changed (by its wire name), not be silently dropped.
func TestMergeQuotePreservesUnknownFields(t *testing.T) {
	frame := rawFrameFrom(t, map[string]any{
		"e": "nse_cm", "tk": "2885", "name": "sf", "ltp": "100.00",
		"eqt": "50", "emqt": "25", // v7.9.0 imbalance-quantity additions
	})
	q := &Quote{}
	changed := mergeQuote(q, frame)
	if q.Extra["eqt"] != "50" || q.Extra["emqt"] != "25" {
		t.Errorf("Extra = %v, want eqt=50 emqt=25", q.Extra)
	}
	if !containsExact(changed, "eqt") || !containsExact(changed, "emqt") {
		t.Errorf("changed = %v, want it to include eqt and emqt", changed)
	}
}

// bp/bq/sp/bs (top-of-book) must never appear on Quote -- they are split out
// into a separate Depth event.
func TestMergeQuoteDoesNotAbsorbTopOfBook(t *testing.T) {
	frame := rawFrameFrom(t, map[string]any{
		"e": "nse_cm", "tk": "2885", "name": "sf",
		"ltp": "100.00", "bp": "99.95", "bq": "10", "sp": "100.05", "bs": "20",
	})
	q := &Quote{}
	changed := mergeQuote(q, frame)
	for _, wire := range []string{"bp", "bq", "sp", "bs"} {
		if containsExact(changed, wire) {
			t.Errorf("changed = %v, must not include top-of-book field %q", changed, wire)
		}
	}

	var bid, ask DepthLevel
	depthChanged := mergeTopOfBook(&bid, &ask, frame)
	if bid.Price != "99.95" || bid.Quantity != 10 {
		t.Errorf("bid = %+v", bid)
	}
	if ask.Price != "100.05" || ask.Quantity != 20 {
		t.Errorf("ask = %+v", ask)
	}
	want := []string{"AskPrice", "AskQuantity", "BidPrice", "BidQuantity"}
	if !sameStringSlice(depthChanged, want) {
		t.Errorf("depthChanged = %v, want %v", depthChanged, want)
	}
}

// The off-by-one trap the plan calls out explicitly: level n's price/
// quantity use suffix n-1 (empty for level 1) but its order count uses
// suffix n. Level 1 pairs bp (unsuffixed) with bno1; level 5 pairs bp4 with
// bno5.
func TestMergeDepthLevelNumberingPairing(t *testing.T) {
	frame := rawFrameFrom(t, map[string]any{
		"e": "nse_cm", "tk": "2885", "name": "dp",
		"bp": "100.00", "bq": "1", "bno1": "11",
		"bp1": "99.95", "bq1": "2", "bno2": "12",
		"bp2": "99.90", "bq2": "3", "bno3": "13",
		"bp3": "99.85", "bq3": "4", "bno4": "14",
		"bp4": "99.80", "bq4": "5", "bno5": "15",
		"sp": "100.05", "bs": "6", "sno1": "21",
		"sp1": "100.10", "bs1": "7", "sno2": "22",
		"sp2": "100.15", "bs2": "8", "sno3": "23",
		"sp3": "100.20", "bs3": "9", "sno4": "24",
		"sp4": "100.25", "bs4": "10", "sno5": "25",
	})
	var bids, asks [5]DepthLevel
	mergeDepth(&bids, &asks, frame)

	// Level 1: bp (unsuffixed) + bno1.
	if bids[0].Price != "100.00" || bids[0].Orders != 11 {
		t.Errorf("bids[0] = %+v, want price 100.00 orders 11 (bp + bno1)", bids[0])
	}
	// Level 5: bp4 + bno5.
	if bids[4].Price != "99.80" || bids[4].Orders != 15 {
		t.Errorf("bids[4] = %+v, want price 99.80 orders 15 (bp4 + bno5)", bids[4])
	}
	if asks[0].Price != "100.05" || asks[0].Orders != 21 {
		t.Errorf("asks[0] = %+v, want price 100.05 orders 21 (sp + sno1)", asks[0])
	}
	if asks[4].Price != "100.25" || asks[4].Orders != 25 {
		t.Errorf("asks[4] = %+v, want price 100.25 orders 25 (sp4 + sno5)", asks[4])
	}
	// Middle level sanity check too, since an off-by-one would still pass a
	// test that only checks the two ends.
	if bids[2].Price != "99.90" || bids[2].Orders != 13 {
		t.Errorf("bids[2] = %+v, want price 99.90 orders 13 (bp2 + bno3)", bids[2])
	}
}

// A depth delta only ever moves some levels; the rest must keep their
// previous value.
func TestMergeDepthDeltaAcrossTicks(t *testing.T) {
	var bids, asks [5]DepthLevel
	first := rawFrameFrom(t, map[string]any{
		"e": "nse_cm", "tk": "2885", "name": "dp",
		"bp": "100.00", "bq": "1", "bno1": "11", "bp1": "99.95", "bq1": "2", "bno2": "12",
	})
	mergeDepth(&bids, &asks, first)

	second := rawFrameFrom(t, map[string]any{"e": "nse_cm", "tk": "2885", "name": "dp", "bp": "100.05"})
	changed := mergeDepth(&bids, &asks, second)

	if bids[0].Price != "100.05" {
		t.Errorf("bids[0].Price = %q, want 100.05", bids[0].Price)
	}
	if bids[1].Price != "99.95" {
		t.Errorf("bids[1].Price = %q, want unchanged 99.95", bids[1].Price)
	}
	if len(changed) != 1 || changed[0] != "Bid1Price" {
		t.Errorf("changed = %v, want exactly [Bid1Price]", changed)
	}
}

func TestMergeIndex(t *testing.T) {
	frame := rawFrameFrom(t, map[string]any{
		"e": "nse_cm", "tk": "Nifty 50", "name": "if",
		"ts": "Nifty 50", "iv": "19500.25", "ic": "19480.10",
		"openingprice": " 19408.60", "highprice": "19510.00", "lowprice": "19400.00",
		"cng": "20.15", "nc": "0.10", "tvalue": "29-Apr-2020 14:34:36",
	})
	idx := &Index{}
	changed := mergeIndex(idx, frame)

	if idx.IndexName != "Nifty 50" {
		t.Errorf("IndexName = %q", idx.IndexName)
	}
	if idx.Open != "19408.60" {
		t.Errorf("Open = %q, want the leading space trimmed", idx.Open)
	}
	if idx.FeedTime.IsZero() {
		t.Error("FeedTime was not parsed")
	}
	if idx.FeedTime.Hour() != 14 || idx.FeedTime.Minute() != 34 {
		t.Errorf("FeedTime = %v, want 14:34 IST", idx.FeedTime)
	}
	if !containsExact(changed, "IndexName") || !containsExact(changed, "FeedTime") {
		t.Errorf("changed = %v", changed)
	}
}

func TestParseFeedTimeLayouts(t *testing.T) {
	ltt, err := parseFeedTime(lastTradedTimeLayout, "29/04/2020 15:59:44")
	if err != nil {
		t.Fatal(err)
	}
	if ltt.Day() != 29 || ltt.Month().String() != "April" || ltt.Hour() != 15 {
		t.Errorf("ltt = %v", ltt)
	}

	fdtm, err := parseFeedTime(feedTimeLayout, "29-Apr-2020 17:34:36")
	if err != nil {
		t.Fatal(err)
	}
	if fdtm.Day() != 29 || fdtm.Hour() != 17 {
		t.Errorf("fdtm = %v", fdtm)
	}

	// A malformed value is reported, not silently zeroed.
	if _, err := parseFeedTime(lastTradedTimeLayout, "not-a-date"); err == nil {
		t.Error("expected a parse error")
	}

	// An absent field (empty string) is not an error.
	zero, err := parseFeedTime(lastTradedTimeLayout, "")
	if err != nil || !zero.IsZero() {
		t.Errorf("parseFeedTime(\"\") = %v, %v; want zero time, nil error", zero, err)
	}
}

func containsExact(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func sameStringSlice(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]bool{}
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			return false
		}
	}
	return true
}
