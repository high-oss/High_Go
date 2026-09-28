// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func configFor(t *testing.T, ts *TestServer, apply func(*Options)) *resolvedConfig {
	t.Helper()
	opts := Options{BaseURL: ts.URL, AccessToken: "tok", APIKey: "key", getenv: noopGetenv}
	if apply != nil {
		apply(&opts)
	}
	config, err := resolveConfig(opts)
	if err != nil {
		t.Fatal(err)
	}
	return config
}

type orderIDBody struct {
	OrderID string `json:"orderId"`
}

func TestRequestUnwrapsTheEnvelope(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		WriteJSON(w, 200, Envelope("a1b2c3", orderIDBody{OrderID: "1"}))
	})
	defer ts.Close()

	data, err := doRequest[orderIDBody](context.Background(), configFor(t, ts, nil), requestOptions{
		method: "GET", path: "/orders/list", auth: authBearer,
	})
	if err != nil {
		t.Fatal(err)
	}
	if data.OrderID != "1" {
		t.Errorf("OrderID = %q", data.OrderID)
	}
}

func TestRequestSendsBearerTokenAndNoAPIKey(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { WriteJSON(w, 200, Envelope("r", true)) })
	defer ts.Close()

	_, err := doRequest[bool](context.Background(), configFor(t, ts, nil), requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	if err != nil {
		t.Fatal(err)
	}
	req := ts.Req(0)
	if req.Header.Get("Authorization") != "Bearer tok" {
		t.Errorf("Authorization = %q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("x-api-key") != "" {
		t.Errorf("x-api-key should be empty, got %q", req.Header.Get("x-api-key"))
	}
}

func TestRequestSendsAPIKeyAndNoBearer(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { WriteJSON(w, 200, Envelope("r", true)) })
	defer ts.Close()

	_, err := doRequest[bool](context.Background(), configFor(t, ts, nil), requestOptions{method: "GET", path: "/auth/generate-access-token", auth: authAPIKey})
	if err != nil {
		t.Fatal(err)
	}
	req := ts.Req(0)
	if req.Header.Get("x-api-key") != "key" {
		t.Errorf("x-api-key = %q", req.Header.Get("x-api-key"))
	}
	if req.Header.Get("Authorization") != "" {
		t.Errorf("Authorization should be empty, got %q", req.Header.Get("Authorization"))
	}
}

func TestRequestFailsBeforeSendingWhenCredentialMissing(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { WriteJSON(w, 200, Envelope("r", true)) })
	defer ts.Close()

	config := configFor(t, ts, func(o *Options) { o.AccessToken = ""; o.APIKey = "" })
	_, err := doRequest[bool](context.Background(), config, requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	if err == nil || !strings.Contains(err.Error(), "accessToken") {
		t.Fatalf("expected an accessToken error, got %v", err)
	}
	if ts.Count() != 0 {
		t.Fatalf("expected no request to be sent, got %d", ts.Count())
	}
}

func TestRequestSerialisesJSONBody(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { WriteJSON(w, 200, Envelope("r", map[string]any{})) })
	defer ts.Close()

	_, err := doRequest[map[string]any](context.Background(), configFor(t, ts, nil), requestOptions{
		method: "POST", path: "/orders", auth: authBearer, body: map[string]any{"quantity": 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := ts.Req(0)
	if req.Body != `{"quantity":10}` {
		t.Errorf("body = %q", req.Body)
	}
	if !strings.Contains(req.Header.Get("Content-Type"), "application/json") {
		t.Errorf("Content-Type = %q", req.Header.Get("Content-Type"))
	}
}

func TestRequestAppendsQueryParameters(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { WriteJSON(w, 200, Envelope("r", true)) })
	defer ts.Close()

	_, err := doRequest[bool](context.Background(), configFor(t, ts, nil), requestOptions{
		method: "GET", path: "/auth/generate-access-token", auth: authAPIKey,
		query: map[string]string{"clientId": "C1", "tOtp": "123456"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := ts.Req(0)
	if !strings.Contains(req.URL, "clientId=C1") || !strings.Contains(req.URL, "tOtp=123456") {
		t.Errorf("URL = %q", req.URL)
	}
}

func TestRequestMapsErrorEnvelopeToAPIError(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		WriteJSON(w, 400, map[string]any{"requestId": "a1b2c3", "code": "ORDER_REJECTED", "message": "Insufficient funds"})
	})
	defer ts.Close()

	_, err := doRequest[map[string]any](context.Background(), configFor(t, ts, nil), requestOptions{method: "POST", path: "/orders", auth: authBearer})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %v", err)
	}
	if apiErr.Code != "ORDER_REJECTED" || apiErr.RequestID != "a1b2c3" {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
}

func TestRequestTurnsNonJSONErrorBodyIntoAPIError(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(502)
		_, _ = w.Write([]byte("<html><body>502 Bad Gateway</body></html>"))
	})
	defer ts.Close()

	_, err := doRequest[map[string]any](context.Background(), configFor(t, ts, func(o *Options) { o.MaxRetries = Ptr(0) }),
		requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %v (%T)", err, err)
	}
	if apiErr.Status != 502 {
		t.Errorf("Status = %d", apiErr.Status)
	}
}

func TestRequestTurnsEmptyBodyIntoZeroValue(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { w.WriteHeader(204) })
	defer ts.Close()

	data, err := doRequest[*orderIDBody](context.Background(), configFor(t, ts, nil), requestOptions{method: "DELETE", path: "/orders/1", auth: authBearer})
	if err != nil {
		t.Fatal(err)
	}
	if data != nil {
		t.Errorf("expected nil, got %+v", data)
	}
}

func TestRequestTimesOutRatherThanHanging(t *testing.T) {
	// The handler blocks on `done` rather than `select {}` forever: httptest's
	// Server.Close waits for every outstanding handler to return, and a
	// handler that never returns would hang the test's cleanup along with it.
	done := make(chan struct{})
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		<-done // never respond within the test's timeout
	})
	defer ts.Close()
	defer close(done)

	config := configFor(t, ts, func(o *Options) { o.TimeoutMs = Ptr(50); o.MaxRetries = Ptr(0) })
	_, err := doRequest[bool](context.Background(), config, requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %v", err)
	}
	if apiErr.Status != 0 {
		t.Errorf("Status = %d", apiErr.Status)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestRequestSendsUserAgent(t *testing.T) {
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { WriteJSON(w, 200, Envelope("r", true)) })
	defer ts.Close()

	_, err := doRequest[bool](context.Background(), configFor(t, ts, nil), requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ts.Req(0).Header.Get("User-Agent"), "high-sdk-go") {
		t.Errorf("User-Agent = %q", ts.Req(0).Header.Get("User-Agent"))
	}
}

// --- logging ---

func TestLoggingPrintsNothingByDefault(t *testing.T) {
	lines, sink := newCaptureSink()
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { WriteJSON(w, 200, Envelope("r", true)) })
	defer ts.Close()

	config := configFor(t, ts, func(o *Options) { o.LogSink = sink })
	_, err := doRequest[bool](context.Background(), config, requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	if err != nil {
		t.Fatal(err)
	}
	if len(*lines) != 0 {
		t.Fatalf("expected no lines, got %v", *lines)
	}
}

func TestLoggingLogsRequestAndStatusAtDebug(t *testing.T) {
	lines, sink := newCaptureSink()
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { WriteJSON(w, 200, Envelope("a1b2c3", true)) })
	defer ts.Close()

	config := configFor(t, ts, func(o *Options) { o.LogLevel = LogLevelDebug; o.LogSink = sink })
	_, err := doRequest[bool](context.Background(), config, requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, l := range *lines {
		all.WriteString(l.message)
		all.WriteString("\n")
	}
	joined := all.String()
	for _, want := range []string{"GET", "orders/list", "200"} {
		if !strings.Contains(joined, want) {
			t.Errorf("log output missing %q:\n%s", want, joined)
		}
	}
}

// The reason logging goes through one module: a caller shipping debug logs to
// an aggregator must not ship their credentials with them.
func TestLoggingNeverLogsCredentials(t *testing.T) {
	lines, sink := newCaptureSink()
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) { WriteJSON(w, 200, Envelope("r", true)) })
	defer ts.Close()

	config := configFor(t, ts, func(o *Options) {
		o.LogLevel = LogLevelDebug
		o.LogSink = sink
		o.AccessToken = "SECRET-TOKEN"
		o.APIKey = "SECRET-KEY"
	})
	_, err := doRequest[bool](context.Background(), config, requestOptions{
		method: "GET", path: "/auth/generate-access-token", auth: authAPIKey,
		query: map[string]string{"clientId": "C1", "tOtp": "123456"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, l := range *lines {
		all.WriteString(l.message)
		if l.detail != nil {
			all.WriteString(" ")
			all.WriteString(fmt.Sprint(l.detail))
		}
	}
	joined := all.String()
	for _, secret := range []string{"SECRET-TOKEN", "SECRET-KEY", "123456"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("log output leaked %q:\n%s", secret, joined)
		}
	}
	if !strings.Contains(joined, "REDACTED") {
		t.Fatalf("log output never mentions REDACTED:\n%s", joined)
	}
}

func TestLoggingWarnsOnRetry(t *testing.T) {
	lines, sink := newCaptureSink()
	ts := StartTestServer(func(w http.ResponseWriter, r *http.Request, index int) {
		if index == 0 {
			WriteJSON(w, 503, map[string]any{"code": "SERVICE_UNAVAILABLE"})
			return
		}
		WriteJSON(w, 200, Envelope("r", true))
	})
	defer ts.Close()

	config := configFor(t, ts, func(o *Options) { o.LogLevel = LogLevelWarn; o.LogSink = sink })
	_, err := doRequest[bool](context.Background(), config, requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, l := range *lines {
		all.WriteString(strings.ToLower(l.message))
	}
	if !strings.Contains(all.String(), "retry") {
		t.Fatalf("expected a retry warning, got %v", *lines)
	}
}
