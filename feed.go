// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Feed is the HIGH live datafeed client: a raw WebSocket carrying JSON text
// frames, built from the same Options as Client but talking to a separate,
// production-only host. There is no sandbox feed -- NewFeed refuses to build
// one for Options.Environment: EnvironmentSandbox rather than quietly
// connecting to production.
//
// A Feed is safe for concurrent use: Subscribe/Unsubscribe/Snapshot calls,
// tick delivery, and the reconnect supervisor all guard the shared
// subscription and merge state with the same mutex.
//
//	feed, err := highopenapi.NewFeed(highopenapi.Options{AccessToken: accessToken})
//	if err := feed.Connect(ctx); err != nil { ... }
//	if err := feed.SubscribeQuotes(ctx, []string{"NSE@2885"}); err != nil { ... }
//	for quote := range feed.Quotes() {
//		fmt.Println(quote.ScripKey, quote.LastTradedPrice)
//	}
type Feed struct {
	config *resolvedConfig

	mu        sync.Mutex
	writeMu   sync.Mutex
	conn      *websocket.Conn
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once

	maxScripPerConn int
	maxScripPerReq  int

	// subscribed is what gets resent after a reconnect: every scrip key
	// currently subscribed, per kind, as its already-translated feed
	// identifier (so a reconnect never re-runs key translation).
	subscribed map[feedKind]map[string]translatedFeedKey
	// reverse maps a feed instrument identifier ("<segment>|<id>", the same
	// shape translatedFeedKey.wireScrip() produces) back to the caller's own
	// scrip key, so an incoming tick -- which only carries e/tk -- can be
	// routed and merged under the key the caller subscribed with.
	reverse map[feedKind]map[string]string

	quoteState map[string]*Quote
	// topOfBook holds the one-level book split out of a FULL quote tick,
	// [0] bid / [1] ask, keyed by scripKey -- separate from depthState,
	// which is the five-level `dp` feed's own state.
	topOfBook map[string]*[2]DepthLevel
	// depthState holds the five-level book.
	depthState map[string]*depthBookState
	indexState map[string]*Index

	quoteCh chan Quote
	depthCh chan Depth
	indexCh chan Index
	errCh   chan error
}

// depthBookState is the persisted five-level book for one instrument
// subscribed via SubscribeDepth.
type depthBookState struct {
	bids [5]DepthLevel
	asks [5]DepthLevel
}

// feedChannelBuffer sizes the four delivery channels. Delivery is
// non-blocking (see sendQuote and friends): a full channel means the caller
// is not draining it fast enough, and a dropped tick is not a data-loss bug
// the way it would be for a REST response, because the next tick still
// carries the fully merged state -- only the intermediate event is missed,
// not the value.
const feedChannelBuffer = 1024

const feedErrorChannelBuffer = 16

// NewFeed builds a Feed from the same Options as New. Environment resolution
// is identical to the REST client's (contract §2) with one addition: there
// is no sandbox feed, so an Options.Environment of EnvironmentSandbox (or
// HIGH_ENVIRONMENT=sandbox) fails here, at construction, rather than at
// Connect or by quietly reaching production.
func NewFeed(opts Options) (*Feed, error) {
	if opts.getenv == nil {
		opts.getenv = defaultGetenv
	}
	config, err := resolveConfig(opts)
	if err != nil {
		return nil, err
	}
	if config.environment == EnvironmentSandbox {
		return nil, fmt.Errorf(
			"datafeed: there is no sandbox feed; the datafeed is production-only (%s). Build the Feed with Environment: EnvironmentProduction, or leave Environment unset",
			Environments[EnvironmentProduction].WS)
	}
	if config.wsBaseURL == "" {
		return nil, fmt.Errorf(
			"datafeed: no datafeed host is configured; set Options.WSBaseURL or HIGH_WS_BASE_URL, or use Environment: EnvironmentProduction")
	}

	return &Feed{
		config: config,
		subscribed: map[feedKind]map[string]translatedFeedKey{
			feedKindQuote: {}, feedKindDepth: {}, feedKindIndex: {},
		},
		reverse: map[feedKind]map[string]string{
			feedKindQuote: {}, feedKindDepth: {}, feedKindIndex: {},
		},
		quoteState: map[string]*Quote{},
		topOfBook:  map[string]*[2]DepthLevel{},
		depthState: map[string]*depthBookState{},
		indexState: map[string]*Index{},
		quoteCh:    make(chan Quote, feedChannelBuffer),
		depthCh:    make(chan Depth, feedChannelBuffer),
		indexCh:    make(chan Index, feedChannelBuffer),
		errCh:      make(chan error, feedErrorChannelBuffer),
	}, nil
}

// String never includes credentials, matching Client.String (contract §7).
func (f *Feed) String() string {
	return "highopenapi.Feed{wsBaseURL: " + f.config.wsBaseURL + ", environment: redacted-credentials}"
}

// GoString backs %#v the same way String backs %v/%s.
func (f *Feed) GoString() string { return f.String() }

// Quotes delivers a complete, merged Quote snapshot every time a subscribed
// instrument's quote fields change. Split out of a FULL market-watch tick's
// top-of-book fields, which arrive on Depths instead -- see the Depth doc
// comment.
func (f *Feed) Quotes() <-chan Quote { return f.quoteCh }

// Depths delivers a complete, merged order-book snapshot every time a
// subscribed instrument's book changes -- both the one-level top-of-book
// carried alongside a quote tick (Source DepthSourceQuote, Levels 1) and the
// feed's own five-level book (Source DepthSourceDepth, Levels 5). Check
// Levels/Source before assuming five levels are populated.
func (f *Feed) Depths() <-chan Depth { return f.depthCh }

// Indices delivers a complete, merged Index snapshot every time a
// subscribed index changes.
func (f *Feed) Indices() <-chan Index { return f.indexCh }

// Errors delivers asynchronous feed failures that happen after Connect
// returns: a reconnect giving up because re-authentication was refused
// (never retried further -- see FeedAuthError), and a resubscribe failure
// after an otherwise-successful reconnect. Connect's own return value
// already covers the initial connection; this channel is for everything
// that can only be discovered later, on a connection already handed back to
// the caller.
func (f *Feed) Errors() <-chan error { return f.errCh }

// Connect dials the feed, sends the auth frame, and blocks until its
// acknowledgement arrives -- nothing else is sent before it (datafeed plan,
// Phase 0). On success it reads maxScripPerConn/maxScripPerReq off the same
// acknowledgement and starts the background read loop; on a NotOk
// acknowledgement it returns a *FeedAuthError and connects nothing -- no
// retry, no reconnect, matching the contract's "the server will keep
// refusing" rule.
//
// ctx governs the dial and the auth handshake, and its cancellation also
// tears the connection down afterwards (as does Close) -- both go through
// the same internal cancellation, so a caller who cancels ctx instead of
// calling Close still gets a clean shutdown.
func (f *Feed) Connect(ctx context.Context) error {
	f.mu.Lock()
	alreadyConnected := f.conn != nil
	f.mu.Unlock()
	if alreadyConnected {
		return fmt.Errorf("datafeed: already connected")
	}

	if f.config.accessToken == "" {
		return newPreflightError("This operation needs an accessToken. Pass it to the client, or set HIGH_ACCESS_TOKEN.")
	}

	conn, ack, err := f.dialAndAuth(ctx)
	if err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)

	f.mu.Lock()
	f.conn = conn
	f.cancel = cancel
	f.maxScripPerConn = ack.MaxScripPerConn
	f.maxScripPerReq = ack.MaxScripPerReq
	f.mu.Unlock()

	f.wg.Add(1)
	go f.runLoop(runCtx)

	return nil
}

// dialAndAuth performs one full dial-then-authenticate handshake: connect,
// send {"type":"cn","sessionid":...} (never "mode" -- that follows the data
// plan server-side), and block for the acknowledgement before returning.
// Used both by Connect and by the reconnect loop, so a reconnect
// re-authenticates exactly the same way the first connection did.
func (f *Feed) dialAndAuth(ctx context.Context) (*websocket.Conn, authAck, error) {
	conn, _, err := websocket.Dial(ctx, f.config.wsBaseURL, nil)
	if err != nil {
		return nil, authAck{}, newTransportError("datafeed: failed to connect", err)
	}

	payload, err := json.Marshal(newAuthRequest(f.config.accessToken))
	if err != nil {
		conn.CloseNow()
		return nil, authAck{}, fmt.Errorf("datafeed: failed to encode the auth frame: %w", err)
	}
	f.logFrame("->", payload)
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		conn.CloseNow()
		return nil, authAck{}, newTransportError("datafeed: failed to send the auth frame", err)
	}

	_, data, err := conn.Read(ctx)
	if err != nil {
		conn.CloseNow()
		return nil, authAck{}, newTransportError("datafeed: failed to read the auth acknowledgement", err)
	}
	f.logFrame("<-", data)

	ack, err := decodeAck(data)
	if err != nil {
		conn.CloseNow()
		return nil, authAck{}, err
	}
	if !ack.ok() {
		conn.CloseNow()
		return nil, authAck{}, classifyFeedAuthError(ack.StCode, ack.Msg)
	}
	return conn, ack, nil
}

func (f *Feed) logFrame(direction string, payload []byte) {
	if f.config.logger == nil || !f.config.logger.enabled(LogLevelDebug) {
		return
	}
	f.config.logger.Debug(fmt.Sprintf("HIGH datafeed %s %d bytes", direction, len(payload)), redactBody(payload))
}

// --- Quotes -----------------------------------------------------------

// SubscribeQuotes subscribes to market-watch (touchline) updates for
// scripKeys, delivered on Quotes -- and, for any top-of-book fields the same
// FULL-mode tick carries, on Depths as a one-level book. Every key must
// translate to a non-index feed identifier; an index key (present in the
// committed index table, or one this SDK cannot translate but recognises as
// an index) is refused -- use SubscribeIndices instead.
func (f *Feed) SubscribeQuotes(ctx context.Context, scripKeys []string) error {
	return f.subscribe(ctx, feedKindQuote, scripKeys, translateNonIndexKey)
}

// UnsubscribeQuotes reverses SubscribeQuotes.
func (f *Feed) UnsubscribeQuotes(ctx context.Context, scripKeys []string) error {
	return f.unsubscribe(ctx, feedKindQuote, scripKeys, translateNonIndexKey)
}

// SnapshotQuotes requests an immediate one-shot quote update for scripKeys,
// delivered the same way an ordinary tick is -- on Quotes (and Depths for
// any top-of-book fields it carries).
func (f *Feed) SnapshotQuotes(ctx context.Context, scripKeys []string) error {
	return f.snapshot(ctx, feedKindQuote, scripKeys, translateNonIndexKey)
}

// --- Depth --------------------------------------------------------------

// SubscribeDepth subscribes to the five-level order book for scripKeys,
// delivered on Depths with Levels 5 and Source DepthSourceDepth. As with
// SubscribeQuotes, an index key is refused -- use SubscribeIndices.
func (f *Feed) SubscribeDepth(ctx context.Context, scripKeys []string) error {
	return f.subscribe(ctx, feedKindDepth, scripKeys, translateNonIndexKey)
}

// UnsubscribeDepth reverses SubscribeDepth.
func (f *Feed) UnsubscribeDepth(ctx context.Context, scripKeys []string) error {
	return f.unsubscribe(ctx, feedKindDepth, scripKeys, translateNonIndexKey)
}

// SnapshotDepth requests an immediate one-shot five-level book for
// scripKeys, delivered on Depths.
func (f *Feed) SnapshotDepth(ctx context.Context, scripKeys []string) error {
	return f.snapshot(ctx, feedKindDepth, scripKeys, translateNonIndexKey)
}

// --- Indices --------------------------------------------------------------

// SubscribeIndices subscribes to index updates for scripKeys, delivered on
// Indices. Every key must be present in the committed index table
// (feed_index_table.go); a key this SDK does not recognise as an index --
// including an ordinary equity, futures or options key -- is refused, naming
// the key and saying so. This is the mirror image of SubscribeQuotes and
// SubscribeDepth's own index rejection: an index key only ever works through
// these three methods.
func (f *Feed) SubscribeIndices(ctx context.Context, scripKeys []string) error {
	return f.subscribe(ctx, feedKindIndex, scripKeys, translateIndexKey)
}

// UnsubscribeIndices reverses SubscribeIndices.
func (f *Feed) UnsubscribeIndices(ctx context.Context, scripKeys []string) error {
	return f.unsubscribe(ctx, feedKindIndex, scripKeys, translateIndexKey)
}

// SnapshotIndices requests an immediate one-shot index update for
// scripKeys, delivered on Indices.
func (f *Feed) SnapshotIndices(ctx context.Context, scripKeys []string) error {
	return f.snapshot(ctx, feedKindIndex, scripKeys, translateIndexKey)
}

// --- Subscription core ----------------------------------------------------

func (f *Feed) subscribe(ctx context.Context, kind feedKind, scripKeys []string, translate func(string) (translatedFeedKey, error)) error {
	if len(scripKeys) == 0 {
		return nil
	}
	translated, err := translateAll(scripKeys, translate)
	if err != nil {
		return err
	}

	f.mu.Lock()
	if f.conn == nil {
		f.mu.Unlock()
		return fmt.Errorf("datafeed: not connected; call Connect first")
	}

	set := f.subscribed[kind]
	numNew := 0
	for _, t := range translated {
		if _, exists := set[t.scripKey]; !exists {
			numNew++
		}
	}
	grandTotal := f.totalSubscribedLocked() + numNew
	if f.maxScripPerConn > 0 && grandTotal > f.maxScripPerConn {
		f.mu.Unlock()
		return &FeedLimitError{Limit: "maxScripPerConn", Max: f.maxScripPerConn, Requested: grandTotal}
	}

	for _, t := range translated {
		set[t.scripKey] = t
		f.reverse[kind][t.wireScrip()] = t.scripKey
	}
	conn, maxPerReq := f.conn, f.maxScripPerReq
	f.mu.Unlock()

	return f.sendChunked(ctx, conn, kind, "s", translated, maxPerReq)
}

func (f *Feed) unsubscribe(ctx context.Context, kind feedKind, scripKeys []string, translate func(string) (translatedFeedKey, error)) error {
	if len(scripKeys) == 0 {
		return nil
	}
	translated, err := translateAll(scripKeys, translate)
	if err != nil {
		return err
	}

	f.mu.Lock()
	if f.conn == nil {
		f.mu.Unlock()
		return fmt.Errorf("datafeed: not connected; call Connect first")
	}
	set := f.subscribed[kind]
	for _, t := range translated {
		delete(set, t.scripKey)
		delete(f.reverse[kind], t.wireScrip())
	}
	conn, maxPerReq := f.conn, f.maxScripPerReq
	f.mu.Unlock()

	return f.sendChunked(ctx, conn, kind, "u", translated, maxPerReq)
}

func (f *Feed) snapshot(ctx context.Context, kind feedKind, scripKeys []string, translate func(string) (translatedFeedKey, error)) error {
	if len(scripKeys) == 0 {
		return nil
	}
	translated, err := translateAll(scripKeys, translate)
	if err != nil {
		return err
	}

	f.mu.Lock()
	if f.conn == nil {
		f.mu.Unlock()
		return fmt.Errorf("datafeed: not connected; call Connect first")
	}
	// A snapshot response needs somewhere to route back to even without a
	// standing subscription, so it registers routing -- but not in
	// `subscribed`, since a one-shot request is not something a reconnect
	// should replay.
	for _, t := range translated {
		if _, exists := f.reverse[kind][t.wireScrip()]; !exists {
			f.reverse[kind][t.wireScrip()] = t.scripKey
		}
	}
	conn, maxPerReq := f.conn, f.maxScripPerReq
	f.mu.Unlock()

	return f.sendChunked(ctx, conn, kind, "sp", translated, maxPerReq)
}

func translateAll(scripKeys []string, translate func(string) (translatedFeedKey, error)) ([]translatedFeedKey, error) {
	out := make([]translatedFeedKey, 0, len(scripKeys))
	for _, key := range scripKeys {
		t, err := translate(key)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// totalSubscribedLocked sums every kind's subscription count -- maxScripPerConn
// caps the whole connection, not any one kind (datafeed plan, Phase 0).
// Caller must hold f.mu.
func (f *Feed) totalSubscribedLocked() int {
	total := 0
	for _, set := range f.subscribed {
		total += len(set)
	}
	return total
}

// sendChunked splits keys into requests of at most maxPerReq (maxScripPerReq
// from the auth ack) and sends one wire request per chunk, so a subscription
// larger than a single request's limit -- but still within the connection's
// overall limit -- is split rather than rejected.
func (f *Feed) sendChunked(ctx context.Context, conn *websocket.Conn, kind feedKind, suffix string, keys []translatedFeedKey, maxPerReq int) error {
	if len(keys) == 0 {
		return nil
	}
	chunkSize := maxPerReq
	if chunkSize <= 0 {
		chunkSize = len(keys)
	}
	for start := 0; start < len(keys); start += chunkSize {
		end := start + chunkSize
		if end > len(keys) {
			end = len(keys)
		}
		scrips := make([]string, 0, end-start)
		for _, t := range keys[start:end] {
			scrips = append(scrips, t.wireScrip())
		}
		req := newSubRequest(kind, suffix, scrips)
		payload, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("datafeed: failed to encode a %s request: %w", req.Type, err)
		}
		if err := f.writeFrame(ctx, conn, payload); err != nil {
			return fmt.Errorf("datafeed: failed to send a %s request: %w", req.Type, err)
		}
	}
	return nil
}

func (f *Feed) writeFrame(ctx context.Context, conn *websocket.Conn, payload []byte) error {
	f.writeMu.Lock()
	defer f.writeMu.Unlock()
	f.logFrame("->", payload)
	return conn.Write(ctx, websocket.MessageText, payload)
}

// --- Read loop and reconnect ----------------------------------------------

const (
	feedReconnectInitialDelay = 500 * time.Millisecond
	feedReconnectMaxDelay     = 30 * time.Second
)

// runLoop owns every conn.Read call for the Feed's lifetime -- coder/websocket
// permits one concurrent reader, so nothing else may read. It alternates
// between reading until the connection fails and reconnecting, stopping for
// good on caller cancellation (ctx, or Close, which cancels the same
// context) or a NotOk re-authentication.
func (f *Feed) runLoop(ctx context.Context) {
	defer f.wg.Done()
	defer f.closeChannelsOnce()

	for {
		f.readUntilError(ctx)
		if ctx.Err() != nil {
			// Caller cancellation (or Close, which cancels the same
			// context): close the socket ourselves right here rather than
			// relying on however promptly the websocket library's own
			// ctx-triggered teardown happens to run -- "closing cleanly" on
			// context cancellation is this package's job, not an incidental
			// side effect to hope for.
			f.closeConn()
			return
		}
		if !f.reconnect(ctx) {
			return
		}
	}
}

func (f *Feed) closeConn() {
	f.mu.Lock()
	conn := f.conn
	f.mu.Unlock()
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "closing")
	}
}

func (f *Feed) readUntilError(ctx context.Context) {
	for {
		f.mu.Lock()
		conn := f.conn
		f.mu.Unlock()
		if conn == nil {
			return
		}
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		f.logFrame("<-", data)
		f.dispatch(data)
	}
}

// reconnect redials and re-authenticates with exponential backoff (capped),
// retrying transport failures indefinitely until ctx is done. A NotOk
// re-authentication is never retried -- same rule as the first Connect --
// and stops the Feed for good, surfacing the error on Errors rather than
// looping against a server that will keep refusing.
func (f *Feed) reconnect(ctx context.Context) bool {
	delay := feedReconnectInitialDelay
	for {
		if ctx.Err() != nil {
			return false
		}

		conn, ack, err := f.dialAndAuth(ctx)
		if err != nil {
			var authErr *FeedAuthError
			if errors.As(err, &authErr) {
				f.config.logger.Error("datafeed: reconnect refused by the server, giving up", authErr.Error())
				f.emitError(authErr)
				return false
			}
			f.config.logger.Warn(fmt.Sprintf("datafeed: reconnect failed, retrying in %s", delay), err.Error())
			if sleepErr := sleepCtx(ctx, delay); sleepErr != nil {
				return false
			}
			delay *= 2
			if delay > feedReconnectMaxDelay {
				delay = feedReconnectMaxDelay
			}
			continue
		}

		f.mu.Lock()
		f.conn = conn
		f.maxScripPerConn = ack.MaxScripPerConn
		f.maxScripPerReq = ack.MaxScripPerReq
		f.mu.Unlock()

		if err := f.resubscribeAll(ctx); err != nil {
			f.emitError(fmt.Errorf("datafeed: reconnected but failed to resubscribe everything: %w", err))
		}
		f.config.logger.Info("datafeed: reconnected", nil)
		return true
	}
}

// resubscribeAll resends every currently-tracked subscription after a
// reconnect, before the Feed is considered connected again (datafeed plan:
// "re-authenticates and re-subscribes everything before declaring itself
// connected").
func (f *Feed) resubscribeAll(ctx context.Context) error {
	f.mu.Lock()
	conn := f.conn
	maxPerReq := f.maxScripPerReq
	byKind := make(map[feedKind][]translatedFeedKey, len(f.subscribed))
	for kind, set := range f.subscribed {
		list := make([]translatedFeedKey, 0, len(set))
		for _, t := range set {
			list = append(list, t)
		}
		byKind[kind] = list
	}
	f.mu.Unlock()

	for kind, list := range byKind {
		if err := f.sendChunked(ctx, conn, kind, "s", list, maxPerReq); err != nil {
			return err
		}
	}
	return nil
}

// --- Tick dispatch and merge -----------------------------------------------

func (f *Feed) dispatch(data []byte) {
	frames, err := splitFrames(data)
	if err != nil {
		f.config.logger.Warn("datafeed: could not parse an incoming frame", err.Error())
		return
	}
	for _, frame := range frames {
		if frameIsAck(frame) {
			f.handleAck(frame)
			continue
		}
		switch frameName(frame) {
		case tickFrameQuote:
			f.handleQuoteTick(frame)
		case tickFrameDepth:
			f.handleDepthTick(frame)
		case tickFrameIndex:
			f.handleIndexTick(frame)
		default:
			f.config.logger.Debug("datafeed: ignoring a frame with an unrecognised name", frameName(frame))
		}
	}
}

func (f *Feed) handleAck(frame rawFrame) {
	stat, _ := fieldString(frame, "stat")
	typ, _ := fieldString(frame, "type")
	msg, _ := fieldString(frame, "msg")
	if stat != "Ok" {
		f.config.logger.Warn(fmt.Sprintf("datafeed: %q not acknowledged", typ), msg)
	}
}

func (f *Feed) handleQuoteTick(frame rawFrame) {
	wireKey, ok := frameInstrumentKey(frame)
	if !ok {
		return
	}

	f.mu.Lock()
	scripKey, ok := f.reverse[feedKindQuote][wireKey]
	if !ok {
		f.mu.Unlock()
		return
	}

	state := f.quoteState[scripKey]
	if state == nil {
		state = &Quote{ScripKey: scripKey}
		f.quoteState[scripKey] = state
	}
	tob := f.topOfBook[scripKey]
	if tob == nil {
		tob = &[2]DepthLevel{}
		f.topOfBook[scripKey] = tob
	}

	quoteChanged := mergeQuote(state, frame)
	depthChanged := mergeTopOfBook(&tob[0], &tob[1], frame)

	var quoteCopy Quote
	emitQuote := len(quoteChanged) > 0
	if emitQuote {
		quoteCopy = *state
		quoteCopy.Changed = quoteChanged
		quoteCopy.Extra = copyStringMap(state.Extra)
	}

	var depthCopy Depth
	emitDepth := len(depthChanged) > 0
	if emitDepth {
		depthCopy = Depth{
			ScripKey: scripKey, Levels: 1, Source: DepthSourceQuote,
			Bids: []DepthLevel{tob[0]}, Asks: []DepthLevel{tob[1]},
			Changed: depthChanged,
		}
	}
	f.mu.Unlock()

	if emitQuote {
		f.sendQuote(quoteCopy)
	}
	if emitDepth {
		f.sendDepth(depthCopy)
	}
}

func (f *Feed) handleDepthTick(frame rawFrame) {
	wireKey, ok := frameInstrumentKey(frame)
	if !ok {
		return
	}

	f.mu.Lock()
	scripKey, ok := f.reverse[feedKindDepth][wireKey]
	if !ok {
		f.mu.Unlock()
		return
	}

	book := f.depthState[scripKey]
	if book == nil {
		book = &depthBookState{}
		f.depthState[scripKey] = book
	}
	changed := mergeDepth(&book.bids, &book.asks, frame)

	var out Depth
	emit := len(changed) > 0
	if emit {
		out = Depth{
			ScripKey: scripKey, Levels: 5, Source: DepthSourceDepth,
			Bids: append([]DepthLevel(nil), book.bids[:]...), Asks: append([]DepthLevel(nil), book.asks[:]...),
			Changed: changed,
		}
	}
	f.mu.Unlock()

	if emit {
		f.sendDepth(out)
	}
}

func (f *Feed) handleIndexTick(frame rawFrame) {
	wireKey, ok := frameInstrumentKey(frame)
	if !ok {
		return
	}

	f.mu.Lock()
	scripKey, ok := f.reverse[feedKindIndex][wireKey]
	if !ok {
		f.mu.Unlock()
		return
	}

	state := f.indexState[scripKey]
	if state == nil {
		state = &Index{ScripKey: scripKey}
		f.indexState[scripKey] = state
	}
	changed := mergeIndex(state, frame)

	var out Index
	emit := len(changed) > 0
	if emit {
		out = *state
		out.Changed = changed
		out.Extra = copyStringMap(state.Extra)
	}
	f.mu.Unlock()

	if emit {
		f.sendIndex(out)
	}
}

func copyStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// sendQuote, sendDepth, sendIndex and emitError are all non-blocking: the
// read loop dispatching a tick must never stall waiting on a caller who has
// stopped draining a channel. See feedChannelBuffer's doc comment for why a
// dropped tick does not lose data -- the next tick still carries the fully
// merged state.
func (f *Feed) sendQuote(q Quote) {
	select {
	case f.quoteCh <- q:
	default:
		f.config.logger.Warn("datafeed: Quotes channel full, dropping a snapshot", q.ScripKey)
	}
}

func (f *Feed) sendDepth(d Depth) {
	select {
	case f.depthCh <- d:
	default:
		f.config.logger.Warn("datafeed: Depths channel full, dropping a snapshot", d.ScripKey)
	}
}

func (f *Feed) sendIndex(i Index) {
	select {
	case f.indexCh <- i:
	default:
		f.config.logger.Warn("datafeed: Indices channel full, dropping a snapshot", i.ScripKey)
	}
}

func (f *Feed) emitError(err error) {
	f.config.logger.Error("datafeed", err.Error())
	select {
	case f.errCh <- err:
	default:
	}
}

func (f *Feed) closeChannelsOnce() {
	f.closeOnce.Do(func() {
		close(f.quoteCh)
		close(f.depthCh)
		close(f.indexCh)
		close(f.errCh)
	})
}

// Close shuts the Feed down: it cancels the context Connect started the
// background loop with (the same cancellation ctx's own expiry would
// trigger), closes the socket, waits for the read loop to exit, and closes
// every delivery channel exactly once. Safe to call even if Connect was
// never called, and safe to call more than once.
func (f *Feed) Close() error {
	f.mu.Lock()
	cancel := f.cancel
	conn := f.conn
	f.mu.Unlock()

	if cancel == nil {
		return nil
	}
	cancel()
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "closing")
	}
	f.wg.Wait()
	f.closeChannelsOnce()
	return nil
}
