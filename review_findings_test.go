// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

// Regression tests pinning guarantees the README/contract make, exercised as
// compositions rather than isolated happy paths — the class of bug the Node
// SDK's own review found: cancellation that only checked the request but not
// the retry sleep, a Retry-After ceiling with no test, a timeout that only
// covered response headers.
package highopenapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// An abort during the retry sleep must stop the call from ever sending its
// next request. An unabortable sleep would let a cancelled call go on to
// issue that request and resolve — exactly what the caller asked not to happen.
func TestCancellationDuringRetrySleepStopsTheNextRequest(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		w.Header().Set("Retry-After", "3")
		WriteJSON(w, 429, map[string]any{"code": "SERVICE_UNAVAILABLE"})
	})
	defer ts.Close()

	ctx, cancel := context.WithCancelCause(context.Background())
	callerErr := errors.New("caller cancelled")
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel(callerErr)
	}()

	config := retryConfigFor(t, ts, nil)
	_, err := doRequest[map[string]any](ctx, config, requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	if !errors.Is(err, callerErr) {
		t.Fatalf("expected the caller's own error, got %v", err)
	}
	if ts.Count() != 1 {
		t.Fatalf("expected exactly 1 request (no retry sent after cancellation), got %d", ts.Count())
	}
}

// A Retry-After above the ceiling must fail fast, not sleep it out.
func TestRetryAfterAboveCeilingFailsFast(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		w.Header().Set("Retry-After", "86400")
		WriteJSON(w, 429, map[string]any{"code": "SERVICE_UNAVAILABLE"})
	})
	defer ts.Close()

	startedAt := time.Now()
	config := retryConfigFor(t, ts, func(o *Options) { o.MaxRetries = Ptr(2); o.MaxRetryDelayMs = Ptr(100) })
	_, err := doRequest[map[string]any](context.Background(), config, requestOptions{method: "GET", path: "/orders/list", auth: authBearer})

	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 429 {
		t.Fatalf("expected a 429 *APIError, got %v", err)
	}
	if time.Since(startedAt) > 2*time.Second {
		t.Fatalf("waited %v — should have given up immediately, not slept toward the header's 86400s", time.Since(startedAt))
	}
	if ts.Count() != 1 {
		t.Fatalf("expected exactly 1 request, got %d", ts.Count())
	}
}

func TestRetryAfterInsideCeilingIsStillHonoured(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, index int) {
		if index == 0 {
			w.Header().Set("Retry-After", "0")
			WriteJSON(w, 429, map[string]any{"code": "SERVICE_UNAVAILABLE"})
			return
		}
		WriteJSON(w, 200, Envelope("r", true))
	})
	defer ts.Close()

	_, err := doRequest[bool](context.Background(), retryConfigFor(t, ts, nil), requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	if err != nil {
		t.Fatal(err)
	}
	if ts.Count() != 2 {
		t.Fatalf("expected 2 requests, got %d", ts.Count())
	}
}

// The timeout must cover the whole exchange, not just the response headers —
// a slow-loris body that stalls after headers arrive must still time out.
func TestTimeoutCoversTheResponseBody(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = fmt.Fprint(w, `{"requestId":"r","da`)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(3 * time.Second) // the rest of the body never arrives within the timeout
	})
	defer ts.Close()

	startedAt := time.Now()
	config := retryConfigFor(t, ts, func(o *Options) { o.TimeoutMs = Ptr(150); o.MaxRetries = Ptr(0) })
	_, err := doRequest[map[string]any](context.Background(), config, requestOptions{method: "GET", path: "/orders/list", auth: authBearer})

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 2*time.Second {
		t.Fatalf("waited %v — the body stall should have timed out around 150ms", elapsed)
	}
}

// Credentials must never be printed through fmt's default struct formatting.
func TestClientCredentialContainment(t *testing.T) {
	client, err := New(Options{AccessToken: "SECRET-TOKEN", APIKey: "SECRET-KEY", getenv: noopGetenv})
	if err != nil {
		t.Fatal(err)
	}
	for name, rendered := range map[string]string{
		"%v":  fmt.Sprintf("%v", client),
		"%+v": fmt.Sprintf("%+v", client),
		"%#v": fmt.Sprintf("%#v", client),
		"%s":  fmt.Sprintf("%s", client),
	} {
		if containsAny(rendered, "SECRET-TOKEN", "SECRET-KEY") {
			t.Fatalf("%s leaked a credential: %s", name, rendered)
		}
	}
}

func TestClientKeepsCredentialsUsableDespiteBeingHidden(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		WriteJSON(w, 200, Envelope("r", map[string]any{"isHoliday": false, "date": "2026-09-25", "exchangeStatus": map[string]any{}}))
	})
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, AccessToken: "SECRET-TOKEN", getenv: noopGetenv})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Market.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ts.Req(0).Header.Get("Authorization") != "Bearer SECRET-TOKEN" {
		t.Fatalf("Authorization = %q", ts.Req(0).Header.Get("Authorization"))
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(sub) > 0 && indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// ENVIRONMENTS must match the pinned spec's servers block, keyed by
// x-environment. Hand-coding the map is fine; letting it drift is not.
func TestEnvironmentsMatchesThePinnedSpec(t *testing.T) {
	doc := LoadPinnedSpec(t)
	fromSpec := map[string]string{}
	for _, s := range doc.Servers {
		origin, err := originOf(s.URL)
		if err != nil {
			t.Fatal(err)
		}
		fromSpec[s.Environment] = origin
	}
	for env, hosts := range Environments {
		if fromSpec[string(env)] != hosts.API {
			t.Errorf("Environments[%q].API = %q, spec says %q", env, hosts.API, fromSpec[string(env)])
		}
	}
	if len(fromSpec) != len(Environments) {
		t.Errorf("spec declares %d servers, Environments has %d entries", len(fromSpec), len(Environments))
	}
}

// Finding 6 (Node review, same class): the facade must not widen a spec enum
// to a bare string. ExpiryType is the generated path-parameter type, and this
// pins that its members still match the pinned spec's enum.
func TestExpiryTypeMatchesThePinnedSpecEnum(t *testing.T) {
	doc := LoadPinnedSpec(t)
	raw, ok := doc.Paths["/scrips/{symbol}/{type}/expiries"]["get"]
	if !ok {
		t.Fatal("expiries operation missing from the pinned spec")
	}
	var op struct {
		Parameters []struct {
			Name   string `json:"name"`
			Schema struct {
				Enum []string `json:"enum"`
			} `json:"schema"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(raw, &op); err != nil {
		t.Fatal(err)
	}
	var enum []string
	for _, p := range op.Parameters {
		if p.Name == "type" {
			enum = p.Schema.Enum
		}
	}
	want := []string{"futures", "options"}
	if len(enum) != len(want) || enum[0] != want[0] || enum[1] != want[1] {
		t.Fatalf("spec enum = %v, want %v", enum, want)
	}
	if string(ExpiryTypeFutures) != "futures" || string(ExpiryTypeOptions) != "options" {
		t.Fatalf("ExpiryType constants = %q / %q", ExpiryTypeFutures, ExpiryTypeOptions)
	}
}
