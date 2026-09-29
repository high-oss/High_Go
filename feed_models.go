// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Decimal is a price, or another value the feed carries with the same care,
// exactly as the wire sent it.
//
// The wire protocol carries these as decimal strings, not binary floats
// (datafeed plan, Phase 0: "Values are strings and must be parsed to the
// language's decimal type, never to a binary float for money" -- ltp
// "1905.65", not a JSON number). Go's standard library has no
// arbitrary-precision decimal type, and this package does no arithmetic on
// these values -- it only carries them from the wire to the caller -- so
// pulling in a decimal-math dependency would spend the one dependency this
// SDK is permitted to add (coder/websocket already spends it) on a
// capability nothing here needs. Decimal is therefore the trimmed wire
// string itself, under a named type: cheap, exact, and distinct enough from
// a plain string that it is not mistaken for an identifier like
// TradingSymbol. A caller who does want float math opts in explicitly with
// Float64.
type Decimal string

// Float64 parses the value with strconv.ParseFloat. Nothing in this package
// calls it; every field that carries money stays a Decimal until a caller
// opts into the precision loss themselves.
func (d Decimal) Float64() (float64, error) {
	return strconv.ParseFloat(string(d), 64)
}

func (d Decimal) String() string { return string(d) }

// istLocation is a fixed UTC+05:30 offset, not a loaded "Asia/Kolkata" zone:
// India has no DST, so the offset never changes, and a fixed zone needs no
// system tzdata (not guaranteed present in a minimal container). The feed's
// timestamps carry no offset of their own -- they are IST wall-clock -- so
// this is where that fact is made explicit rather than left implicit by
// parsing into UTC.
var istLocation = time.FixedZone("IST", 5*3600+30*60)

// parseFeedTime parses raw with layout in istLocation, trimming leading
// (and trailing) whitespace first -- the vendor sample shows at least one
// numeric field with a leading space (" 9408.60"), and nothing rules out the
// same in a timestamp. An empty string after trimming is not an error: the
// field was simply absent from this delta. A malformed value is reported to
// the caller rather than silently zeroed.
func parseFeedTime(layout, raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, nil
	}
	return time.ParseInLocation(layout, trimmed, istLocation)
}

// Layouts for the feed's three different timestamp formats (plan, Phase 0):
// none carries a timezone offset of its own; all are IST wall-clock.
const (
	lastTradedTimeLayout = "02/01/2006 15:04:05"  // ltt: 29/04/2020 15:59:44
	feedTimeLayout       = "02-Jan-2006 15:04:05" // fdtm and index tvalue: 29-Apr-2020 17:34:36
)

// rawFrame is one element of a tick array as it arrives on the wire, decoded
// just far enough to route it: every value stays undecoded so the merge
// functions below can parse each field with its own rule (decimal, integer,
// or one of the two timestamp layouts) and so a field neither this package
// nor the caller has ever seen is preserved rather than dropped.
type rawFrame map[string]json.RawMessage

// fieldString reads one field as a string, whether the wire sent it quoted
// (the normal case -- prices, quantities and timestamps are all decimal or
// text strings) or as a bare JSON number or boolean. A field absent from
// this delta returns ok == false, distinct from a field present but empty.
func fieldString(frame rawFrame, key string) (string, bool) {
	raw, present := frame[key]
	if !present || len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, true
	}
	// Not a JSON string (a bare number, e.g. maxScripPerConn-style fields on
	// other frames) -- the raw JSON text is already the value.
	return string(raw), true
}

// fieldInt64 reads one field as an integer, tolerating the same
// quoted-or-bare shapes as fieldString and a leading/trailing space.
func fieldInt64(frame rawFrame, key string) (int64, bool, error) {
	s, present := fieldString(frame, key)
	if !present {
		return 0, false, nil
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0, false, nil
	}
	// Some counters arrive as a decimal-looking string ("0.00"); truncate
	// through float64 rather than fail a whole tick over it.
	if v, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return v, true, nil
	}
	f, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, true, err
	}
	return int64(f), true, nil
}

// fieldDecimal reads one field as a Decimal: the trimmed wire string,
// unparsed. See the Decimal type comment for why this package never parses
// money to float64.
func fieldDecimal(frame rawFrame, key string) (Decimal, bool) {
	s, present := fieldString(frame, key)
	if !present {
		return "", false
	}
	return Decimal(strings.TrimSpace(s)), true
}

// DepthLevel is one price level of an order book.
type DepthLevel struct {
	Price    Decimal
	Quantity int64
	// Orders is the order count at this level. It is only ever populated
	// from the five-level `dp` feed (bno1..bno5 / sno1..sno5) -- the
	// one-level top-of-book split out of a quote tick carries no order
	// count on the wire, so Orders is 0 there. Check Depth.Levels/Source,
	// not this field, to tell the two apart.
	Orders int64
}

// DepthSource says which feed frame a Depth snapshot was built from.
type DepthSource string

const (
	// DepthSourceQuote is the one-level top-of-book carried alongside a
	// FULL-mode quote tick (bp/bq/sp/bs). Depth.Levels is always 1.
	DepthSourceQuote DepthSource = "quote"
	// DepthSourceDepth is the five-level order book (frame `dp`).
	// Depth.Levels is always 5.
	DepthSourceDepth DepthSource = "depth"
)

// Quote is a market-watch snapshot: the complete, merged state of every
// quote field HIGH has ever sent for this instrument, keyed by the caller's
// own scrip key -- never the feed's token. Ticks are deltas; this package
// owns the merge, so every field here is always current even though a given
// wire tick may only have touched one of them.
type Quote struct {
	// ScripKey is the HIGH scrip key this snapshot was subscribed with.
	ScripKey string
	// TradingSymbol is the feed's own symbol for this instrument (`ts`).
	TradingSymbol      string
	LastTradedPrice    Decimal
	LastTradedQuantity int64
	// LastTradedTime is IST wall-clock (see the Decimal and istLocation
	// comments for why no conversion to UTC happens here). Zero when never
	// sent or when the last value failed to parse -- see Extra for the raw
	// string in that case.
	LastTradedTime    time.Time
	Volume            int64
	Turnover          Decimal
	AverageTradePrice Decimal
	Open              Decimal
	High              Decimal
	Low               Decimal
	// PreviousClose is the prior session's close (`c`), not today's.
	PreviousClose Decimal
	YearHigh      Decimal
	YearLow       Decimal
	// Change is the absolute move (`cng`) and ChangePercent is the
	// percentage move (`nc`) -- the reverse of what the vendor's own sample
	// README claims; the apiDoc's own numbers settle it (feed_test.go pins
	// this with the worked example from the plan).
	Change            Decimal
	ChangePercent     Decimal
	TotalBuyQuantity  int64
	TotalSellQuantity int64
	OpenInterest      int64
	LowerCircuitLimit Decimal
	UpperCircuitLimit Decimal
	// FeedTime is the exchange update time (`fdtm`), IST wall-clock.
	FeedTime   time.Time
	Multiplier int64
	Precision  int64

	// Changed lists the Go field names (this struct's, using these exact
	// names) this delta actually touched -- never the fields that merely
	// carried their previous value forward. Sorted for a stable order.
	Changed []string
	// Extra carries any wire field this package does not know about (raw,
	// trimmed strings), so a vendor addition reaches the caller instead of
	// silently vanishing.
	Extra map[string]string
}

// Index is a merged index snapshot, keyed by the caller's own scrip key.
type Index struct {
	ScripKey string
	// IndexName is the feed's own name for this index (`tk`/`ts`).
	IndexName     string
	IndexValue    Decimal
	PreviousClose Decimal
	Open          Decimal
	High          Decimal
	Low           Decimal
	Change        Decimal
	ChangePercent Decimal
	// FeedTime is IST wall-clock (`tvalue`).
	FeedTime time.Time

	Changed []string
	Extra   map[string]string
}

// Depth is a merged order-book snapshot, keyed by the caller's own scrip
// key. Levels and Source together say exactly what this is: a one-level
// top-of-book split out of a quote tick (Levels 1, Source
// DepthSourceQuote), or the feed's own five-level book (Levels 5, Source
// DepthSourceDepth). The two are never mixed into one another -- a one-level
// book is never padded out to look like a five-level one.
type Depth struct {
	ScripKey string
	Levels   int
	Source   DepthSource
	Bids     []DepthLevel
	Asks     []DepthLevel

	Changed []string
	Extra   map[string]string
}

// quoteFieldSetters maps a wire key to a setter that parses it into q and
// returns the Go field name that changed. Built once; see mergeQuote.
var quoteFieldSetters = map[string]func(q *Quote, frame rawFrame) (string, error){
	"ts": func(q *Quote, f rawFrame) (string, error) {
		if v, ok := fieldString(f, "ts"); ok {
			q.TradingSymbol = strings.TrimSpace(v)
			return "TradingSymbol", nil
		}
		return "", nil
	},
	"ltp": decimalSetter(func(q *Quote) *Decimal { return &q.LastTradedPrice }, "ltp", "LastTradedPrice"),
	"ltq": intSetter(func(q *Quote) *int64 { return &q.LastTradedQuantity }, "ltq", "LastTradedQuantity"),
	"ltt": func(q *Quote, f rawFrame) (string, error) {
		v, ok := fieldString(f, "ltt")
		if !ok {
			return "", nil
		}
		t, err := parseFeedTime(lastTradedTimeLayout, v)
		if err != nil {
			q.setExtra("ltt_unparsed", v)
			return "LastTradedTime", nil
		}
		q.LastTradedTime = t
		return "LastTradedTime", nil
	},
	"v":   intSetter(func(q *Quote) *int64 { return &q.Volume }, "v", "Volume"),
	"to":  decimalSetter(func(q *Quote) *Decimal { return &q.Turnover }, "to", "Turnover"),
	"ap":  decimalSetter(func(q *Quote) *Decimal { return &q.AverageTradePrice }, "ap", "AverageTradePrice"),
	"op":  decimalSetter(func(q *Quote) *Decimal { return &q.Open }, "op", "Open"),
	"h":   decimalSetter(func(q *Quote) *Decimal { return &q.High }, "h", "High"),
	"lo":  decimalSetter(func(q *Quote) *Decimal { return &q.Low }, "lo", "Low"),
	"c":   decimalSetter(func(q *Quote) *Decimal { return &q.PreviousClose }, "c", "PreviousClose"),
	"yh":  decimalSetter(func(q *Quote) *Decimal { return &q.YearHigh }, "yh", "YearHigh"),
	"yl":  decimalSetter(func(q *Quote) *Decimal { return &q.YearLow }, "yl", "YearLow"),
	"cng": decimalSetter(func(q *Quote) *Decimal { return &q.Change }, "cng", "Change"),
	"nc":  decimalSetter(func(q *Quote) *Decimal { return &q.ChangePercent }, "nc", "ChangePercent"),
	"tbq": intSetter(func(q *Quote) *int64 { return &q.TotalBuyQuantity }, "tbq", "TotalBuyQuantity"),
	"tsq": intSetter(func(q *Quote) *int64 { return &q.TotalSellQuantity }, "tsq", "TotalSellQuantity"),
	"oi":  intSetter(func(q *Quote) *int64 { return &q.OpenInterest }, "oi", "OpenInterest"),
	"lcl": decimalSetter(func(q *Quote) *Decimal { return &q.LowerCircuitLimit }, "lcl", "LowerCircuitLimit"),
	"ucl": decimalSetter(func(q *Quote) *Decimal { return &q.UpperCircuitLimit }, "ucl", "UpperCircuitLimit"),
	"fdtm": func(q *Quote, f rawFrame) (string, error) {
		v, ok := fieldString(f, "fdtm")
		if !ok {
			return "", nil
		}
		t, err := parseFeedTime(feedTimeLayout, v)
		if err != nil {
			q.setExtra("fdtm_unparsed", v)
			return "FeedTime", nil
		}
		q.FeedTime = t
		return "FeedTime", nil
	},
	"mul":  intSetter(func(q *Quote) *int64 { return &q.Multiplier }, "mul", "Multiplier"),
	"prec": intSetter(func(q *Quote) *int64 { return &q.Precision }, "prec", "Precision"),
}

func (q *Quote) setExtra(key, value string) {
	if q.Extra == nil {
		q.Extra = map[string]string{}
	}
	q.Extra[key] = value
}

// topOfBookKeys are the fields a FULL-mode quote tick carries alongside the
// quote fields above, split out into a separate Depth event rather than
// folded into Quote (datafeed plan, "one frame, two events").
var topOfBookKeys = []string{"bp", "bq", "sp", "bs"}

func decimalSetter(field func(*Quote) *Decimal, wireKey, goName string) func(*Quote, rawFrame) (string, error) {
	return func(q *Quote, f rawFrame) (string, error) {
		if v, ok := fieldDecimal(f, wireKey); ok {
			*field(q) = v
			return goName, nil
		}
		return "", nil
	}
}

func intSetter(field func(*Quote) *int64, wireKey, goName string) func(*Quote, rawFrame) (string, error) {
	return func(q *Quote, f rawFrame) (string, error) {
		v, present, err := fieldInt64(f, wireKey)
		if !present {
			return "", nil
		}
		if err != nil {
			return "", nil // malformed integer: leave the previous value, do not fail the whole tick
		}
		*field(q) = v
		return goName, nil
	}
}

// knownQuoteWireKeys is quoteFieldSetters' key set plus the top-of-book and
// routing/identity keys, so mergeQuote can tell a genuinely unknown field
// (destined for Extra) from one it deliberately handles elsewhere.
var knownQuoteWireKeys = buildKnownQuoteKeys()

func buildKnownQuoteKeys() map[string]bool {
	out := map[string]bool{"e": true, "tk": true, "name": true}
	for k := range quoteFieldSetters {
		out[k] = true
	}
	for _, k := range topOfBookKeys {
		out[k] = true
	}
	return out
}

// mergeQuote applies one delta frame onto the persisted quote state,
// returning the sorted Go field names this specific delta changed. Fields
// the wire did not send keep their previous value -- ticks are deltas, and
// this is the merge the plan requires so a caller never sees nulls for
// fields that simply did not move this time.
func mergeQuote(state *Quote, frame rawFrame) []string {
	var changed []string
	for _, setter := range quoteFieldSetters {
		name, err := setter(state, frame)
		if err == nil && name != "" {
			changed = append(changed, name)
		}
	}
	for wireKey, raw := range frame {
		if knownQuoteWireKeys[wireKey] {
			continue
		}
		state.setExtra(wireKey, rawJSONText(raw))
		changed = append(changed, wireKey)
	}
	sortUnique(changed)
	return changed
}

// mergeTopOfBook applies the four top-of-book fields a FULL quote tick
// carries onto a persisted one-level bid/ask pair, returning the sorted
// field names changed. Field names here match DepthLevel's own field names,
// prefixed Bid/Ask, so a caller reading Changed does not need to know the
// wire's bp/bq/sp/bs.
func mergeTopOfBook(bid, ask *DepthLevel, frame rawFrame) []string {
	var changed []string
	if v, ok := fieldDecimal(frame, "bp"); ok {
		bid.Price = v
		changed = append(changed, "BidPrice")
	}
	if v, present, err := fieldInt64(frame, "bq"); present && err == nil {
		bid.Quantity = v
		changed = append(changed, "BidQuantity")
	}
	if v, ok := fieldDecimal(frame, "sp"); ok {
		ask.Price = v
		changed = append(changed, "AskPrice")
	}
	if v, present, err := fieldInt64(frame, "bs"); present && err == nil {
		ask.Quantity = v
		changed = append(changed, "AskQuantity")
	}
	sortUnique(changed)
	return changed
}

// depthLevelSuffixes pairs each of the five levels' price/quantity suffix
// with its order-count suffix. The two numbering bases differ (plan, Phase
// 0): price and quantity start unsuffixed at level 1 and count 1..4 for
// levels 2..5, but order counts start at 1 for level 1 and count 1..5 --
// so level n is bp{n-1} (empty string for n=1) but bno{n}. Naive index
// pairing is the exact off-by-one trap the plan calls out; this table makes
// the pairing explicit instead of computed.
var depthLevelSuffixes = [5]struct{ priceQty, order string }{
	{"", "1"},
	{"1", "2"},
	{"2", "3"},
	{"3", "4"},
	{"4", "5"},
}

// mergeDepth applies a five-level `dp` delta onto persisted five-level bid
// and ask arrays, returning the sorted changed field names ("Bid1Price",
// "Ask3Orders", ...). Absent levels/fields on this delta keep their
// previous value, same as mergeQuote.
func mergeDepth(bids, asks *[5]DepthLevel, frame rawFrame) []string {
	var changed []string
	for i, suf := range depthLevelSuffixes {
		level := i + 1
		if v, ok := fieldDecimal(frame, "bp"+suf.priceQty); ok {
			bids[i].Price = v
			changed = append(changed, depthFieldName("Bid", level, "Price"))
		}
		if v, present, err := fieldInt64(frame, "bq"+suf.priceQty); present && err == nil {
			bids[i].Quantity = v
			changed = append(changed, depthFieldName("Bid", level, "Quantity"))
		}
		if v, present, err := fieldInt64(frame, "bno"+suf.order); present && err == nil {
			bids[i].Orders = v
			changed = append(changed, depthFieldName("Bid", level, "Orders"))
		}
		if v, ok := fieldDecimal(frame, "sp"+suf.priceQty); ok {
			asks[i].Price = v
			changed = append(changed, depthFieldName("Ask", level, "Price"))
		}
		if v, present, err := fieldInt64(frame, "bs"+suf.priceQty); present && err == nil {
			asks[i].Quantity = v
			changed = append(changed, depthFieldName("Ask", level, "Quantity"))
		}
		if v, present, err := fieldInt64(frame, "sno"+suf.order); present && err == nil {
			asks[i].Orders = v
			changed = append(changed, depthFieldName("Ask", level, "Orders"))
		}
	}
	sortUnique(changed)
	return changed
}

func depthFieldName(side string, level int, field string) string {
	return side + strconv.Itoa(level) + field
}

// indexFieldSetters mirrors quoteFieldSetters for the `if` frame.
var indexKnownWireKeys = map[string]bool{
	"e": true, "tk": true, "name": true, "ts": true, "iv": true, "ic": true,
	"openingprice": true, "highprice": true, "lowprice": true, "cng": true,
	"nc": true, "tvalue": true,
}

// mergeIndex applies one `if` delta onto the persisted index state,
// returning the sorted Go field names changed. tk *is* the index's name on
// this frame (plan, Phase 0), so it is folded into IndexName exactly like
// ts, not treated as an opaque identifier the way it is for a non-index
// scrip.
func mergeIndex(state *Index, frame rawFrame) []string {
	var changed []string
	if v, ok := fieldString(frame, "ts"); ok {
		state.IndexName = strings.TrimSpace(v)
		changed = append(changed, "IndexName")
	} else if v, ok := fieldString(frame, "tk"); ok {
		state.IndexName = strings.TrimSpace(v)
		changed = append(changed, "IndexName")
	}
	if v, ok := fieldDecimal(frame, "iv"); ok {
		state.IndexValue = v
		changed = append(changed, "IndexValue")
	}
	if v, ok := fieldDecimal(frame, "ic"); ok {
		state.PreviousClose = v
		changed = append(changed, "PreviousClose")
	}
	if v, ok := fieldDecimal(frame, "openingprice"); ok {
		state.Open = v
		changed = append(changed, "Open")
	}
	if v, ok := fieldDecimal(frame, "highprice"); ok {
		state.High = v
		changed = append(changed, "High")
	}
	if v, ok := fieldDecimal(frame, "lowprice"); ok {
		state.Low = v
		changed = append(changed, "Low")
	}
	if v, ok := fieldDecimal(frame, "cng"); ok {
		state.Change = v
		changed = append(changed, "Change")
	}
	if v, ok := fieldDecimal(frame, "nc"); ok {
		state.ChangePercent = v
		changed = append(changed, "ChangePercent")
	}
	if v, ok := fieldString(frame, "tvalue"); ok {
		if t, err := parseFeedTime(feedTimeLayout, v); err == nil {
			state.FeedTime = t
		} else {
			if state.Extra == nil {
				state.Extra = map[string]string{}
			}
			state.Extra["tvalue_unparsed"] = v
		}
		changed = append(changed, "FeedTime")
	}
	for wireKey, raw := range frame {
		if indexKnownWireKeys[wireKey] {
			continue
		}
		if state.Extra == nil {
			state.Extra = map[string]string{}
		}
		state.Extra[wireKey] = rawJSONText(raw)
		changed = append(changed, wireKey)
	}
	sortUnique(changed)
	return changed
}

func rawJSONText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
}

func sortUnique(items []string) {
	if len(items) < 2 {
		return
	}
	// Small slices (at most a few dozen fields) -- insertion sort plus a
	// dedupe pass is plenty and keeps this dependency-free.
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j-1] > items[j]; j-- {
			items[j-1], items[j] = items[j], items[j-1]
		}
	}
}
