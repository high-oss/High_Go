// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/high-oss/High_Go/generated"
)

// InstrumentCategory selects which published instrument list to fetch.
// Aliased to the generated enum type rather than widened to a bare string —
// never widen a spec enum in a facade signature (contract §8; see ExpiryType
// in resource_scrips.go for the same ruling).
type InstrumentCategory = generated.InstrumentFileInstrument

// The five published categories, re-exported under names that read well at
// the call site. oapi-codegen names the underlying constants for their bare
// enum value (generated.Equity, not generated.InstrumentFileInstrumentEquity)
// because "equity" does not collide with any other enum in the spec; these
// aliases exist so callers do not have to know that.
const (
	InstrumentAll         = generated.All
	InstrumentEquity      = generated.Equity
	InstrumentDerivatives = generated.Derivatives
	InstrumentCommodity   = generated.Commodity
	InstrumentEtfs        = generated.Etfs
)

// instrumentCategoryOrder fixes an iteration order over the five categories,
// used wherever a stable (rather than Go's randomised map) order matters:
// building the last-known fallback manifest and listing "available
// categories" in an error message.
var instrumentCategoryOrder = []InstrumentCategory{
	InstrumentAll, InstrumentEquity, InstrumentDerivatives, InstrumentCommodity, InstrumentEtfs,
}

// defaultInstrumentsAllowedHost is the CDN host the instrument list's files
// are published under today. Options.InstrumentsAllowedHosts overrides this
// allowlist; a file whose host is not on it is refused before any request
// for it is made — the manifest names the download, but this package does
// not trust it blindly.
const defaultInstrumentsAllowedHost = "high-space.blr1.cdn.digitaloceanspaces.com"

// instrumentColumns is the instrument list's 17-column CSV header, in file
// order. It doubles as the columns used to build the last-known fallback
// manifest below — it has been stable since the instrument list shipped.
var instrumentColumns = []string{
	"exchange", "segment", "instrument", "high_trading_symbol", "scrip_key", "isin",
	"scrip_code", "symbol", "name", "group_series", "has_fno", "underlying_symbol",
	"expiry", "option_type", "strike_price", "price_tick", "lot_size",
}

// lastKnownInstrumentURLs is baked in at build time: if the manifest request
// itself fails (the endpoint is down, or the environment has never exported
// a manifest), Instruments falls back to these rather than taking the whole
// instrument list down with it. This is a var, not a const, purely so a test
// in this package can point it at a local server for the duration of one
// test; production callers have no way to reach it and never need to.
var lastKnownInstrumentURLs = map[InstrumentCategory]string{
	InstrumentAll:         "https://high-space.blr1.cdn.digitaloceanspaces.com/scrip-master/scrip-master.csv",
	InstrumentEquity:      "https://high-space.blr1.cdn.digitaloceanspaces.com/scrip-master/scrip-master-equity.csv",
	InstrumentDerivatives: "https://high-space.blr1.cdn.digitaloceanspaces.com/scrip-master/scrip-master-derivatives.csv",
	InstrumentCommodity:   "https://high-space.blr1.cdn.digitaloceanspaces.com/scrip-master/scrip-master-commodity.csv",
	InstrumentEtfs:        "https://high-space.blr1.cdn.digitaloceanspaces.com/scrip-master/scrip-master-etfs.csv",
}

// Instrument is one row of the instrument list CSV.
//
// Every field is the zero value when its cell is blank, except StrikePrice.
// ScripCode, HasFnO, PriceTick and LotSize never take a genuine zero in real
// data — no real broker token, lot size or price tick is 0, and HasFnO's own
// false state IS zero — so a blank cell and a real value can never be
// confused for these four, and a plain int is unambiguous. StrikePrice is
// different: every non-derivative row (and every future) has no strike at
// all, and unlike the int columns a caller reasonably might compute or
// compare a strike of exactly 0 in some downstream pricing logic, so leaving
// it a plain float64 would let a blank cell silently read as "strike price
// zero" instead of "not applicable". StrikePrice is therefore *float64: nil
// means the cell was blank.
//
// PriceTick is carried exactly as the CSV states it, unnormalised. Its scale
// (paise, rupees, or something else) varies by segment; rescaling it here
// would be a silent pricing bug baked into every caller.
type Instrument struct {
	Exchange string
	Segment  string
	// InstrumentType is the CSV's `instrument` column — a segment-derived
	// classification such as EQUITY, IDX, FUTSTK or OPTSTK. Named
	// differently from the struct field so it does not read like the
	// InstrumentCategory this row was fetched under.
	InstrumentType   string
	TradingSymbol    string
	ScripKey         string
	ISIN             string
	ScripCode        int
	Symbol           string
	Name             string
	GroupSeries      string
	HasFnO           int
	UnderlyingSymbol string
	Expiry           string
	OptionType       string
	// StrikePrice is nil when the cell was blank — see the type comment.
	StrikePrice *float64
	PriceTick   int
	LotSize     int
}

// InstrumentsResource covers the instrument list: the scrip master CSV,
// published per category. It needs no credentials, and neither do the files
// it downloads (contract-adjacent, but deliberately unauthenticated — see
// the README).
type InstrumentsResource struct {
	config *resolvedConfig

	mu       sync.Mutex
	manifest *generated.InstrumentsManifest // cached for the Client's lifetime once fetched
}

// List is the eager form: every row of category, materialised into a slice.
// The `all` and `derivatives` files are 14MB and 11MB; prefer Stream unless
// the caller genuinely needs every row in memory at once.
func (r *InstrumentsResource) List(ctx context.Context, category InstrumentCategory) ([]Instrument, error) {
	var out []Instrument
	for instrument, err := range r.Stream(ctx, category) {
		if err != nil {
			return nil, err
		}
		out = append(out, instrument)
	}
	return out, nil
}

// Stream is the primary form: a range-over-func iterator that reads the CSV
// as it downloads rather than buffering the whole file first.
//
//	for instrument, err := range high.Instruments.Stream(ctx, highopenapi.InstrumentEquity) {
//		if err != nil {
//			log.Fatal(err)
//		}
//		fmt.Println(instrument.TradingSymbol)
//	}
//
// Breaking out of the loop early closes the response body; nothing is
// leaked. The `all` and `derivatives` files are large enough that the
// default 30s TimeoutMs may not be enough on a slow connection — TimeoutMs
// governs the manifest lookup like every other operation, but the CSV
// download itself is bounded only by ctx, so pass one with a longer deadline
// for a slow link rather than raising the client-wide default:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
//	defer cancel()
//	rows, err := high.Instruments.List(ctx, highopenapi.InstrumentAll)
func (r *InstrumentsResource) Stream(ctx context.Context, category InstrumentCategory) iter.Seq2[Instrument, error] {
	return func(yield func(Instrument, error) bool) {
		file, columns, err := r.resolve(ctx, category)
		if err != nil {
			yield(Instrument{}, err)
			return
		}

		resp, err := r.download(ctx, file.Url)
		if err != nil {
			yield(Instrument{}, err)
			return
		}
		defer resp.Body.Close()

		reader := csv.NewReader(resp.Body)

		header, err := reader.Read()
		if err != nil {
			yield(Instrument{}, newPreflightError(fmt.Sprintf(
				"instrument list %q: failed to read the CSV header: %s", category, err.Error())))
			return
		}
		if !equalStrings(header, columns) {
			yield(Instrument{}, newPreflightError(fmt.Sprintf(
				"instrument list %q: CSV header %v does not match the manifest's columns %v — refusing to guess field positions",
				category, header, columns)))
			return
		}
		index := make(map[string]int, len(header))
		for i, name := range header {
			index[name] = i
		}

		for {
			record, err := reader.Read()
			if err == io.EOF {
				return
			}
			if err != nil {
				yield(Instrument{}, newTransportError(fmt.Sprintf("instrument list %q: failed reading a CSV row", category), err))
				return
			}
			row, err := parseInstrumentRow(index, record)
			if !yield(row, err) {
				return // caller broke; the deferred Close above runs on the way out
			}
			if err != nil {
				return
			}
		}
	}
}

// resolve fetches the manifest (or, on failure, the last-known fallback),
// finds the entry for category, and validates its URL before returning it —
// nothing is downloaded here.
func (r *InstrumentsResource) resolve(ctx context.Context, category InstrumentCategory) (generated.InstrumentFile, []string, error) {
	manifest, err := r.getManifest(ctx)
	if err != nil {
		return generated.InstrumentFile{}, nil, err
	}

	// A manifest naming a category this SDK does not recognise is simply
	// never matched below and never seen — it is not an error. Only a
	// requested category the manifest does not list is one.
	for _, f := range manifest.Files {
		if f.Instrument == category {
			if err := validateInstrumentURL(f.Url, r.config.instrumentsAllowedHosts); err != nil {
				return generated.InstrumentFile{}, nil, err
			}
			return f, manifest.Columns, nil
		}
	}

	available := make([]string, 0, len(manifest.Files))
	for _, f := range manifest.Files {
		available = append(available, string(f.Instrument))
	}
	return generated.InstrumentFile{}, nil, newPreflightError(fmt.Sprintf(
		"instrument list: %q is not one of the published categories (%s)", category, strings.Join(available, ", ")))
}

// getManifest returns the cached manifest, fetching it at most once per
// Client. A Client is used concurrently, so the cache is guarded by a mutex.
func (r *InstrumentsResource) getManifest(ctx context.Context) (generated.InstrumentsManifest, error) {
	r.mu.Lock()
	cached := r.manifest
	r.mu.Unlock()
	if cached != nil {
		return *cached, nil
	}

	// security: [] in the spec — this is deliberately authNone, not
	// authBearer: the manifest is public, and attaching a bearer token or API
	// key to it (or to the CSV it points at) would be sending a credential
	// somewhere it was never asked for.
	manifest, err := doRequest[generated.InstrumentsManifest](ctx, r.config, requestOptions{
		method: "GET", path: "/instruments", auth: authNone,
	})
	if err != nil {
		r.config.logger.Warn(fmt.Sprintf(
			"instrument list: manifest request failed, falling back to the URLs baked in at build time: %s", err.Error()), nil)
		return fallbackManifest(), nil
	}

	r.mu.Lock()
	r.manifest = &manifest
	r.mu.Unlock()
	return manifest, nil
}

// fallbackManifest reconstructs a manifest from lastKnownInstrumentURLs, used
// only when the manifest endpoint itself could not be reached.
func fallbackManifest() generated.InstrumentsManifest {
	files := make([]generated.InstrumentFile, 0, len(instrumentCategoryOrder))
	for _, category := range instrumentCategoryOrder {
		files = append(files, generated.InstrumentFile{Instrument: category, Url: lastKnownInstrumentURLs[category]})
	}
	return generated.InstrumentsManifest{Columns: instrumentColumns, Files: files}
}

// validateInstrumentURL refuses a download URL before any request for it is
// made: it must be https, and its host must be on the configured allowlist.
// The manifest names the download, but it points at a third-party CDN, and
// this package does not trust that pointer blindly.
func validateInstrumentURL(rawURL string, allowedHosts []string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return newPreflightError(fmt.Sprintf("instrument list: manifest URL %q does not parse: %s", rawURL, err.Error()))
	}
	if parsed.Scheme != "https" {
		return newPreflightError(fmt.Sprintf("instrument list: refusing a non-https manifest URL %q", rawURL))
	}
	host := strings.ToLower(parsed.Hostname())
	for _, allowed := range allowedHosts {
		if host == strings.ToLower(allowed) {
			return nil
		}
	}
	return newPreflightError(fmt.Sprintf(
		"instrument list: refusing manifest URL %q — host %q is not in the configured allowlist %v", rawURL, host, allowedHosts))
}

// download streams the CSV. It attaches no credentials whatsoever — not
// Authorization, not x-api-key — regardless of what the Client was
// configured with: this request goes to a third-party CDN, not the HIGH API.
//
// Unlike doRequest, this is not wrapped in its own context.WithTimeout: the
// manifest lookup above already spent config.timeoutMs on a JSON call the
// size of a few kilobytes, but the CSV itself can be 14MB, and clamping that
// to the same per-request budget would make the documented 30s default
// actively hostile to the two largest files. The download is bounded only by
// ctx — see Stream's doc comment for a longer-lived context on a slow link.
func (r *InstrumentsResource) download(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, newTransportError("instrument list: failed to build the download request", err)
	}
	req.Header.Set("Accept", "text/csv")
	req.Header.Set("User-Agent", r.config.userAgent)

	r.config.logger.Info(fmt.Sprintf("HIGH -> GET %s", redactURL(rawURL)), nil)
	resp, err := r.config.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return nil, newTransportError("instrument list: download failed", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, errorFromResponse(resp.StatusCode, raw)
	}
	return resp, nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// parseInstrumentRow builds one Instrument from a CSV record, given the
// column-name-to-index map Stream built from the (already validated) header.
func parseInstrumentRow(index map[string]int, record []string) (Instrument, error) {
	col := func(name string) string {
		i, ok := index[name]
		if !ok || i >= len(record) {
			return ""
		}
		return record[i]
	}

	row := Instrument{
		Exchange:         col("exchange"),
		Segment:          col("segment"),
		InstrumentType:   col("instrument"),
		TradingSymbol:    col("high_trading_symbol"),
		ScripKey:         col("scrip_key"),
		ISIN:             col("isin"),
		Symbol:           col("symbol"),
		Name:             col("name"),
		GroupSeries:      col("group_series"),
		UnderlyingSymbol: col("underlying_symbol"),
		Expiry:           col("expiry"),
		OptionType:       col("option_type"),
	}

	var err error
	if row.ScripCode, err = parseIntColumn(col("scrip_code")); err != nil {
		return Instrument{}, newPreflightError(fmt.Sprintf("instrument list: row %v: scrip_code: %s", record, err.Error()))
	}
	if row.HasFnO, err = parseIntColumn(col("has_fno")); err != nil {
		return Instrument{}, newPreflightError(fmt.Sprintf("instrument list: row %v: has_fno: %s", record, err.Error()))
	}
	if row.PriceTick, err = parseIntColumn(col("price_tick")); err != nil {
		return Instrument{}, newPreflightError(fmt.Sprintf("instrument list: row %v: price_tick: %s", record, err.Error()))
	}
	if row.LotSize, err = parseIntColumn(col("lot_size")); err != nil {
		return Instrument{}, newPreflightError(fmt.Sprintf("instrument list: row %v: lot_size: %s", record, err.Error()))
	}

	if raw := col("strike_price"); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return Instrument{}, newPreflightError(fmt.Sprintf("instrument list: row %v: strike_price: %s", record, err.Error()))
		}
		row.StrikePrice = &v
	}

	return row, nil
}

func parseIntColumn(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	return strconv.Atoi(raw)
}
