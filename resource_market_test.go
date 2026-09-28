// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi_test

import (
	"context"
	"net/http"
	"testing"

	highopenapi "github.com/high-live/high-openapi"
)

func TestMarketStatusReturnsTheExchangeStatusMap(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"isHoliday": false, "date": "2026-09-25",
			"exchangeStatus": map[string]any{"NSE": map[string]any{"status": "OPEN", "timeRemainingInSeconds": 1}},
		}))
	})
	defer ts.Close()

	status, err := clientFor(t, ts, nil).Market.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	nse, ok := status.ExchangeStatus["NSE"]
	if !ok || nse.Status != "OPEN" {
		t.Fatalf("exchangeStatus[NSE] = %+v, ok=%v", nse, ok)
	}
	if ts.Req(0).URL != "/v1/market/status" {
		t.Errorf("URL = %q", ts.Req(0).URL)
	}
}

func TestClientHonoursAnExplicitVersionPath(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"isHoliday": false, "date": "2026-09-25", "exchangeStatus": map[string]any{},
		}))
	})
	defer ts.Close()

	client := clientFor(t, ts, func(o *highopenapi.Options) { o.VersionPath = highopenapi.Ptr("v2") })
	if _, err := client.Market.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ts.Req(0).URL != "/v2/market/status" {
		t.Errorf("URL = %q", ts.Req(0).URL)
	}
}
