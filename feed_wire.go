// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"encoding/json"
	"fmt"
	"strings"
)

// feedKind is which of the feed's three channels an instrument is on. It is
// unexported: the public surface exposes dedicated methods per kind
// (SubscribeQuotes / SubscribeDepth / SubscribeIndices, and their
// Unsubscribe/Snapshot counterparts) rather than taking this as a parameter,
// so a caller can never pass an index key to the quote method by mistake and
// have it silently do the wrong thing. The wire protocol and the internal
// merge/subscription state are still naturally organised around three
// kinds, so this stays one small type rather than three parallel copies of
// the same bookkeeping.
type feedKind int

const (
	feedKindQuote feedKind = iota
	feedKindDepth
	feedKindIndex
)

// wirePrefix is the frame-type prefix this kind's requests use: "mw" market
// watch, "dp" depth, "if" indices (datafeed plan). Suffixed "s" to
// subscribe, "u" to unsubscribe, "sp" to snapshot.
func (k feedKind) wirePrefix() string {
	switch k {
	case feedKindQuote:
		return "mw"
	case feedKindDepth:
		return "dp"
	case feedKindIndex:
		return "if"
	default:
		return ""
	}
}

func (k feedKind) String() string {
	switch k {
	case feedKindQuote:
		return "quote"
	case feedKindDepth:
		return "depth"
	case feedKindIndex:
		return "index"
	default:
		return "unknown"
	}
}

// tickFrameName is the `name` field on an individual element of a pushed
// tick array, which is how routing happens -- "not by guessing from which
// keys are present" (plan, Phase 3).
const (
	tickFrameQuote = "sf"
	tickFrameDepth = "dp"
	tickFrameIndex = "if"
)

// authRequest is the frame that must be sent first, and the only thing sent
// before its acknowledgement arrives. mode is deliberately never a field
// here -- it is filled in server-side from the caller's data plan (plan,
// Phase 0: "`mode` follows the data plan"), so this SDK cannot claim a tier
// it does not hold.
type authRequest struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionid"`
}

func newAuthRequest(accessToken string) authRequest {
	return authRequest{Type: "cn", SessionID: accessToken}
}

// authAck is the `cn` response: a bare JSON object (not an array, unlike
// every other ack), carrying the two limits on success or stCode/msg on
// failure.
type authAck struct {
	Stat            string `json:"stat"`
	Type            string `json:"type"`
	Msg             string `json:"msg"`
	StCode          int    `json:"stCode"`
	MaxScripPerConn int    `json:"maxScripPerConn"`
	MaxScripPerReq  int    `json:"maxScripPerReq"`
	SType           string `json:"sType"`
}

func (a authAck) ok() bool { return a.Stat == "Ok" }

// subRequest is one subscribe/unsubscribe/snapshot request. Scrips is
// "&"-joined per the plan; ChannelNum is fixed at 1 -- this SDK does not
// expose channel pause/resume/full/lite, so one channel per Feed is enough.
type subRequest struct {
	Type       string `json:"type"`
	Scrips     string `json:"scrips"`
	ChannelNum int    `json:"channelnum"`
}

func newSubRequest(kind feedKind, suffix string, scrips []string) subRequest {
	return subRequest{Type: kind.wirePrefix() + suffix, Scrips: strings.Join(scrips, "&"), ChannelNum: feedChannelNum}
}

// feedChannelNum is the one channel every request uses.
const feedChannelNum = 1

// FeedAuthErrorKind names the cases of a NotOk auth acknowledgement a caller
// can act on distinctly (datafeed plan, Phase 0).
type FeedAuthErrorKind int

const (
	// FeedAuthErrorUnknown is any NotOk this SDK does not recognise. StCode
	// and Msg are always preserved intact regardless of Kind -- see
	// FeedAuthError's doc comment for why Kind is a best-effort
	// classification, not a closed, fully-confirmed enum.
	FeedAuthErrorUnknown FeedAuthErrorKind = iota
	// FeedAuthErrorMalformedRequest is vendor stCode 11002 ("invalid field
	// count"), the one non-11001 NotOk code the apiDoc itself documents.
	FeedAuthErrorMalformedRequest
	// FeedAuthErrorNoDataPlan is a missing/inactive Data API subscription:
	// the caller needs to subscribe before this token can open a feed
	// connection.
	FeedAuthErrorNoDataPlan
	// FeedAuthErrorInvalidToken is an invalid or expired access token: the
	// caller needs a fresh one (there is no token refresh in this SDK --
	// see the shared contract §3).
	FeedAuthErrorInvalidToken
)

func (k FeedAuthErrorKind) String() string {
	switch k {
	case FeedAuthErrorMalformedRequest:
		return "malformed request"
	case FeedAuthErrorNoDataPlan:
		return "no data plan"
	case FeedAuthErrorInvalidToken:
		return "invalid token"
	default:
		return "unknown"
	}
}

// FeedAuthError is a NotOk `cn` acknowledgement. It is never retried and
// never triggers a reconnect -- the server will keep refusing (plan, Phase
// 0) -- whether it is the very first Connect or a re-authentication after a
// transport reconnect.
//
// Kind classifies the two named cases the plan calls for: a missing data
// plan and an invalid token. The exact gateway stCode for each is not yet
// confirmed -- the plan itself says so ("Needed to finish this: the exact
// stCode our gateway returns for a missing data plan... Until it is known,
// an unrecognised NotOk surfaces with its code and message intact"). Only
// stCode 11002 is vendor-documented. For the other two, Kind is a
// best-effort classification from Msg's wording, done here rather than left
// for every caller to reimplement; StCode and Msg are always exact and
// complete regardless of whether Kind guessed right, so nothing is ever
// swallowed by a wrong guess.
type FeedAuthError struct {
	StCode int
	Msg    string
	Kind   FeedAuthErrorKind
}

func (e *FeedAuthError) Error() string {
	return fmt.Sprintf("datafeed: auth failed (%s, stCode %d): %s", e.Kind, e.StCode, e.Msg)
}

func classifyFeedAuthError(stCode int, msg string) *FeedAuthError {
	kind := FeedAuthErrorUnknown
	switch {
	case stCode == 11002:
		kind = FeedAuthErrorMalformedRequest
	default:
		lower := strings.ToLower(msg)
		switch {
		case strings.Contains(lower, "data plan") || strings.Contains(lower, "subscription") || strings.Contains(lower, "plan"):
			kind = FeedAuthErrorNoDataPlan
		case strings.Contains(lower, "token") || strings.Contains(lower, "session") || strings.Contains(lower, "expired") || strings.Contains(lower, "auth"):
			kind = FeedAuthErrorInvalidToken
		}
	}
	return &FeedAuthError{StCode: stCode, Msg: msg, Kind: kind}
}

// FeedLimitError is returned when a subscription would exceed a limit the
// auth ack declared -- maxScripPerConn (the whole connection) or a request
// this SDK would otherwise have had to split further than is sensible. It
// is never sent to the server; the SDK refuses locally instead of letting
// the server drop the connection over it (plan, Phase 0).
type FeedLimitError struct {
	Limit     string // "maxScripPerConn"
	Max       int
	Requested int
}

func (e *FeedLimitError) Error() string {
	return fmt.Sprintf("datafeed: %s is %d; this subscription would need %d", e.Limit, e.Max, e.Requested)
}

// decodeAck parses a `cn`-style bare-object acknowledgement.
func decodeAck(data []byte) (authAck, error) {
	var ack authAck
	if err := json.Unmarshal(data, &ack); err != nil {
		return authAck{}, fmt.Errorf("datafeed: could not parse the auth acknowledgement: %w", err)
	}
	return ack, nil
}

// splitFrames decodes one inbound message into its individual tick/ack
// elements. Ticks and sub/unsub acks both arrive as a JSON array; splitFrames
// also tolerates a bare object (defensively -- only the `cn` ack is
// documented as one, but a lone element is handled the same way either
// shape arrives).
func splitFrames(data []byte) ([]rawFrame, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var elems []rawFrame
		if err := json.Unmarshal(data, &elems); err != nil {
			return nil, fmt.Errorf("datafeed: could not parse a frame array: %w", err)
		}
		return elems, nil
	}
	var one rawFrame
	if err := json.Unmarshal(data, &one); err != nil {
		return nil, fmt.Errorf("datafeed: could not parse a frame: %w", err)
	}
	return []rawFrame{one}, nil
}

func frameName(f rawFrame) string {
	s, _ := fieldString(f, "name")
	return s
}

func frameIsAck(f rawFrame) bool {
	_, ok := f["stat"]
	return ok
}

// frameInstrumentKey is the "<e>|<tk>" identifier a tick names itself by --
// the same shape as translatedFeedKey.wireScrip(), so the Feed's
// subscription reverse-index can be keyed identically for both directions.
func frameInstrumentKey(f rawFrame) (string, bool) {
	e, okE := fieldString(f, "e")
	tk, okTk := fieldString(f, "tk")
	if !okE || !okTk {
		return "", false
	}
	return strings.TrimSpace(e) + "|" + strings.TrimSpace(tk), true
}
