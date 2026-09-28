// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func retryConfigFor(t *testing.T, ts *TestServer, apply func(*Options)) *resolvedConfig {
	t.Helper()
	opts := Options{BaseURL: ts.URL, AccessToken: "tok", TimeoutMs: Ptr(2000), getenv: noopGetenv}
	if apply != nil {
		apply(&opts)
	}
	config, err := resolveConfig(opts)
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func TestRetryOnServiceUnavailableReturnsEventualSuccess(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, index int) {
		if index == 0 {
			WriteJSON(w, 503, map[string]any{"code": "SERVICE_UNAVAILABLE", "message": "try later"})
			return
		}
		WriteJSON(w, 200, Envelope("r", map[string]any{"ok": true}))
	})
	defer ts.Close()

	data, err := doRequest[map[string]any](context.Background(), retryConfigFor(t, ts, nil), requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	if err != nil {
		t.Fatal(err)
	}
	if data["ok"] != true {
		t.Errorf("data = %v", data)
	}
	if ts.Count() != 2 {
		t.Fatalf("expected 2 requests, got %d", ts.Count())
	}
}

// The most important test in the SDK (carried over verbatim from the Node suite's own words).
func TestNeverRetriesAWriteEvenOn503(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		WriteJSON(w, 503, map[string]any{"code": "SERVICE_UNAVAILABLE", "message": "try later"})
	})
	defer ts.Close()

	_, err := doRequest[map[string]any](context.Background(), retryConfigFor(t, ts, nil), requestOptions{method: "POST", path: "/orders", auth: authBearer, body: map[string]any{}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %v", err)
	}
	if ts.Count() != 1 {
		t.Fatalf("expected exactly 1 request, got %d", ts.Count())
	}
}

func TestNeverRetriesPatchOrDelete(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		WriteJSON(w, 503, map[string]any{"code": "SERVICE_UNAVAILABLE"})
	})
	defer ts.Close()

	config := retryConfigFor(t, ts, nil)
	if _, err := doRequest[map[string]any](context.Background(), config, requestOptions{method: "PATCH", path: "/orders", auth: authBearer, body: map[string]any{}}); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := doRequest[map[string]any](context.Background(), config, requestOptions{method: "DELETE", path: "/orders/1", auth: authBearer}); err == nil {
		t.Fatal("expected an error")
	}
	if ts.Count() != 2 {
		t.Fatalf("expected exactly 2 requests, got %d", ts.Count())
	}
}

func TestDoesNotRetryA4xxThatIsNot429(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		WriteJSON(w, 400, map[string]any{"code": "VALIDATION_ERROR"})
	})
	defer ts.Close()

	_, err := doRequest[map[string]any](context.Background(), retryConfigFor(t, ts, nil), requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	if err == nil {
		t.Fatal("expected an error")
	}
	if ts.Count() != 1 {
		t.Fatalf("expected exactly 1 request, got %d", ts.Count())
	}
}

func TestGivesUpAfterMaxRetriesAndThrowsTheLastError(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		WriteJSON(w, 500, map[string]any{"code": "UNHANDLED_ERROR"})
	})
	defer ts.Close()

	_, err := doRequest[map[string]any](context.Background(), retryConfigFor(t, ts, func(o *Options) { o.MaxRetries = Ptr(2) }),
		requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 500 {
		t.Fatalf("expected a 500 *APIError, got %v", err)
	}
	if ts.Count() != 3 { // 1 initial + 2 retries
		t.Fatalf("expected 3 requests, got %d", ts.Count())
	}
}

func TestHonoursANumericRetryAfterOn429(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, index int) {
		if index == 0 {
			w.Header().Set("Retry-After", "0")
			WriteJSON(w, 429, map[string]any{"code": "SERVICE_UNAVAILABLE"})
			return
		}
		WriteJSON(w, 200, Envelope("r", true))
	})
	defer ts.Close()

	data, err := doRequest[bool](context.Background(), retryConfigFor(t, ts, nil), requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	if err != nil {
		t.Fatal(err)
	}
	if !data {
		t.Error("expected true")
	}
	if ts.Count() != 2 {
		t.Fatalf("expected 2 requests, got %d", ts.Count())
	}
}

// --- retryAfterMs ---

func TestRetryAfterMsReadsSeconds(t *testing.T) {
	if d, ok := retryAfterMs("2", time.Now()); !ok || d != 2*time.Second {
		t.Errorf("retryAfterMs(2) = %v, %v", d, ok)
	}
	if d, ok := retryAfterMs("0", time.Now()); !ok || d != 0 {
		t.Errorf("retryAfterMs(0) = %v, %v", d, ok)
	}
}

func TestRetryAfterMsReadsHTTPDate(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-09-28T10:00:00Z")
	d, ok := retryAfterMs("Mon, 28 Sep 2026 10:00:05 GMT", now)
	if !ok || d != 5*time.Second {
		t.Errorf("retryAfterMs = %v, %v", d, ok)
	}
}

func TestRetryAfterMsNeverNegativeForPastDate(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-09-28T10:00:10Z")
	d, ok := retryAfterMs("Mon, 28 Sep 2026 10:00:05 GMT", now)
	if !ok || d != 0 {
		t.Errorf("retryAfterMs = %v, %v", d, ok)
	}
}

func TestRetryAfterMsIgnoresMissingOrUnparseable(t *testing.T) {
	if _, ok := retryAfterMs("", time.Now()); ok {
		t.Error("expected ok=false for empty header")
	}
	if _, ok := retryAfterMs("soon", time.Now()); ok {
		t.Error("expected ok=false for unparseable header")
	}
}

// --- cancellation ---

func TestCancellationSurfacesCallerAbortWithoutRetrying(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		time.Sleep(1 * time.Second)
		WriteJSON(w, 503, map[string]any{"code": "SERVICE_UNAVAILABLE"})
	})
	defer ts.Close()

	ctx, cancel := context.WithCancelCause(context.Background())
	callerErr := errors.New("caller cancelled")
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel(callerErr)
	}()

	_, err := doRequest[map[string]any](ctx, retryConfigFor(t, ts, nil), requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	if !errors.Is(err, callerErr) {
		t.Fatalf("expected the caller's own error, got %v", err)
	}
	if ts.Count() != 1 {
		t.Fatalf("expected exactly 1 request, got %d", ts.Count())
	}
}

func TestCancellationReportsSDKTimeoutAsTimeoutNotAbort(t *testing.T) {
	done := make(chan struct{})
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { <-done })
	defer ts.Close()
	defer close(done)

	config := retryConfigFor(t, ts, func(o *Options) { o.TimeoutMs = Ptr(50); o.MaxRetries = Ptr(0) })
	_, err := doRequest[bool](context.Background(), config, requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %v", err)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("Error() = %q", err.Error())
	}
}
