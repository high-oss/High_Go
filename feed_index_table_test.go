// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// sourceIndexFile mirrors index-feed-map.json's shape: an "indices" array of
// unambiguous rows, and an "ambiguous" array of scripKeys the scrip master
// genuinely shares between two different indices (one token, two index
// names -- a fact about the scrip master, not a bad join).
type sourceIndexFile struct {
	Indices []struct {
		ScripKey    string `json:"scripKey"`
		FeedSegment string `json:"feedSegment"`
		FeedSymbol  string `json:"feedSymbol"`
		Name        string `json:"name"`
	} `json:"indices"`
	Ambiguous []struct {
		ScripKey   string   `json:"scripKey"`
		Candidates []string `json:"candidates"`
	} `json:"ambiguous"`
}

// deliberatelyAbsentIndexKeys are the four index rows with no feed
// counterpart at all (datafeed plan, Phase 1 table) -- present in neither of
// index-feed-map.json's arrays, so they cannot be rebuilt from it; this test
// only checks that feed_index_table.go's indexFeedAbsent still carries all
// four, same as it checks indexFeedAmbiguous against the source.
var deliberatelyAbsentIndexKeys = []string{"NSE@26016", "BSE@19058", "BSE@19081", "BSE@19018"}

// TestIndexTableMatchesSource rebuilds indexFeedMap and indexFeedAmbiguous
// from the committed copy of index-feed-map.json -- independently of
// feed_index_table.go, which was generated from the same source once and
// committed (datafeed plan, Phase 1: "each SDK ships a test that rebuilds
// the join from the committed source and asserts the generated table still
// matches"). A new, changed, or newly-ambiguous index upstream then shows up
// here as a failing test rather than a silent gap between the source and
// the shipped table.
func TestIndexTableMatchesSource(t *testing.T) {
	data, err := os.ReadFile("feed_index_table_source.json")
	if err != nil {
		t.Fatal(err)
	}
	var src sourceIndexFile
	if err := json.Unmarshal(data, &src); err != nil {
		t.Fatal(err)
	}
	if len(src.Indices) != 81 {
		t.Fatalf("source has %d unambiguous indices, want 81", len(src.Indices))
	}
	if len(src.Ambiguous) != 6 {
		t.Fatalf("source has %d ambiguous entries, want 6", len(src.Ambiguous))
	}

	wantMap := map[string]indexFeedEntry{}
	for _, r := range src.Indices {
		wantMap[r.ScripKey] = indexFeedEntry{feedSegment: r.FeedSegment, feedSymbol: r.FeedSymbol, name: r.Name}
	}
	if !reflect.DeepEqual(indexFeedMap, wantMap) {
		t.Errorf("indexFeedMap does not match a fresh rebuild from feed_index_table_source.json's \"indices\" array — "+
			"regenerate feed_index_table.go from index-feed-map.json (got %d entries, want %d)",
			len(indexFeedMap), len(wantMap))
	}

	wantAmbiguous := map[string][]string{}
	for _, r := range src.Ambiguous {
		wantAmbiguous[r.ScripKey] = r.Candidates
	}
	if len(indexFeedAmbiguous) != len(wantAmbiguous) {
		t.Errorf("indexFeedAmbiguous has %d entries, want %d", len(indexFeedAmbiguous), len(wantAmbiguous))
	}
	for key, wantCandidates := range wantAmbiguous {
		got, ok := indexFeedAmbiguous[key]
		if !ok {
			t.Errorf("indexFeedAmbiguous is missing %q, found ambiguous in the source", key)
			continue
		}
		if !sameStringSlice(got, wantCandidates) {
			t.Errorf("indexFeedAmbiguous[%q] = %v, want %v", key, got, wantCandidates)
		}
	}

	// A key must never appear in more than one of the three tables.
	for key := range indexFeedMap {
		if _, ok := indexFeedAmbiguous[key]; ok {
			t.Errorf("%q is in both indexFeedMap and indexFeedAmbiguous", key)
		}
		if _, ok := indexFeedAbsent[key]; ok {
			t.Errorf("%q is in both indexFeedMap and indexFeedAbsent", key)
		}
	}
	for key := range indexFeedAmbiguous {
		if _, ok := indexFeedAbsent[key]; ok {
			t.Errorf("%q is in both indexFeedAmbiguous and indexFeedAbsent", key)
		}
	}

	for _, key := range deliberatelyAbsentIndexKeys {
		if _, ok := indexFeedAbsent[key]; !ok {
			t.Errorf("indexFeedAbsent is missing the deliberately-absent key %q", key)
		}
		if _, ok := wantMap[key]; ok {
			t.Errorf("%q was expected to be absent from the source's \"indices\" array but is present", key)
		}
	}
	if len(indexFeedAbsent) != len(deliberatelyAbsentIndexKeys) {
		t.Errorf("indexFeedAbsent has %d entries, want exactly the %d deliberately-absent ones", len(indexFeedAbsent), len(deliberatelyAbsentIndexKeys))
	}
}
