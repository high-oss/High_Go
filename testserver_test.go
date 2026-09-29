// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
)

// ReceivedRequest is one exchange a TestServer captured.
type ReceivedRequest struct {
	Method string
	URL    string
	Header http.Header
	Body   string
}

// TestServer is a real HTTP server on a random port. Every test in this
// module exercises an actual exchange against it rather than a mocked
// transport, so a change in how the SDK builds requests is caught.
type TestServer struct {
	*httptest.Server

	mu       sync.Mutex
	Requests []ReceivedRequest
}

// Handler receives the request plus its zero-based index among every request
// this server has received so far, so a test can make the Nth response
// different from the first (e.g. "fail once, then succeed").
type Handler func(w http.ResponseWriter, r *http.Request, index int)

// StartTestServer starts a TestServer. Callers must Close it.
func StartTestServer(handler Handler) *TestServer {
	ts := &TestServer{}
	ts.Server = httptest.NewServer(ts.wrap(handler))
	return ts
}

// StartTLSTestServer is StartTestServer over TLS with a self-signed
// certificate, for the one case a plain-HTTP local server cannot stand in
// for: the instrument list's https-only validation (Review Focus 3) refuses
// anything else, so proving a download actually succeeds needs a real https
// exchange. ts.Client() (embedded from *httptest.Server) is pre-configured
// to trust the certificate.
func StartTLSTestServer(handler Handler) *TestServer {
	ts := &TestServer{}
	ts.Server = httptest.NewTLSServer(ts.wrap(handler))
	return ts
}

func (ts *TestServer) wrap(handler Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ts.mu.Lock()
		index := len(ts.Requests)
		ts.Requests = append(ts.Requests, ReceivedRequest{
			Method: r.Method,
			URL:    r.URL.String(),
			Header: r.Header.Clone(),
			Body:   string(body),
		})
		ts.mu.Unlock()
		handler(w, r, index)
	}
}

// Count returns how many requests this server has received so far.
func (ts *TestServer) Count() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.Requests)
}

// Req returns the i-th received request.
func (ts *TestServer) Req(i int) ReceivedRequest {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.Requests[i]
}

// WriteJSON writes a JSON response.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// Envelope builds the {requestId, data} shape every successful response carries.
func Envelope(requestID string, data any) map[string]any {
	return map[string]any{"requestId": requestID, "data": data}
}
