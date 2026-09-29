// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func okAuthAck(maxScripPerConn, maxScripPerReq int) []byte {
	b, _ := json.Marshal(map[string]any{
		"stat": "Ok", "type": "cn", "msg": "successful", "stCode": 200,
		"maxScripPerConn": maxScripPerConn, "maxScripPerReq": maxScripPerReq, "sType": "v2.0",
	})
	return b
}

func readJSONFrame(ctx context.Context, conn *websocket.Conn) (map[string]any, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// Nothing may be sent before the auth acknowledgement arrives, and the auth
// frame itself must never carry "mode" -- that is filled in server-side from
// the data plan (datafeed plan, Phase 0).
func TestFeedAuthBeforeSubscribeOrdering(t *testing.T) {
	var mu sync.Mutex
	var order []string
	authDone := make(chan struct{})

	srv := StartFeedTestServer(func(ctx context.Context, conn *websocket.Conn) {
		req, err := readJSONFrame(ctx, conn)
		if err != nil {
			return
		}
		if req["type"] != "cn" {
			t.Errorf("first frame type = %v, want %q", req["type"], "cn")
		}
		if req["sessionid"] != "tok123" {
			t.Errorf("sessionid = %v, want tok123", req["sessionid"])
		}
		if _, hasMode := req["mode"]; hasMode {
			t.Error("auth frame must not carry \"mode\" -- it follows the data plan server-side")
		}
		mu.Lock()
		order = append(order, "cn")
		mu.Unlock()

		if err := conn.Write(ctx, websocket.MessageText, okAuthAck(500, 200)); err != nil {
			return
		}
		close(authDone)

		req2, err := readJSONFrame(ctx, conn)
		if err != nil {
			return
		}
		mu.Lock()
		order = append(order, fmt.Sprint(req2["type"]))
		mu.Unlock()
		<-ctx.Done()
	})
	defer srv.Close()

	feed, err := NewFeed(Options{WSBaseURL: srv.WSURL(), AccessToken: "tok123"})
	if err != nil {
		t.Fatal(err)
	}
	defer feed.Close()

	if err := feed.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-authDone:
	default:
		t.Fatal("Connect returned before the auth acknowledgement was received")
	}

	if err := feed.SubscribeQuotes(context.Background(), []string{"NSE@2885"}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		got := append([]string(nil), order...)
		mu.Unlock()
		if len(got) >= 2 {
			if got[0] != "cn" || got[1] != "mws" {
				t.Fatalf("order = %v, want [cn mws]", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("order = %v, want [cn mws]", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A NotOk acknowledgement is a typed error carrying stCode/msg, classified
// into the named cases the plan calls for, and must never be retried or
// reconnected -- the server will keep refusing.
func TestFeedAuthNotOkNotRetried(t *testing.T) {
	var connections int32
	srv := StartFeedTestServer(func(ctx context.Context, conn *websocket.Conn) {
		atomic.AddInt32(&connections, 1)
		if _, err := readJSONFrame(ctx, conn); err != nil {
			return
		}
		b, _ := json.Marshal(map[string]any{
			"stat": "NotOk", "type": "cn", "msg": "No active data plan for this account", "stCode": 40101,
		})
		conn.Write(ctx, websocket.MessageText, b)
	})
	defer srv.Close()

	feed, err := NewFeed(Options{WSBaseURL: srv.WSURL(), AccessToken: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	defer feed.Close()

	err = feed.Connect(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	var authErr *FeedAuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("err = %v (%T), want *FeedAuthError", err, err)
	}
	if authErr.StCode != 40101 {
		t.Errorf("StCode = %d, want 40101", authErr.StCode)
	}
	if authErr.Kind != FeedAuthErrorNoDataPlan {
		t.Errorf("Kind = %v, want FeedAuthErrorNoDataPlan", authErr.Kind)
	}

	// Give a wrongful retry/reconnect loop a chance to fire before asserting
	// it did not.
	time.Sleep(300 * time.Millisecond)
	if got := atomic.LoadInt32(&connections); got != 1 {
		t.Fatalf("server accepted %d connections, want exactly 1 — a NotOk auth must never be retried", got)
	}
}

func TestFeedAuthNotOkInvalidToken(t *testing.T) {
	srv := StartFeedTestServer(func(ctx context.Context, conn *websocket.Conn) {
		if _, err := readJSONFrame(ctx, conn); err != nil {
			return
		}
		b, _ := json.Marshal(map[string]any{
			"stat": "NotOk", "type": "cn", "msg": "Session token expired", "stCode": 40103,
		})
		conn.Write(ctx, websocket.MessageText, b)
	})
	defer srv.Close()

	feed, err := NewFeed(Options{WSBaseURL: srv.WSURL(), AccessToken: "stale-tok"})
	if err != nil {
		t.Fatal(err)
	}
	defer feed.Close()

	err = feed.Connect(context.Background())
	var authErr *FeedAuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("err = %v, want *FeedAuthError", err)
	}
	if authErr.Kind != FeedAuthErrorInvalidToken {
		t.Errorf("Kind = %v, want FeedAuthErrorInvalidToken", authErr.Kind)
	}
}

// Both maxScripPerReq (split large subscriptions across requests) and
// maxScripPerConn (refuse locally rather than let the server drop the
// connection) must be honoured.
func TestFeedLimitsHonoured(t *testing.T) {
	var mu sync.Mutex
	var subRequests [][]string

	srv := StartFeedTestServer(func(ctx context.Context, conn *websocket.Conn) {
		if _, err := readJSONFrame(ctx, conn); err != nil {
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, okAuthAck(5, 2)); err != nil {
			return
		}
		for {
			req, err := readJSONFrame(ctx, conn)
			if err != nil {
				return
			}
			if req["type"] == "mws" {
				scrips, _ := req["scrips"].(string)
				mu.Lock()
				subRequests = append(subRequests, strings.Split(scrips, "&"))
				mu.Unlock()
			}
		}
	})
	defer srv.Close()

	feed, err := NewFeed(Options{WSBaseURL: srv.WSURL(), AccessToken: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	defer feed.Close()
	if err := feed.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}

	// maxScripPerReq is 2: 3 keys must split into two requests.
	if err := feed.SubscribeQuotes(context.Background(), []string{"NSE@1", "NSE@2", "NSE@3"}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(subRequests)
		got := append([][]string(nil), subRequests...)
		mu.Unlock()
		if n == 2 {
			if len(got[0])+len(got[1]) != 3 {
				t.Fatalf("split sizes = %v, want to total 3", got)
			}
			if len(got[0]) > 2 || len(got[1]) > 2 {
				t.Fatalf("split sizes = %v, want each at most maxScripPerReq=2", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("received %d subscribe requests, want 2 (maxScripPerReq=2 splitting 3 keys)", n)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// maxScripPerConn is 5; 3 are already subscribed, so 3 more (total 6)
	// must be refused locally, before anything is sent to the server.
	err = feed.SubscribeQuotes(context.Background(), []string{"NSE@4", "NSE@5", "NSE@6"})
	var limitErr *FeedLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("err = %v, want *FeedLimitError", err)
	}
	if limitErr.Max != 5 || limitErr.Requested != 6 {
		t.Errorf("limitErr = %+v, want Max 5 Requested 6", limitErr)
	}
}

// A FULL market-watch tick carries quote fields and top-of-book fields
// together; they must be split into two separate typed emissions, and the
// top-of-book split must never be presented as a five-level book.
func TestFeedQuoteDepthSplit(t *testing.T) {
	srv := StartFeedTestServer(func(ctx context.Context, conn *websocket.Conn) {
		if _, err := readJSONFrame(ctx, conn); err != nil {
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, okAuthAck(500, 200)); err != nil {
			return
		}
		if _, err := readJSONFrame(ctx, conn); err != nil { // the subscribe request
			return
		}
		tick, _ := json.Marshal([]map[string]any{{
			"name": "sf", "e": "nse_cm", "tk": "2885",
			"ltp": "1905.65", "op": "1900.00",
			"bp": "1905.60", "bq": "10", "sp": "1905.70", "bs": "20",
		}})
		conn.Write(ctx, websocket.MessageText, tick)
		<-ctx.Done()
	})
	defer srv.Close()

	feed, err := NewFeed(Options{WSBaseURL: srv.WSURL(), AccessToken: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	defer feed.Close()
	if err := feed.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := feed.SubscribeQuotes(context.Background(), []string{"NSE@2885"}); err != nil {
		t.Fatal(err)
	}

	var quote Quote
	select {
	case quote = <-feed.Quotes():
	case <-time.After(2 * time.Second):
		t.Fatal("no Quote event arrived")
	}
	var depth Depth
	select {
	case depth = <-feed.Depths():
	case <-time.After(2 * time.Second):
		t.Fatal("no Depth event arrived")
	}

	if quote.ScripKey != "NSE@2885" || quote.LastTradedPrice != "1905.65" {
		t.Errorf("quote = %+v", quote)
	}
	if depth.Levels != 1 || depth.Source != DepthSourceQuote {
		t.Errorf("depth = %+v, want Levels 1, Source DepthSourceQuote", depth)
	}
	if len(depth.Bids) != 1 || depth.Bids[0].Price != "1905.60" || len(depth.Asks) != 1 || depth.Asks[0].Price != "1905.70" {
		t.Errorf("depth book = %+v", depth)
	}
}

// A transport failure must trigger reconnect-with-backoff, re-authenticate,
// and re-subscribe everything before the Feed is usable again.
func TestFeedReconnectAndResubscribe(t *testing.T) {
	var connections int32
	resubscribed := make(chan []string, 1)

	srv := StartFeedTestServer(func(ctx context.Context, conn *websocket.Conn) {
		n := atomic.AddInt32(&connections, 1)
		if _, err := readJSONFrame(ctx, conn); err != nil {
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, okAuthAck(500, 200)); err != nil {
			return
		}
		req, err := readJSONFrame(ctx, conn)
		if err != nil {
			return
		}
		scrips, _ := req["scrips"].(string)

		if n == 1 {
			// Simulate a transport failure right after the first subscribe.
			conn.CloseNow()
			return
		}
		select {
		case resubscribed <- strings.Split(scrips, "&"):
		default:
		}
		<-ctx.Done()
	})
	defer srv.Close()

	feed, err := NewFeed(Options{WSBaseURL: srv.WSURL(), AccessToken: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	defer feed.Close()
	if err := feed.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := feed.SubscribeQuotes(context.Background(), []string{"NSE@2885"}); err != nil {
		t.Fatal(err)
	}

	select {
	case scrips := <-resubscribed:
		if len(scrips) != 1 || scrips[0] != "nse_cm|2885" {
			t.Fatalf("resubscribed scrips = %v, want [nse_cm|2885]", scrips)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no reconnect-and-resubscribe observed within 5s")
	}
	if got := atomic.LoadInt32(&connections); got < 2 {
		t.Fatalf("connections = %d, want at least 2 (initial + reconnect)", got)
	}
}

// There is no sandbox feed: a Feed configured for sandbox must fail at
// construction rather than connecting anywhere.
func TestFeedSandboxRefusedAtConstruction(t *testing.T) {
	_, err := NewFeed(Options{Environment: EnvironmentSandbox, AccessToken: "tok"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "sandbox") {
		t.Errorf("err = %v, want it to mention sandbox", err)
	}

	// Also true when sandbox comes from the environment variable rather than
	// an explicit option.
	_, err = resolveConfigForFeedTest(t, "sandbox")
	if err == nil {
		t.Fatal("expected an error for HIGH_ENVIRONMENT=sandbox")
	}
}

func resolveConfigForFeedTest(t *testing.T, env string) (*Feed, error) {
	t.Helper()
	opts := Options{AccessToken: "tok", getenv: func(k string) string {
		if k == "HIGH_ENVIRONMENT" {
			return env
		}
		return ""
	}}
	return NewFeed(opts)
}

// The auth frame carries the access token as "sessionid"; it must never
// reach a log line, and the Feed itself must not print it via %v.
func TestFeedCredentialsAbsentFromLogs(t *testing.T) {
	lines, sink := newCaptureSink()
	srv := StartFeedTestServer(func(ctx context.Context, conn *websocket.Conn) {
		if _, err := readJSONFrame(ctx, conn); err != nil {
			return
		}
		conn.Write(ctx, websocket.MessageText, okAuthAck(500, 200))
		<-ctx.Done()
	})
	defer srv.Close()

	const secretToken = "super-secret-access-token-value"
	feed, err := NewFeed(Options{
		WSBaseURL: srv.WSURL(), AccessToken: secretToken,
		LogLevel: LogLevelDebug, LogSink: sink,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer feed.Close()
	if err := feed.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}

	if rendered := fmt.Sprintf("%v / %+v / %#v", feed, feed, feed); strings.Contains(rendered, secretToken) {
		t.Fatalf("formatting the Feed leaked the access token: %s", rendered)
	}

	time.Sleep(50 * time.Millisecond) // let the debug log line for the ack land
	for _, line := range *lines {
		blob := line.message + fmt.Sprint(line.detail)
		if strings.Contains(blob, secretToken) {
			t.Fatalf("a log line leaked the access token: %+v", line)
		}
	}
}

// Cancelling the context passed to Connect must close the socket cleanly --
// no reconnect attempt, and every delivery channel closes so a caller's
// range loop terminates.
func TestFeedContextCancellationClosesCleanly(t *testing.T) {
	serverSawClose := make(chan struct{})
	srv := StartFeedTestServer(func(ctx context.Context, conn *websocket.Conn) {
		if _, err := readJSONFrame(ctx, conn); err != nil {
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, okAuthAck(500, 200)); err != nil {
			return
		}
		if _, _, err := conn.Read(ctx); err != nil {
			close(serverSawClose)
		}
	})
	defer srv.Close()

	feed, err := NewFeed(Options{WSBaseURL: srv.WSURL(), AccessToken: "tok"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := feed.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	cancel()

	select {
	case <-serverSawClose:
	case <-time.After(10 * time.Second):
		t.Fatal("server never observed the socket close after ctx cancellation")
	}

	done := make(chan struct{})
	go func() {
		for range feed.Quotes() {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Quotes channel never closed after context cancellation")
	}

	_ = feed.Close() // idempotent, and must not hang
}
