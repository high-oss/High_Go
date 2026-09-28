// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

type capturedLine struct {
	level   string
	message string
	detail  any
}

type capturingSink struct {
	lines *[]capturedLine
}

func (s capturingSink) push(level, m string, d any) {
	*s.lines = append(*s.lines, capturedLine{level: level, message: m, detail: d})
}
func (s capturingSink) Error(m string, d any) { s.push("error", m, d) }
func (s capturingSink) Warn(m string, d any)  { s.push("warn", m, d) }
func (s capturingSink) Info(m string, d any)  { s.push("info", m, d) }
func (s capturingSink) Debug(m string, d any) { s.push("debug", m, d) }

func newCaptureSink() (*[]capturedLine, LogSink) {
	lines := &[]capturedLine{}
	return lines, capturingSink{lines: lines}
}

func TestLogLevelsOrdersLeastToMostVerbose(t *testing.T) {
	want := []LogLevel{LogLevelSilent, LogLevelError, LogLevelWarn, LogLevelInfo, LogLevelDebug}
	if len(LogLevels) != len(want) {
		t.Fatalf("LogLevels = %v", LogLevels)
	}
	for i := range want {
		if LogLevels[i] != want[i] {
			t.Fatalf("LogLevels[%d] = %q, want %q", i, LogLevels[i], want[i])
		}
	}
}

func TestLoggerPrintsNothingAtSilent(t *testing.T) {
	lines, sink := newCaptureSink()
	log := newLogger(LogLevelSilent, sink)
	log.Error("boom", nil)
	log.Warn("hmm", nil)
	log.Info("fyi", nil)
	log.Debug("detail", nil)
	if len(*lines) != 0 {
		t.Fatalf("expected no lines, got %v", *lines)
	}
}

func TestLoggerPrintsChosenLevelAndMoreSevere(t *testing.T) {
	lines, sink := newCaptureSink()
	log := newLogger(LogLevelWarn, sink)
	log.Error("boom", nil)
	log.Warn("hmm", nil)
	log.Info("fyi", nil)
	log.Debug("detail", nil)
	if len(*lines) != 2 || (*lines)[0].level != "error" || (*lines)[1].level != "warn" {
		t.Fatalf("lines = %v", *lines)
	}
}

func TestLoggerPrintsEverythingAtDebug(t *testing.T) {
	lines, sink := newCaptureSink()
	log := newLogger(LogLevelDebug, sink)
	log.Error("a", nil)
	log.Warn("b", nil)
	log.Info("c", nil)
	log.Debug("d", nil)
	if len(*lines) != 4 {
		t.Fatalf("lines = %v", *lines)
	}
}

func TestLoggerNeverEvaluatesASuppressedLevel(t *testing.T) {
	_, sink := newCaptureSink()
	log := newLogger(LogLevelError, sink)
	if log.enabled(LogLevelDebug) {
		t.Error("debug should not be enabled")
	}
	if !log.enabled(LogLevelError) {
		t.Error("error should be enabled")
	}
}

func TestRedactURLMasksTOtpAndCredentials(t *testing.T) {
	url := "https://openapi.high.live/v1/auth/generate-access-token?clientId=C1&tOtp=123456"
	safe := redactURL(url)
	if strings.Contains(safe, "123456") {
		t.Errorf("safe URL still contains the TOTP: %q", safe)
	}
	if !strings.Contains(safe, "tOtp=REDACTED") {
		t.Errorf("safe URL missing tOtp=REDACTED: %q", safe)
	}
	if !strings.Contains(safe, "clientId=C1") {
		t.Errorf("safe URL lost clientId: %q", safe)
	}
}

func TestRedactURLMasksEverySensitiveParam(t *testing.T) {
	url := "https://h.example/v1/x?apiKey=k&accessToken=t&tokenId=ti&stepToken=st&consentId=c"
	safe := redactURL(url)
	for _, secret := range []string{"=k", "=t&", "=ti", "=st"} {
		if strings.Contains(safe, secret) {
			t.Errorf("safe URL leaked %q: %q", secret, safe)
		}
	}
	if !strings.Contains(safe, "consentId=c") {
		t.Errorf("safe URL lost consentId: %q", safe)
	}
}

func TestRedactURLLeavesCleanURLUntouched(t *testing.T) {
	url := "https://openapi.high.live/v1/orders/list"
	if redactURL(url) != url {
		t.Errorf("redactURL changed a clean URL: %q", redactURL(url))
	}
}

func TestRedactURLReturnsMalformedURLUnchanged(t *testing.T) {
	if redactURL("not a url") != "not a url" {
		t.Error("redactURL should return an unparseable string unchanged")
	}
}

func TestTimestampsPrefixEveryLine(t *testing.T) {
	lines, sink := newCaptureSink()
	newLogger(LogLevelDebug, sink).Warn("something happened", nil)
	re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z WARN {2}something happened$`)
	if !re.MatchString((*lines)[0].message) {
		t.Fatalf("message = %q", (*lines)[0].message)
	}
}

func TestLevelsArePaddedToAlign(t *testing.T) {
	lines, sink := newCaptureSink()
	log := newLogger(LogLevelDebug, sink)
	log.Error("a", nil)
	log.Debug("b", nil)
	col := func(msg string) int {
		// position right after the "Z " following the timestamp, i.e. where the level starts
		i := strings.Index(msg, "Z ")
		return i + 2
	}
	if col((*lines)[0].message) != col((*lines)[1].message) {
		t.Fatalf("levels not aligned: %q / %q", (*lines)[0].message, (*lines)[1].message)
	}
}

func TestRedactBodyMasksCredentialShapedKeysAtAnyDepth(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"tradingSymbol": "RELIANCE-EQ",
		"nested": map[string]any{
			"accessToken": "SECRET", "tOtp": "123456", "apiKey": "K", "password": "p", "pin": "1234",
		},
	})
	safe, ok := redactBody(raw).(map[string]any)
	if !ok {
		t.Fatalf("redactBody did not return a map: %v", redactBody(raw))
	}
	if safe["tradingSymbol"] != "RELIANCE-EQ" {
		t.Errorf("tradingSymbol = %v", safe["tradingSymbol"])
	}
	nested, ok := safe["nested"].(map[string]any)
	if !ok {
		t.Fatal("nested is not a map")
	}
	for _, key := range []string{"accessToken", "tOtp", "apiKey", "password", "pin"} {
		if nested[key] != "REDACTED" {
			t.Errorf("nested[%q] = %v, want REDACTED", key, nested[key])
		}
	}
}

func TestRedactBodyWalksArrays(t *testing.T) {
	raw, _ := json.Marshal([]map[string]any{{"tOtp": "1"}, {"quantity": 10}})
	safe, ok := redactBody(raw).([]any)
	if !ok || len(safe) != 2 {
		t.Fatalf("redactBody = %v", redactBody(raw))
	}
	first := safe[0].(map[string]any)
	if first["tOtp"] != "REDACTED" {
		t.Errorf("first.tOtp = %v", first["tOtp"])
	}
}

func TestRedactBodyTruncatesLargePayloads(t *testing.T) {
	symbols := make([]string, 500)
	for i := range symbols {
		symbols[i] = "SYM"
	}
	raw, _ := json.Marshal(map[string]any{"symbols": symbols})
	safe := redactBody(raw)
	serialised, _ := json.Marshal(safe)
	if len(serialised) >= 2000 {
		t.Fatalf("expected truncation, got %d bytes", len(serialised))
	}
	if s, ok := safe.(string); !ok || !strings.Contains(s, "truncated") {
		t.Fatalf("expected a truncation note, got %v", safe)
	}
}

func TestRedactBodyPassesEmptyThrough(t *testing.T) {
	if redactBody(nil) != nil {
		t.Error("expected nil for empty input")
	}
}
