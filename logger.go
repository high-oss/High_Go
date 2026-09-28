// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"time"
)

// LogLevel controls how much the SDK prints. Setting a level prints that
// level and everything more severe: LogLevelWarn prints warnings and errors,
// LogLevelDebug prints everything. The default is LogLevelSilent — a library
// that prints uninvited is a bad citizen in someone else's application.
type LogLevel string

const (
	LogLevelSilent LogLevel = "silent"
	LogLevelError  LogLevel = "error"
	LogLevelWarn   LogLevel = "warn"
	LogLevelInfo   LogLevel = "info"
	LogLevelDebug  LogLevel = "debug"
)

var logLevelRank = map[LogLevel]int{
	LogLevelSilent: 0,
	LogLevelError:  1,
	LogLevelWarn:   2,
	LogLevelInfo:   3,
	LogLevelDebug:  4,
}

// LogLevels lists every level, least to most verbose.
var LogLevels = []LogLevel{LogLevelSilent, LogLevelError, LogLevelWarn, LogLevelInfo, LogLevelDebug}

func (l LogLevel) valid() bool {
	_, ok := logLevelRank[l]
	return ok
}

func logLevelStrings() []string {
	out := make([]string, len(LogLevels))
	for i, l := range LogLevels {
		out[i] = string(l)
	}
	return out
}

// LogSink is anything that can receive the SDK's log lines. detail is
// genuinely open — a map, a string, or nil — matching every call site's
// loggable payload; there is no single concrete type for it.
type LogSink interface {
	Error(message string, detail any)
	Warn(message string, detail any)
	Info(message string, detail any)
	Debug(message string, detail any)
}

type stderrSink struct{}

func (stderrSink) Error(m string, d any) { printLine(m, d) }
func (stderrSink) Warn(m string, d any)  { printLine(m, d) }
func (stderrSink) Info(m string, d any)  { printLine(m, d) }
func (stderrSink) Debug(m string, d any) { printLine(m, d) }

func printLine(message string, detail any) {
	if detail == nil {
		fmt.Fprintln(os.Stderr, message)
		return
	}
	fmt.Fprintln(os.Stderr, message, detail)
}

// logger drops anything below its configured level.
type logger struct {
	sink      LogSink
	threshold int
}

func newLogger(level LogLevel, sink LogSink) *logger {
	if sink == nil {
		sink = stderrSink{}
	}
	return &logger{sink: sink, threshold: logLevelRank[level]}
}

func (l *logger) enabled(level LogLevel) bool { return logLevelRank[level] <= l.threshold }

func (l *logger) Error(message string, detail any) {
	if l.enabled(LogLevelError) {
		l.sink.Error(format(LogLevelError, message), detail)
	}
}

func (l *logger) Warn(message string, detail any) {
	if l.enabled(LogLevelWarn) {
		l.sink.Warn(format(LogLevelWarn, message), detail)
	}
}

func (l *logger) Info(message string, detail any) {
	if l.enabled(LogLevelInfo) {
		l.sink.Info(format(LogLevelInfo, message), detail)
	}
}

func (l *logger) Debug(message string, detail any) {
	if l.enabled(LogLevelDebug) {
		l.sink.Debug(format(LogLevelDebug, message), detail)
	}
}

// format renders "2026-09-28T17:05:12.345Z WARN  message" — sortable, and
// aligned in a terminal (the level is padded to 5 characters).
func format(level LogLevel, message string) string {
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	return fmt.Sprintf("%s %-5s %s", ts, upper(string(level)), message)
}

func upper(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// sensitiveParams must never reach a log sink. tOtp is a live second factor
// and the token parameters are bearer credentials.
var sensitiveParams = map[string]bool{
	"tOtp": true, "apiKey": true, "accessToken": true, "tokenId": true, "stepToken": true,
}

// redactURL replaces sensitive query parameter values with REDACTED. A URL
// that fails to parse is returned unchanged rather than throwing inside a log
// call, which would turn an observability feature into an outage.
func redactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	query := parsed.Query()
	touched := false
	for key := range query {
		if sensitiveParams[key] {
			query.Set(key, "REDACTED")
			touched = true
		}
	}
	if !touched {
		return raw
	}

	// Preserve query-parameter order as far as Go's url.Values allows; exact
	// byte-for-byte order is not a contract guarantee, only that no secret
	// leaks and every non-secret parameter survives.
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	encoded := url.Values{}
	for _, k := range keys {
		for _, v := range query[k] {
			encoded.Add(k, v)
		}
	}
	parsed.RawQuery = encoded.Encode()
	return parsed.String()
}

// sensitiveKeys are object keys whose values must never be logged, whatever
// nesting they sit at. Query parameters are handled by redactURL; this covers
// request and response bodies.
var sensitiveKeys = map[string]bool{
	"tOtp": true, "totp": true, "otp": true, "apiKey": true, "accessToken": true,
	"refreshToken": true, "tokenId": true, "stepToken": true, "password": true,
	"pin": true, "authorization": true,
}

// maxLoggedJSON: above this, a logged payload is replaced by a note rather
// than printed.
const maxLoggedJSON = 1500

// redactBody deep-copies a JSON-shaped payload for logging, masking
// credential-shaped keys at any nesting depth and replacing anything
// oversized with a note. raw is typically the bytes about to be sent, or just
// received; it is re-parsed here purely for the purpose of logging.
func redactBody(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		// Not JSON — still worth a bounded, redaction-free peek.
		s := string(raw)
		if len(s) > 200 {
			s = s[:200]
		}
		return s
	}

	masked := maskValue(value)
	serialised, err := json.Marshal(masked)
	if err != nil {
		return "[unserialisable]"
	}
	if len(serialised) > maxLoggedJSON {
		return fmt.Sprintf("[truncated: %d bytes of JSON]", len(serialised))
	}
	return masked
}

func maskValue(value any) any {
	switch v := value.(type) {
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = maskValue(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, nested := range v {
			if sensitiveKeys[k] {
				out[k] = "REDACTED"
			} else {
				out[k] = maskValue(nested)
			}
		}
		return out
	default:
		return v
	}
}
