// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	highopenapi "github.com/high-oss/High_Go"
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
