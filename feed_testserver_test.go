// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/coder/websocket"
)

// FeedTestServer is a real local WebSocket server (httptest.Server plus
// coder/websocket's own Accept), never a mocked socket, for exercising Feed
// against genuine frames. Handle is invoked once per accepted connection, in
// its own goroutine, and owns that connection's entire lifecycle -- reading
// and writing whatever frames the test needs, in order.
type FeedTestServer struct {
	*httptest.Server

	mu    sync.Mutex
	conns []*websocket.Conn
}

// StartFeedTestServer starts a FeedTestServer. Callers must Close it, which
// also closes every connection it ever accepted.
func StartFeedTestServer(handle func(ctx context.Context, conn *websocket.Conn)) *FeedTestServer {
	fts := &FeedTestServer{}
	fts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		fts.mu.Lock()
		fts.conns = append(fts.conns, conn)
		fts.mu.Unlock()
		handle(r.Context(), conn)
	}))
	return fts
}

// WSURL is the server's URL as a ws:// URL, suitable for Options.WSBaseURL.
func (fts *FeedTestServer) WSURL() string {
	return "ws" + strings.TrimPrefix(fts.Server.URL, "http")
}

// Close closes every accepted connection and shuts down the underlying
// httptest.Server.
func (fts *FeedTestServer) Close() {
	fts.mu.Lock()
	conns := append([]*websocket.Conn(nil), fts.conns...)
	fts.mu.Unlock()
	for _, c := range conns {
		_ = c.CloseNow()
	}
	fts.Server.Close()
}
