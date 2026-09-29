// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"strings"
	"testing"
)

func TestTranslateNonIndexKeyKnownPrefixes(t *testing.T) {
	cases := []struct {
		key       string
		wireScrip string
	}{
		{"NSE@15563", "nse_cm|15563"},
		{"BSE@532540", "bse_cm|532540"},
		{"NSEFO@52190", "nse_fo|52190"},
		{"BSEFO@842150", "bse_fo|842150"},
		{"MCXFO@227915", "mcx_fo|227915"},
	}
	for _, c := range cases {
		got, err := translateNonIndexKey(c.key)
		if err != nil {
			t.Fatalf("translateNonIndexKey(%q): %v", c.key, err)
		}
		if got.wireScrip() != c.wireScrip {
			t.Errorf("translateNonIndexKey(%q).wireScrip() = %q, want %q", c.key, got.wireScrip(), c.wireScrip)
		}
	}
}

// MCX spot has no known feed code (datafeed plan, Phase 1 table) -- it must
// be rejected explicitly, not passed through or guessed at.
func TestTranslateNonIndexKeyRejectsMCXSpot(t *testing.T) {
	_, err := translateNonIndexKey("MCX@226490")
	if err == nil {
		t.Fatal("expected an error for MCX spot")
	}
	if !strings.Contains(err.Error(), "MCX@226490") || !strings.Contains(err.Error(), "MCX spot") {
		t.Errorf("error = %q, want it to name the key and say MCX spot", err.Error())
	}
}

func TestTranslateNonIndexKeyRejectsUnknownPrefix(t *testing.T) {
	_, err := translateNonIndexKey("NSECDS@123")
	if err == nil {
		t.Fatal("expected an error for an unsupported prefix")
	}
	if !strings.Contains(err.Error(), "NSECDS@123") {
		t.Errorf("error = %q, want it to name the key", err.Error())
	}
}

func TestTranslateNonIndexKeyRejectsMalformedKey(t *testing.T) {
	for _, key := range []string{"garbage", "NSE@", "@123"} {
		if _, err := translateNonIndexKey(key); err == nil {
			t.Errorf("translateNonIndexKey(%q): expected an error", key)
		}
	}
}

// Indices share the NSE@/BSE@ prefix with cash scrips. SubscribeQuotes and
// SubscribeDepth must refuse an index key rather than silently deriving a
// (wrong) equity-token feed identifier from it -- that would be accepted by
// the server and then simply never tick.
func TestTranslateNonIndexKeyRejectsIndexKey(t *testing.T) {
	_, err := translateNonIndexKey("NSE@26000") // Nifty 50, in indexFeedMap
	if err == nil {
		t.Fatal("expected an error for an index key")
	}
	if !strings.Contains(err.Error(), "NSE@26000") || !strings.Contains(err.Error(), "SubscribeIndices") {
		t.Errorf("error = %q, want it to name the key and point at SubscribeIndices", err.Error())
	}
}

// The same rejection must apply to an index key this SDK cannot itself
// translate (indexFeedExcluded) -- it is still recognised as an index, so it
// must not fall through to the prefix rule and be treated as an equity.
func TestTranslateNonIndexKeyRejectsExcludedIndexKey(t *testing.T) {
	_, err := translateNonIndexKey("NSE@26016") // HANGSENG BEES-NAV, deliberately excluded
	if err == nil {
		t.Fatal("expected an error for an excluded index key")
	}
	if !strings.Contains(err.Error(), "SubscribeIndices") {
		t.Errorf("error = %q, want it to point at SubscribeIndices", err.Error())
	}
}

func TestTranslateIndexKeySuccess(t *testing.T) {
	got, err := translateIndexKey("NSE@26000")
	if err != nil {
		t.Fatal(err)
	}
	if got.wireScrip() != "nse_cm|Nifty 50" {
		t.Errorf("wireScrip = %q, want nse_cm|Nifty 50", got.wireScrip())
	}

	got, err = translateIndexKey("BSE@19000")
	if err != nil {
		t.Fatal(err)
	}
	if got.wireScrip() != "bse_cm|SENSEX" {
		t.Errorf("wireScrip = %q, want bse_cm|SENSEX", got.wireScrip())
	}
}

// The mirror image of TestTranslateNonIndexKeyRejectsIndexKey: SubscribeIndices
// must refuse an ordinary equity key.
func TestTranslateIndexKeyRejectsNonIndex(t *testing.T) {
	_, err := translateIndexKey("NSE@2885") // RELIANCE-EQ, an ordinary equity
	if err == nil {
		t.Fatal("expected an error for a non-index key")
	}
	if !strings.Contains(err.Error(), "NSE@2885") || !strings.Contains(err.Error(), "not a known index") {
		t.Errorf("error = %q, want it to name the key and say it is not a known index", err.Error())
	}
}

func TestTranslateIndexKeyRejectsDeliberatelyExcluded(t *testing.T) {
	_, err := translateIndexKey("BSE@19018") // S&P BSE DOLLEX 30
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "DOLLEX") {
		t.Errorf("error = %q, want the DOLLEX 30 reason", err.Error())
	}
}

// NSE@26002 is a real case: the scrip master shares this one token between
// two different indices (Nifty FMCG and Nifty50 PR 2x Lev). Picking either
// on a hunch would silently stream a plausible-looking but potentially wrong
// index under the caller's key, so this must fail with a clear error naming
// both candidates rather than resolving it by a coin-flip.
func TestTranslateIndexKeyRejectsAmbiguous(t *testing.T) {
	_, err := translateIndexKey("NSE@26002")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "more than one index") {
		t.Errorf("error = %q, want it to say the key resolves to more than one index", err.Error())
	}
	for _, candidate := range []string{"Nifty FMCG", "Nifty50 PR 2x Lev"} {
		if !strings.Contains(err.Error(), candidate) {
			t.Errorf("error = %q, want it to name candidate %q", err.Error(), candidate)
		}
	}
}

// SubscribeQuotes/SubscribeDepth must refuse an ambiguous index key exactly
// as they refuse any other index key -- pointing at the index methods, where
// the ambiguity itself (and both candidates) is then what the caller sees.
func TestTranslateNonIndexKeyRejectsAmbiguousIndexKey(t *testing.T) {
	_, err := translateNonIndexKey("NSE@26002")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "SubscribeIndices") {
		t.Errorf("error = %q, want it to point at SubscribeIndices", err.Error())
	}
}
