// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"fmt"
	"strings"
)

// feedSegmentByPrefix is the closed set of HIGH scrip-key prefixes the feed
// covers, mapped to the feed's own segment code (datafeed plan, Phase 1).
// MCX@ (MCX spot) is deliberately absent: the feed has no known code for it,
// and it is rejected explicitly in translateNonIndexKey rather than falling
// through to some guessed segment.
var feedSegmentByPrefix = map[string]string{
	"NSE@":   "nse_cm",
	"BSE@":   "bse_cm",
	"NSEFO@": "nse_fo",
	"BSEFO@": "bse_fo",
	"MCXFO@": "mcx_fo",
}

const supportedPrefixes = "NSE@, BSE@, NSEFO@, BSEFO@, MCXFO@"

// translatedFeedKey is a HIGH scrip key translated into the feed's own
// identifier. Internal only -- wireScrip's output is what goes on the wire;
// it never reaches a caller.
type translatedFeedKey struct {
	scripKey    string
	feedSegment string
	feedID      string // a token for a non-index key, an index's feed symbol otherwise
}

// wireScrip is what a subscribe/unsubscribe/snapshot request sends on the
// wire for one instrument: "<feedSegment>|<feedID>".
func (k translatedFeedKey) wireScrip() string {
	return k.feedSegment + "|" + k.feedID
}

// indexLookupResult says how a key relates to the committed index table
// (feed_index_table.go) -- see lookupIndex.
type indexLookupResult int

const (
	// indexNotFound: key is not in any of the three tables below at all.
	indexNotFound indexLookupResult = iota
	// indexFound: a clean, single-valued mapping (indexFeedMap).
	indexFound
	// indexAmbiguous: the scrip master genuinely shares this scripKey
	// between two different indices (indexFeedAmbiguous) -- a fact about the
	// scrip master, not a bad join. Resolving it by picking either candidate
	// would silently stream a plausible-looking but potentially wrong index
	// under the caller's key, so it is refused instead.
	indexAmbiguous
	// indexAbsent: a key this SDK recognises as an index but the feed has no
	// counterpart for at all (indexFeedAbsent).
	indexAbsent
)

// lookupIndex reports how key relates to the committed index table. entry is
// only valid when the result is indexFound; candidates only when
// indexAmbiguous; reason only when indexAbsent.
func lookupIndex(key string) (result indexLookupResult, entry indexFeedEntry, candidates []string, reason string) {
	if e, found := indexFeedMap[key]; found {
		return indexFound, e, nil, ""
	}
	if c, found := indexFeedAmbiguous[key]; found {
		return indexAmbiguous, indexFeedEntry{}, c, ""
	}
	if r, found := indexFeedAbsent[key]; found {
		return indexAbsent, indexFeedEntry{}, nil, r
	}
	return indexNotFound, indexFeedEntry{}, nil, ""
}

// translateIndexKey resolves key for SubscribeIndices, UnsubscribeIndices and
// SnapshotIndices. Only a key in indexFeedMap succeeds. Everything else --
// including a key this SDK recognises as an index but cannot translate
// (because the feed has no counterpart, or because it genuinely names more
// than one index), and a key that is not an index at all -- is a named
// error, never a silent fallback or a coin-flip pick.
func translateIndexKey(key string) (translatedFeedKey, error) {
	switch result, entry, candidates, reason := lookupIndex(key); result {
	case indexFound:
		return translatedFeedKey{scripKey: key, feedSegment: entry.feedSegment, feedID: entry.feedSymbol}, nil
	case indexAmbiguous:
		return translatedFeedKey{}, fmt.Errorf(
			"datafeed: %q resolves to more than one index (%s); subscribe using the specific feed symbol you mean, once the ambiguity is resolved upstream",
			key, strings.Join(candidates, ", "))
	case indexAbsent:
		return translatedFeedKey{}, fmt.Errorf("datafeed: index %q has no feed counterpart: %s", key, reason)
	default:
		return translatedFeedKey{}, fmt.Errorf(
			"datafeed: %q is not a known index (not in the committed index table); use SubscribeQuotes or SubscribeDepth for a non-index scrip", key)
	}
}

// translateNonIndexKey resolves key for SubscribeQuotes and SubscribeDepth
// (and their Unsubscribe/Snapshot counterparts).
//
// An index key is always rejected here -- even one this SDK cannot itself
// translate -- rather than falling through to the prefix rule below.
// Indices share the NSE@/BSE@ prefix with cash scrips, so without this check
// an index key would derive a feed identifier from its numeric token (e.g.
// "nse_cm|26000"), which the feed does not recognise for an index and which
// would therefore simply never tick: a silent failure indistinguishable from
// a dead instrument. See lookupIndex.
func translateNonIndexKey(key string) (translatedFeedKey, error) {
	if result, _, _, _ := lookupIndex(key); result != indexNotFound {
		return translatedFeedKey{}, fmt.Errorf(
			"datafeed: %q is an index; use SubscribeIndices, UnsubscribeIndices or SnapshotIndices instead", key)
	}

	at := strings.IndexByte(key, '@')
	if at < 0 {
		return translatedFeedKey{}, fmt.Errorf(
			"datafeed: %q does not look like a HIGH scrip key (no '@'); supported prefixes: %s", key, supportedPrefixes)
	}
	prefix, id := key[:at+1], key[at+1:]
	if id == "" {
		return translatedFeedKey{}, fmt.Errorf("datafeed: %q has no identifier after %q", key, prefix)
	}

	segment, ok := feedSegmentByPrefix[prefix]
	if !ok {
		if prefix == "MCX@" {
			return translatedFeedKey{}, fmt.Errorf(
				"datafeed: %q is an MCX spot scrip; the feed has no known code for MCX spot (supported prefixes: %s)", key, supportedPrefixes)
		}
		return translatedFeedKey{}, fmt.Errorf(
			"datafeed: %q has an unsupported prefix %q (supported prefixes: %s)", key, prefix, supportedPrefixes)
	}
	return translatedFeedKey{scripKey: key, feedSegment: segment, feedID: id}, nil
}
