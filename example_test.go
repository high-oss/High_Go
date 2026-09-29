// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	highopenapi "github.com/high-oss/High_Go"

	"github.com/coder/websocket"
)

// Example is the quickstart from the README, made runnable and verified by
// `go test` (Go's convention for a documented, checked example) against a
// local server standing in for the real HIGH Open API host. Point BaseURL at
// nothing and set Environment instead to talk to the real API.
func Example() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"requestId":"r1","data":{"availableBalance":125000.5,"ledgerBalance":150000,"todaysBalance":155000.5,"marginUtilized":30000,"marginAgainstAssets":50000,"todaysPayIn":10000,"todaysPayout":5000,"mtfFunds":{"mtfCash":0,"mtfFunded":0,"totalMTFFunding":0},"charges":{"delayedPaymentCharges":0,"dpCharges":0,"totalCharges":0},"unsettledFutureAmount":0}}`)
	}))
	defer server.Close()

	high, err := highopenapi.New(highopenapi.Options{
		BaseURL:     server.URL, // in a real program: Environment: highopenapi.EnvironmentSandbox
		APIKey:      "your-api-key",
		AccessToken: "your-access-token",
	})
	if err != nil {
		panic(err)
	}

	funds, err := high.Portfolio.Funds(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Println(funds.AvailableBalance)
	// Output: 125000.5
}

// Example_errors shows the one error type every failing call returns, and
// how to recover its fields with errors.As.
func Example_errors() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"requestId":"r2","code":"ORDER_REJECTED","message":"Insufficient funds"}`)
	}))
	defer server.Close()

	high, err := highopenapi.New(highopenapi.Options{BaseURL: server.URL, AccessToken: "your-access-token"})
	if err != nil {
		panic(err)
	}

	_, err = high.Orders.Place(context.Background(), highopenapi.PlaceOrderRequest{})

	var apiErr *highopenapi.APIError
	if errors.As(err, &apiErr) {
		fmt.Println(apiErr.Status, apiErr.Code)
	}
	// Output: 400 ORDER_REJECTED
}

// Example_instruments streams the equity instrument list and prints the
// first row. It needs no credentials — Instruments is the one resource that
// does not. Two local servers stand in for the real HIGH API and the file's
// real host; a real program only ever talks to the former.
func Example_instruments() {
	const csv = "exchange,segment,instrument,high_trading_symbol,scrip_key,isin,scrip_code,symbol,name,group_series,has_fno,underlying_symbol,expiry,option_type,strike_price,price_tick,lot_size\n" +
		"NSE,ES,EQUITY,RELIANCE-EQ,NSE@2885,INE002A01018,2885,RELIANCE,Reliance Industries,EQ,1,RELIANCE,,,,5,1\n"

	files := highopenapi.StartTLSTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		w.Header().Set("Content-Type", "text/csv")
		fmt.Fprint(w, csv)
	})
	defer files.Close()

	api := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"generatedAt": "2026-09-29T02:53:10.000Z",
			"columns": []string{
				"exchange", "segment", "instrument", "high_trading_symbol", "scrip_key", "isin",
				"scrip_code", "symbol", "name", "group_series", "has_fno", "underlying_symbol",
				"expiry", "option_type", "strike_price", "price_tick", "lot_size",
			},
			"files": []map[string]any{
				{"instrument": "equity", "url": files.URL + "/equity.csv", "bytes": len(csv), "rows": 1,
					"checksum": "d41d8cd98f00b204e9800998ecf8427e", "updatedAt": "2026-09-29T02:53:09.000Z"},
			},
		}))
	})
	defer api.Close()

	filesHost, err := url.Parse(files.URL)
	if err != nil {
		panic(err)
	}

	high, err := highopenapi.New(highopenapi.Options{
		BaseURL:                 api.URL,
		HTTPClient:              files.Client(), // trusts the local files server's certificate
		InstrumentsAllowedHosts: []string{filesHost.Hostname()},
	})
	if err != nil {
		panic(err)
	}

	for instrument, err := range high.Instruments.Stream(context.Background(), highopenapi.InstrumentEquity) {
		if err != nil {
			panic(err)
		}
		fmt.Println(instrument.TradingSymbol, instrument.ScripCode)
		break
	}
	// Output: RELIANCE-EQ 2885
}

// Example_feed subscribes to live quotes for one scrip and prints the first
// tick. A local WebSocket server stands in for the real datafeed host --
// openapi-feed.high.live is production-only and is never dialled by this
// suite (there is no sandbox feed; see Feed's own doc comment and
// NewFeed). A real program passes Environment: highopenapi.EnvironmentProduction
// (or leaves Environment unset) instead of WSBaseURL.
func Example_feed() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		ctx := r.Context()

		// Auth: read the {"type":"cn",...} frame, then acknowledge it with
		// the two limits Feed must honour.
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
		ack, _ := json.Marshal(map[string]any{
			"stat": "Ok", "type": "cn", "msg": "successful", "stCode": 200,
			"maxScripPerConn": 500, "maxScripPerReq": 200, "sType": "v2.0",
		})
		if err := conn.Write(ctx, websocket.MessageText, ack); err != nil {
			return
		}

		// The subscribe request, then one market-watch tick.
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
		tick, _ := json.Marshal([]map[string]any{
			{"name": "sf", "e": "nse_cm", "tk": "2885", "ts": "RELIANCE-EQ", "ltp": "1905.65", "op": "1900.00"},
		})
		conn.Write(ctx, websocket.MessageText, tick)
		<-ctx.Done()
	}))
	defer srv.Close()

	feed, err := highopenapi.NewFeed(highopenapi.Options{
		WSBaseURL:   "ws" + srv.URL[len("http"):], // in a real program: Environment: highopenapi.EnvironmentProduction
		AccessToken: "your-access-token",
	})
	if err != nil {
		panic(err)
	}
	defer feed.Close()

	ctx := context.Background()
	if err := feed.Connect(ctx); err != nil {
		panic(err)
	}
	if err := feed.SubscribeQuotes(ctx, []string{"NSE@2885"}); err != nil {
		panic(err)
	}

	quote := <-feed.Quotes()
	fmt.Println(quote.ScripKey, quote.LastTradedPrice)
	// Output: NSE@2885 1905.65
}
