// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	highopenapi "github.com/high-oss/High_Go"
	"github.com/high-oss/High_Go/generated"
)

func TestOrdersPlacePostsTheBodyToOrders(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{"orderId": "1", "error": ""}))
	})
	defer ts.Close()

	result, err := clientFor(t, ts, nil).Orders.Place(context.Background(), highopenapi.PlaceOrderRequest{
		TradeSide:     generated.PlaceOrderJSONBodyTradeSideB,
		TradingSymbol: highopenapi.Ptr("RELIANCE-EQ"),
		ProductType:   generated.PlaceOrderJSONBodyProductTypeDELIVERY,
		Flavor:        generated.PlaceOrderJSONBodyFlavorREGULAR,
		OrderType:     generated.PlaceOrderJSONBodyOrderTypeLIMIT,
		Validity:      generated.PlaceOrderJSONBodyValidityDAY,
		Quantity:      10,
		Price:         highopenapi.Ptr(float32(1410)),
		IsAMO:         false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OrderId != "1" {
		t.Errorf("OrderId = %q", result.OrderId)
	}
	req := ts.Req(0)
	if req.Method != "POST" || req.URL != "/v1/orders" {
		t.Fatalf("method/URL = %s %s", req.Method, req.URL)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(req.Body), &sent); err != nil {
		t.Fatal(err)
	}
	if sent["tradingSymbol"] != "RELIANCE-EQ" {
		t.Errorf("sent tradingSymbol = %v", sent["tradingSymbol"])
	}
}

func TestOrdersModifyPatchesOrders(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{"orderId": "1", "error": ""}))
	})
	defer ts.Close()

	orderID := "2609250000123502"
	_, err := clientFor(t, ts, nil).Orders.Modify(context.Background(), highopenapi.ModifyOrderRequest{
		OrderId:       orderID,
		TradeSide:     generated.ModifyOrderJSONBodyTradeSideB,
		TradingSymbol: highopenapi.Ptr("RELIANCE-EQ"),
		ProductType:   generated.ModifyOrderJSONBodyProductType(generated.PlaceOrderJSONBodyProductTypeDELIVERY),
		Flavor:        generated.ModifyOrderJSONBodyFlavor(generated.PlaceOrderJSONBodyFlavorREGULAR),
		OrderType:     generated.ModifyOrderJSONBodyOrderType(generated.PlaceOrderJSONBodyOrderTypeLIMIT),
		Validity:      generated.ModifyOrderJSONBodyValidity(generated.PlaceOrderJSONBodyValidityDAY),
		Quantity:      5,
		Price:         highopenapi.Ptr(float32(1400)),
		IsAMO:         false,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := ts.Req(0)
	if req.Method != "PATCH" || req.URL != "/v1/orders" {
		t.Fatalf("method/URL = %s %s", req.Method, req.URL)
	}
}

func TestOrdersCancelDeletesTheOrderByID(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{"orderId": "1", "error": ""}))
	})
	defer ts.Close()

	_, err := clientFor(t, ts, nil).Orders.Cancel(context.Background(), "2609250000123502")
	if err != nil {
		t.Fatal(err)
	}
	req := ts.Req(0)
	if req.Method != "DELETE" || req.URL != "/v1/orders/2609250000123502" {
		t.Fatalf("method/URL = %s %s", req.Method, req.URL)
	}
}

func TestOrdersGetFetchesOneOrder(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{"orders": []any{}, "scrips": map[string]any{}}))
	})
	defer ts.Close()

	book, err := clientFor(t, ts, nil).Orders.Get(context.Background(), "2609250000123456")
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Orders) != 0 {
		t.Errorf("Orders = %v", book.Orders)
	}
	if ts.Req(0).URL != "/v1/orders/2609250000123456" {
		t.Errorf("URL = %q", ts.Req(0).URL)
	}
}

func TestOrdersListFetchesTheOrderBook(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{"orders": []any{}, "scrips": map[string]any{}}))
	})
	defer ts.Close()

	if _, err := clientFor(t, ts, nil).Orders.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ts.Req(0).URL != "/v1/orders/list" {
		t.Errorf("URL = %q", ts.Req(0).URL)
	}
}

func TestOrdersTradesFetchesTheTradeBook(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{"trades": []any{}, "scrips": map[string]any{}}))
	})
	defer ts.Close()

	if _, err := clientFor(t, ts, nil).Orders.Trades(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ts.Req(0).URL != "/v1/orders/trades" {
		t.Errorf("URL = %q", ts.Req(0).URL)
	}
}

func TestOrdersTradesForFetchesTheFillsOfOneOrder(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{"trades": []any{}, "scrips": map[string]any{}}))
	})
	defer ts.Close()

	if _, err := clientFor(t, ts, nil).Orders.TradesFor(context.Background(), "2609250000123456"); err != nil {
		t.Fatal(err)
	}
	if ts.Req(0).URL != "/v1/orders/2609250000123456/trades" {
		t.Errorf("URL = %q", ts.Req(0).URL)
	}
}

func TestOrdersChargesPostsAnEstimateRequest(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"quantity": 10, "price": 1410, "productType": "DELIVERY", "tradeSide": "B",
			"tradedValue": 14100, "charges": map[string]any{
				"brokerage": 5, "stt": 0, "clearingCharges": 0, "dpCharges": 0, "stampDuty": 0,
				"transactionCharges": 0, "turnoverCharges": 0, "gst": 0, "totalBuyCharges": 0,
				"totalSellCharges": 0, "breakevenPrice": 0,
			},
		}))
	})
	defer ts.Close()

	charges, err := clientFor(t, ts, nil).Orders.Charges(context.Background(), highopenapi.OrderChargesRequest{
		TradingSymbol: highopenapi.Ptr("RELIANCE-EQ"),
		Quantity:      10,
		Price:         highopenapi.Ptr(float32(1410)),
		ProductType:   generated.OrderChargesJSONBodyProductTypeDELIVERY,
		TradeSide:     generated.OrderChargesJSONBodyTradeSideB,
	})
	if err != nil {
		t.Fatal(err)
	}
	if charges.TradedValue != 14100 {
		t.Errorf("TradedValue = %v", charges.TradedValue)
	}
	req := ts.Req(0)
	if req.Method != "POST" || req.URL != "/v1/orders/charges" {
		t.Fatalf("method/URL = %s %s", req.Method, req.URL)
	}
}

func TestOrdersMarginPostsAnArrayOfOrders(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"margins": []any{}, "spanSummary": map[string]any{
				"span": 0, "exposureMargin": 0, "optionPremium": 0, "marginBenefit": 0, "totalMargin": 0,
			},
		}))
	})
	defer ts.Close()

	_, err := clientFor(t, ts, nil).Orders.Margin(context.Background(), highopenapi.OrderMarginRequest{
		{
			TradingSymbol: highopenapi.Ptr("RELIANCE-EQ"),
			Quantity:      10,
			Price:         highopenapi.Ptr(float32(1410)),
			ProductType:   generated.OrderMarginJSONBodyProductTypeDELIVERY,
			TradeSide:     generated.OrderMarginJSONBodyTradeSideB,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := ts.Req(0)
	if req.URL != "/v1/orders/margin" {
		t.Errorf("URL = %q", req.URL)
	}
	var sent []any
	if err := json.Unmarshal([]byte(req.Body), &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 {
		t.Errorf("expected an array body, got %v", sent)
	}
}

// Path parameters percent-encoded (contract §11 item 11): an order id that
// would otherwise alter the path.
func TestOrdersCancelEncodesAnOrderIDThatWouldAlterThePath(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{"orderId": "x", "error": ""}))
	})
	defer ts.Close()

	if _, err := clientFor(t, ts, nil).Orders.Cancel(context.Background(), "a/b"); err != nil {
		t.Fatal(err)
	}
	if ts.Req(0).URL != "/v1/orders/a%2Fb" {
		t.Errorf("URL = %q", ts.Req(0).URL)
	}
}
