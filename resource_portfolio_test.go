// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi_test

import (
	"context"
	"net/http"
	"testing"

	highopenapi "github.com/high-oss/High_Go"
	"github.com/high-oss/High_Go/generated"
)

func TestPortfolioPositionsReturnsSnapshotAndList(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"totalStocks": 1,
			"snapshot": map[string]any{
				"bookedPL": 100, "unrealisedPL": 123, "totalPL": 223,
				"bookedPLPercent": 0.71, "unrealisedPLPercent": 0.88, "totalPLPercent": 0.8,
			},
			"positions": []any{}, "scrips": map[string]any{},
		}))
	})
	defer ts.Close()

	positions, err := clientFor(t, ts, nil).Portfolio.Positions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if positions.Snapshot.TotalPL != 223 {
		t.Errorf("Snapshot.TotalPL = %v", positions.Snapshot.TotalPL)
	}
	if ts.Req(0).URL != "/v1/portfolio/positions" {
		t.Errorf("URL = %q", ts.Req(0).URL)
	}
}

// positions and holdings each have a `snapshot`, but different shapes —
// holdings uses the investment snapshot, not the P&L one.
func TestPortfolioHoldingsUsesTheInvestmentSnapshot(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"totalStocks": 1,
			"snapshot": map[string]any{
				"investment": 13000, "currentValue": 14123, "previousDayValue": 14058,
				"totalPL": 1123, "dayPL": 65, "totalPLPercent": 8.64, "dayPLPercent": 0.46,
			},
			"holdings": []any{}, "scrips": map[string]any{},
		}))
	})
	defer ts.Close()

	holdings, err := clientFor(t, ts, nil).Portfolio.Holdings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if holdings.Snapshot.Investment != 13000 {
		t.Errorf("Snapshot.Investment = %v", holdings.Snapshot.Investment)
	}
	if ts.Req(0).URL != "/v1/portfolio/holdings" {
		t.Errorf("URL = %q", ts.Req(0).URL)
	}
}

func TestPortfolioFundsReturnsBalances(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"availableBalance": 125000.5, "ledgerBalance": 150000, "todaysBalance": 155000.5,
			"marginUtilized": 30000, "marginAgainstAssets": 50000, "todaysPayIn": 10000, "todaysPayout": 5000,
			"mtfFunds":              map[string]any{"mtfCash": 0, "mtfFunded": 0, "totalMTFFunding": 0},
			"charges":               map[string]any{"delayedPaymentCharges": 0, "dpCharges": 0, "totalCharges": 0},
			"unsettledFutureAmount": 0,
		}))
	})
	defer ts.Close()

	funds, err := clientFor(t, ts, nil).Portfolio.Funds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if funds.AvailableBalance != 125000.5 {
		t.Errorf("AvailableBalance = %v", funds.AvailableBalance)
	}
}

func TestPortfolioConvertPositionPatchesConvertEndpoint(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"tradingSymbol": "RELIANCE-EQ", "exchangeSegment": "NSE", "scripCode": "2885",
			"quantity": 50, "isSubmitted": true,
		}))
	})
	defer ts.Close()

	_, err := clientFor(t, ts, nil).Portfolio.ConvertPosition(context.Background(), highopenapi.ConvertPositionRequest{
		TradingSymbol:     highopenapi.Ptr("RELIANCE-EQ"),
		Quantity:          50,
		TradeSide:         generated.ConvertPositionJSONBodyTradeSideB,
		SourceProductType: generated.ConvertPositionJSONBodySourceProductTypeINTRADAY,
		TargetProductType: generated.ConvertPositionJSONBodyTargetProductTypeDELIVERY,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := ts.Req(0)
	if req.Method != "PATCH" || req.URL != "/v1/portfolio/positions/convert" {
		t.Fatalf("method/URL = %s %s", req.Method, req.URL)
	}
}

func TestPortfolioExitAllPositionsDeletesWithNoBody(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"totalCount": 0, "successCount": 0, "failureCount": 0, "positions": []any{},
		}))
	})
	defer ts.Close()

	_, err := clientFor(t, ts, nil).Portfolio.ExitAllPositions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	req := ts.Req(0)
	if req.Method != "DELETE" || req.URL != "/v1/portfolio/positions/exit/all" {
		t.Fatalf("method/URL = %s %s", req.Method, req.URL)
	}
	if req.Body != "" {
		t.Errorf("body = %q, want empty", req.Body)
	}
}

// The API really does expect a body on this DELETE.
func TestPortfolioExitPositionDeletesWithABody(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"tradingSymbol": "RELIANCE-EQ", "exchangeSegment": "NSE", "scripCode": "2885",
			"quantity": 50, "flavor": "REGULAR", "isSubmitted": true,
		}))
	})
	defer ts.Close()

	_, err := clientFor(t, ts, nil).Portfolio.ExitPosition(context.Background(), highopenapi.ExitPositionRequest{
		TradingSymbol: highopenapi.Ptr("RELIANCE-EQ"),
		ProductType:   generated.ExitPositionJSONBodyProductTypeINTRADAY,
		Flavor:        generated.ExitPositionJSONBodyFlavorREGULAR,
		TradeSide:     generated.ExitPositionJSONBodyTradeSideS,
		Quantity:      50,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := ts.Req(0)
	if req.Method != "DELETE" || req.URL != "/v1/portfolio/positions/exit" {
		t.Fatalf("method/URL = %s %s", req.Method, req.URL)
	}
	if req.Body == "" {
		t.Error("expected a non-empty body")
	}
}

func TestPortfolioNeverRetriesASquareOffEvenOn503(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 503, map[string]any{"code": "SERVICE_UNAVAILABLE"})
	})
	defer ts.Close()

	_, err := clientFor(t, ts, nil).Portfolio.ExitAllPositions(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if ts.Count() != 1 {
		t.Fatalf("expected exactly 1 request, got %d", ts.Count())
	}
}
