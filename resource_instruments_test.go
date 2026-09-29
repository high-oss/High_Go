// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	highopenapi "github.com/high-oss/High_Go"
)

// The manifest endpoint is an internal implementation detail (the plan this
// was built from is explicit: "no public manifest() method"). Instruments
// exposes exactly the streaming and eager forms, nothing that hints at how
// the download URL is resolved.
func TestInstrumentsExposesExactlyStreamAndList(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{}))
	})
	defer ts.Close()

	instrumentsType := reflect.TypeOf(clientFor(t, ts, nil).Instruments)
	methods := map[string]bool{}
	for i := 0; i < instrumentsType.NumMethod(); i++ {
		methods[instrumentsType.Method(i).Name] = true
	}
	if len(methods) != 2 || !methods["Stream"] || !methods["List"] {
		t.Fatalf("InstrumentsResource method set = %v, want exactly {Stream, List}", methods)
	}
}

// instrumentHeader is the 17-column CSV header, in the order the manifest
// declares (highopenapi.InstrumentsManifest's own Columns).
var instrumentHeader = []string{
	"exchange", "segment", "instrument", "high_trading_symbol", "scrip_key", "isin",
	"scrip_code", "symbol", "name", "group_series", "has_fno", "underlying_symbol",
	"expiry", "option_type", "strike_price", "price_tick", "lot_size",
}

const sampleCSV = `exchange,segment,instrument,high_trading_symbol,scrip_key,isin,scrip_code,symbol,name,group_series,has_fno,underlying_symbol,expiry,option_type,strike_price,price_tick,lot_size
NSE,ES,EQUITY,RELIANCE-EQ,NSE@2885,INE002A01018,2885,RELIANCE,"Reliance, Industries Ltd",EQ,1,RELIANCE,,,,5,1
NSE,FO,OPTSTK,RELIANCE26SEP1400CE,NSE@55321,,55321,RELIANCE,Reliance Industries Ltd Sep Options,,0,RELIANCE,2026-09-30,CE,1400.5,5,250
`

// manifestFor builds the {requestId, data} envelope for a manifest whose
// "equity" entry points at csvURL; the other four categories point at a URL
// nothing in these tests ever dereferences, since only "equity" is fetched.
func manifestFor(csvURL string) map[string]any {
	file := func(instrument, url string) map[string]any {
		return map[string]any{
			"instrument": instrument, "url": url, "bytes": len(sampleCSV), "rows": 2,
			"checksum": "d41d8cd98f00b204e9800998ecf8427e", "updatedAt": "2026-09-29T02:53:09.000Z",
		}
	}
	return map[string]any{
		"generatedAt": "2026-09-29T02:53:10.000Z",
		"columns":     instrumentHeader,
		"files": []any{
			file("all", csvURL+"/unused-all.csv"),
			file("equity", csvURL+"/equity.csv"),
			file("derivatives", csvURL+"/unused-derivatives.csv"),
			file("commodity", csvURL+"/unused-commodity.csv"),
			file("etfs", csvURL+"/unused-etfs.csv"),
		},
	}
}

func serveCSV(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.WriteHeader(200)
	fmt.Fprint(w, body)
}

// TestInstrumentsListParsesEveryColumn is the baseline: a manifest server and
// a CSV server, both real local httptest servers, wired together exactly as
// production wires the gateway to the CDN.
func TestInstrumentsListParsesEveryColumn(t *testing.T) {
	csvServer := highopenapi.StartTLSTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { serveCSV(w, sampleCSV) })
	defer csvServer.Close()

	manifest := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", manifestFor(csvServer.URL)))
	})
	defer manifest.Close()

	client := clientFor(t, manifest, func(o *highopenapi.Options) {
		o.InstrumentsAllowedHosts = []string{hostOf(t, csvServer.URL)}
		o.HTTPClient = csvServer.Client()
	})

	rows, err := client.Instruments.List(context.Background(), highopenapi.InstrumentEquity)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d: %+v", len(rows), rows)
	}

	equity := rows[0]
	if equity.Exchange != "NSE" || equity.TradingSymbol != "RELIANCE-EQ" || equity.ScripCode != 2885 {
		t.Fatalf("equity row = %+v", equity)
	}
	if equity.Name != "Reliance, Industries Ltd" {
		t.Fatalf("quoted name column with an embedded comma was mis-split: %q", equity.Name)
	}
	if equity.HasFnO != 1 || equity.PriceTick != 5 || equity.LotSize != 1 {
		t.Fatalf("equity int columns = %+v", equity)
	}
	if equity.StrikePrice != nil {
		t.Fatalf("equity StrikePrice should be nil (blank cell), got %v", *equity.StrikePrice)
	}
	if equity.Expiry != "" || equity.OptionType != "" {
		t.Fatalf("equity Expiry/OptionType should be blank, got %q/%q", equity.Expiry, equity.OptionType)
	}

	option := rows[1]
	if option.StrikePrice == nil || *option.StrikePrice != 1400.5 {
		t.Fatalf("option StrikePrice = %v, want 1400.5", option.StrikePrice)
	}
	if option.Expiry != "2026-09-30" || option.OptionType != "CE" {
		t.Fatalf("option Expiry/OptionType = %q/%q", option.Expiry, option.OptionType)
	}
	if option.ISIN != "" {
		t.Fatalf("option ISIN should be blank, got %q", option.ISIN)
	}
}

// The manifest call and the CSV download must both go out with no
// credentials at all — the CSV host is a third-party CDN, and a bearer token
// or API key reaching it would leak the caller's credential.
func TestInstrumentsAttachesNoCredentialsToEitherRequest(t *testing.T) {
	csvServer := highopenapi.StartTLSTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { serveCSV(w, sampleCSV) })
	defer csvServer.Close()

	manifest := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", manifestFor(csvServer.URL)))
	})
	defer manifest.Close()

	client := clientFor(t, manifest, func(o *highopenapi.Options) {
		o.AccessToken = "SECRET-TOKEN"
		o.APIKey = "SECRET-KEY"
		o.InstrumentsAllowedHosts = []string{hostOf(t, csvServer.URL)}
		o.HTTPClient = csvServer.Client()
	})

	if _, err := client.Instruments.List(context.Background(), highopenapi.InstrumentEquity); err != nil {
		t.Fatal(err)
	}

	manifestReq := manifest.Req(0)
	if manifestReq.Header.Get("Authorization") != "" || manifestReq.Header.Get("x-api-key") != "" {
		t.Fatalf("manifest request carried credentials: %v", manifestReq.Header)
	}
	csvReq := csvServer.Req(0)
	if csvReq.Header.Get("Authorization") != "" || csvReq.Header.Get("x-api-key") != "" {
		t.Fatalf("CSV download carried credentials: %v", csvReq.Header)
	}
}

// The manifest is cached for the Client's lifetime: a second List/Stream call
// for a different category must not hit the manifest endpoint again.
func TestInstrumentsCachesTheManifestAcrossCalls(t *testing.T) {
	csvServer := highopenapi.StartTLSTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { serveCSV(w, sampleCSV) })
	defer csvServer.Close()

	manifest := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", manifestFor(csvServer.URL)))
	})
	defer manifest.Close()

	client := clientFor(t, manifest, func(o *highopenapi.Options) {
		o.InstrumentsAllowedHosts = []string{hostOf(t, csvServer.URL)}
		o.HTTPClient = csvServer.Client()
	})

	if _, err := client.Instruments.List(context.Background(), highopenapi.InstrumentEquity); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Instruments.List(context.Background(), highopenapi.InstrumentEquity); err != nil {
		t.Fatal(err)
	}
	if manifest.Count() != 1 {
		t.Fatalf("expected the manifest endpoint to be hit exactly once, got %d", manifest.Count())
	}
	if csvServer.Count() != 2 {
		t.Fatalf("expected the CSV to be downloaded twice (once per call), got %d", csvServer.Count())
	}
}

// Review Focus 1: a category the manifest does not list is a clear error.
func TestInstrumentsRequestingAnUnlistedCategoryIsAClearError(t *testing.T) {
	manifest := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		data := manifestFor("http://unused.invalid")
		data["files"] = []any{} // no categories published at all
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", data))
	})
	defer manifest.Close()

	client := clientFor(t, manifest, nil)
	_, err := client.Instruments.List(context.Background(), highopenapi.InstrumentEquity)
	if err == nil || !strings.Contains(err.Error(), "equity") {
		t.Fatalf("expected an error naming the missing category, got %v", err)
	}
}

// Review Focus 1 (other half): a manifest entry naming a category this SDK
// does not know must not crash — it is simply never matched.
func TestInstrumentsIgnoresACategoryTheManifestListsThatTheSDKDoesNotKnow(t *testing.T) {
	csvServer := highopenapi.StartTLSTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { serveCSV(w, sampleCSV) })
	defer csvServer.Close()

	manifest := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		data := manifestFor(csvServer.URL)
		data["files"] = append(data["files"].([]any), map[string]any{
			"instrument": "bonds", "url": "https://unknown.example/bonds.csv", "bytes": 1, "rows": 1,
			"checksum": "x", "updatedAt": "2026-09-29T02:53:09.000Z",
		})
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", data))
	})
	defer manifest.Close()

	client := clientFor(t, manifest, func(o *highopenapi.Options) {
		o.InstrumentsAllowedHosts = []string{hostOf(t, csvServer.URL)}
		o.HTTPClient = csvServer.Client()
	})
	if _, err := client.Instruments.List(context.Background(), highopenapi.InstrumentEquity); err != nil {
		t.Fatalf("an unrelated unknown category must not break a known one: %v", err)
	}
}

// Review Focus 2: a CSV header that does not match the manifest's columns
// must fail loudly, never silently shift fields.
func TestInstrumentsCSVHeaderMismatchIsAClearError(t *testing.T) {
	csvServer := highopenapi.StartTLSTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		serveCSV(w, "exchange,segment,symbol\nNSE,ES,RELIANCE\n") // missing 14 columns
	})
	defer csvServer.Close()

	manifest := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", manifestFor(csvServer.URL)))
	})
	defer manifest.Close()

	client := clientFor(t, manifest, func(o *highopenapi.Options) {
		o.InstrumentsAllowedHosts = []string{hostOf(t, csvServer.URL)}
		o.HTTPClient = csvServer.Client()
	})
	_, err := client.Instruments.List(context.Background(), highopenapi.InstrumentEquity)
	var apiErr *highopenapi.APIError
	if err == nil || !errors.As(err, &apiErr) {
		t.Fatalf("expected a *APIError, got %v", err)
	}
	if !strings.Contains(err.Error(), "header") {
		t.Fatalf("expected the error to mention the header mismatch, got %v", err)
	}
	if csvServer.Count() != 1 {
		t.Fatalf("expected exactly one CSV request, got %d", csvServer.Count())
	}
}

// Review Focus 3 (first half): a non-https manifest URL is refused before
// any request for it is made.
func TestInstrumentsRefusesANonHTTPSDownloadURL(t *testing.T) {
	csvServer := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { serveCSV(w, sampleCSV) })
	defer csvServer.Close()

	manifest := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		data := manifestFor(csvServer.URL) // csvServer.URL is http://, not https
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", data))
	})
	defer manifest.Close()

	client := clientFor(t, manifest, func(o *highopenapi.Options) {
		o.InstrumentsAllowedHosts = []string{hostOf(t, csvServer.URL)}
	})
	_, err := client.Instruments.List(context.Background(), highopenapi.InstrumentEquity)
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("expected a scheme error, got %v", err)
	}
	if csvServer.Count() != 0 {
		t.Fatalf("expected no request to the CSV host, got %d", csvServer.Count())
	}
}

// Review Focus 3 (second half): a URL whose host is not on the allowlist is
// refused before any request for it is made, even though it is https.
func TestInstrumentsRefusesADownloadURLOutsideTheAllowlist(t *testing.T) {
	manifest := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		data := manifestFor("https://not-the-allowed-host.example")
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", data))
	})
	defer manifest.Close()

	// Deliberately not setting InstrumentsAllowedHosts: the default allowlist
	// is the real CDN host, which "not-the-allowed-host.example" is not.
	client := clientFor(t, manifest, nil)
	_, err := client.Instruments.List(context.Background(), highopenapi.InstrumentEquity)
	if err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("expected an allowlist error, got %v", err)
	}
}

// Breaking out of Stream early must close the response body — proven here by
// having the CSV handler block until it observes its request context end,
// which only happens once the client gives up the connection.
func TestInstrumentsStreamClosesTheBodyOnEarlyBreak(t *testing.T) {
	serverSawDisconnect := make(chan struct{})
	csvServer := highopenapi.StartTLSTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(200)
		fmt.Fprint(w, strings.Join(instrumentHeader, ",")+"\n")
		fmt.Fprint(w, "NSE,ES,EQUITY,RELIANCE-EQ,NSE@2885,INE002A01018,2885,RELIANCE,Reliance,EQ,1,RELIANCE,,,,5,1\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
		close(serverSawDisconnect)
	})
	defer csvServer.Close()

	manifest := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", manifestFor(csvServer.URL)))
	})
	defer manifest.Close()

	client := clientFor(t, manifest, func(o *highopenapi.Options) {
		o.InstrumentsAllowedHosts = []string{hostOf(t, csvServer.URL)}
		o.HTTPClient = csvServer.Client()
	})

	rows := 0
	for _, err := range client.Instruments.Stream(context.Background(), highopenapi.InstrumentEquity) {
		if err != nil {
			t.Fatal(err)
		}
		rows++
		break
	}
	if rows != 1 {
		t.Fatalf("expected to read exactly 1 row before breaking, got %d", rows)
	}

	select {
	case <-serverSawDisconnect:
	case <-time.After(2 * time.Second):
		t.Fatal("server never observed the client disconnect — the response body was not closed on early break")
	}
}

// The caller's context cancels an in-flight download, same as every other operation.
func TestInstrumentsContextCancelsAnInFlightDownload(t *testing.T) {
	started := make(chan struct{})
	csvServer := highopenapi.StartTLSTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(200)
		fmt.Fprint(w, strings.Join(instrumentHeader, ",")+"\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		close(started)
		<-r.Context().Done()
	})
	defer csvServer.Close()

	manifest := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", manifestFor(csvServer.URL)))
	})
	defer manifest.Close()

	client := clientFor(t, manifest, func(o *highopenapi.Options) {
		o.InstrumentsAllowedHosts = []string{hostOf(t, csvServer.URL)}
		o.HTTPClient = csvServer.Client()
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		for _, err := range client.Instruments.Stream(ctx, highopenapi.InstrumentEquity) {
			if err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()

	<-started
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected the cancellation to surface as an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stream never returned after the context was cancelled")
	}
}

// The CSV download is bounded only by the caller's context, not by
// TimeoutMs: TimeoutMs still governs the manifest lookup (a JSON call like
// any other), but clamping a 14MB/11MB file to the same per-request budget
// would make the documented 30s default actively hostile to the two largest
// files. A very small TimeoutMs would fail the manifest call outright if it
// applied there too widely, so this proves the download survives a CSV
// handler slower than TimeoutMs while the manifest itself responds quickly.
func TestInstrumentsDownloadIsBoundedByContextNotByTimeoutMs(t *testing.T) {
	csvServer := highopenapi.StartTLSTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		time.Sleep(150 * time.Millisecond) // longer than the 50ms TimeoutMs below
		serveCSV(w, sampleCSV)
	})
	defer csvServer.Close()

	manifest := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", manifestFor(csvServer.URL)))
	})
	defer manifest.Close()

	client := clientFor(t, manifest, func(o *highopenapi.Options) {
		o.TimeoutMs = highopenapi.Ptr(50)
		o.InstrumentsAllowedHosts = []string{hostOf(t, csvServer.URL)}
		o.HTTPClient = csvServer.Client()
	})

	rows, err := client.Instruments.List(context.Background(), highopenapi.InstrumentEquity)
	if err != nil {
		t.Fatalf("the CSV download must not inherit TimeoutMs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

// hostOf extracts just the hostname (no port) the same way
// validateInstrumentURL compares it, so tests can allowlist a local
// httptest.Server by its ephemeral host:port URL.
func hostOf(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Hostname()
}
