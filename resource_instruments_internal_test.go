// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// A manifest request failure must not take the instrument list down with
// it: Instruments falls back to lastKnownInstrumentURLs (baked in at build
// time) and logs a warning, per the shared SDK plan's Phase 3 resolution
// rule. lastKnownInstrumentURLs points at the real production CDN, which a
// test must not dial — so, for the duration of this one test, it is
// swapped for a local httptest server and restored afterwards. Production
// code never has a way to do this: the map is unexported and nothing in the
// public API mutates it.
func TestInstrumentsFallsBackToTheBakedInURLsWhenTheManifestRequestFails(t *testing.T) {
	// The fallback URLs are https (they point at the real CDN), so the local
	// stand-in must be too, or validateInstrumentURL would (correctly) refuse
	// it for the wrong reason. httptest.NewTLSServer gives a real TLS
	// exchange with a self-signed cert; ts.Client() below is pre-configured
	// to trust it.
	csvServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serveCSVInternal(w) }))
	defer csvServer.Close()

	original := lastKnownInstrumentURLs
	lastKnownInstrumentURLs = map[InstrumentCategory]string{
		InstrumentAll:         csvServer.URL + "/all.csv",
		InstrumentEquity:      csvServer.URL + "/equity.csv",
		InstrumentDerivatives: csvServer.URL + "/derivatives.csv",
		InstrumentCommodity:   csvServer.URL + "/commodity.csv",
		InstrumentEtfs:        csvServer.URL + "/etfs.csv",
	}
	t.Cleanup(func() { lastKnownInstrumentURLs = original })

	manifestServer := StartTestServer(func(w http.ResponseWriter, r *http.Request, index int) {
		WriteJSON(w, 500, map[string]any{"code": "SERVICE_UNAVAILABLE", "message": "manifest not exported yet"})
	})
	defer manifestServer.Close()

	lines, sink := newCaptureSink()
	config := configFor(t, manifestServer, func(o *Options) {
		o.MaxRetries = Ptr(0)
		o.LogLevel = LogLevelWarn
		o.LogSink = sink
		o.HTTPClient = csvServer.Client()
		o.InstrumentsAllowedHosts = []string{hostOfInternal(t, csvServer.URL)}
	})
	resource := &InstrumentsResource{config: config}

	rows, err := resource.List(context.Background(), InstrumentEquity)
	if err != nil {
		t.Fatalf("expected the fallback URL to serve the list, got %v", err)
	}
	if len(rows) != 1 || rows[0].TradingSymbol != "RELIANCE-EQ" {
		t.Fatalf("rows = %+v", rows)
	}

	var warned bool
	for _, l := range *lines {
		if strings.Contains(strings.ToLower(l.message), "falling back") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("expected a warning about falling back, got %v", *lines)
	}
}

// Once the manifest has been fetched successfully, it is cached for the
// Client's lifetime — verified here at the resource level with a shared
// mutex, since a Client is used concurrently.
func TestInstrumentsGetManifestIsFetchedAtMostOnce(t *testing.T) {
	csvServer := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { serveCSVInternal(w) })
	defer csvServer.Close()

	manifestServer := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		WriteJSON(w, 200, Envelope("r", manifestForInternal(csvServer.URL)))
	})
	defer manifestServer.Close()

	config := configFor(t, manifestServer, func(o *Options) {
		o.InstrumentsAllowedHosts = []string{hostOfInternal(t, csvServer.URL)}
	})
	resource := &InstrumentsResource{config: config}

	if _, err := resource.getManifest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := resource.getManifest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manifestServer.Count() != 1 {
		t.Fatalf("expected exactly 1 manifest request, got %d", manifestServer.Count())
	}
}

func serveCSVInternal(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/csv")
	w.WriteHeader(200)
	w.Write([]byte(strings.Join(instrumentColumns, ",") + "\n" +
		"NSE,ES,EQUITY,RELIANCE-EQ,NSE@2885,INE002A01018,2885,RELIANCE,Reliance,EQ,1,RELIANCE,,,,5,1\n"))
}

func manifestForInternal(csvURL string) map[string]any {
	file := func(instrument, url string) map[string]any {
		return map[string]any{
			"instrument": instrument, "url": url, "bytes": 1, "rows": 1,
			"checksum": "d41d8cd98f00b204e9800998ecf8427e", "updatedAt": "2026-09-29T02:53:09.000Z",
		}
	}
	return map[string]any{
		"generatedAt": "2026-09-29T02:53:10.000Z",
		"columns":     instrumentColumns,
		"files": []any{
			file("all", csvURL+"/all.csv"),
			file("equity", csvURL+"/equity.csv"),
			file("derivatives", csvURL+"/derivatives.csv"),
			file("commodity", csvURL+"/commodity.csv"),
			file("etfs", csvURL+"/etfs.csv"),
		},
	}
}

func hostOfInternal(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Hostname()
}
