// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"context"

	"github.com/high-live/high-openapi/generated"
)

// Request/response types for orders, derived from the generated package so a
// spec change that renames or retypes a field breaks the build rather than
// silently drifting.
type (
	PlaceOrderRequest   = generated.PlaceOrderJSONRequestBody
	ModifyOrderRequest  = generated.ModifyOrderJSONRequestBody
	OrderChargesRequest = generated.OrderChargesJSONRequestBody
	OrderMarginRequest  = generated.OrderMarginJSONRequestBody
)

// OrdersResource covers order placement, modification, cancellation, the
// order and trade books, and pre-trade estimates.
type OrdersResource struct {
	config *resolvedConfig
}

// Place places a regular, bracket, cover or GTT order. Never retried.
func (r *OrdersResource) Place(ctx context.Context, body PlaceOrderRequest) (generated.OrderPlacementResult, error) {
	return doRequest[generated.OrderPlacementResult](ctx, r.config, requestOptions{
		method: "POST", path: "/orders", auth: authBearer, body: body,
	})
}

// Modify modifies a pending order. Never retried.
func (r *OrdersResource) Modify(ctx context.Context, body ModifyOrderRequest) (generated.OrderPlacementResult, error) {
	return doRequest[generated.OrderPlacementResult](ctx, r.config, requestOptions{
		method: "PATCH", path: "/orders", auth: authBearer, body: body,
	})
}

// Get fetches one order, in the same shape as the order book.
func (r *OrdersResource) Get(ctx context.Context, orderID string) (generated.OrderBook, error) {
	path, err := pathOf("/orders/{orderId}", map[string]string{"orderId": orderID})
	if err != nil {
		return generated.OrderBook{}, err
	}
	return doRequest[generated.OrderBook](ctx, r.config, requestOptions{method: "GET", path: path, auth: authBearer})
}

// Cancel cancels a pending order. Never retried.
func (r *OrdersResource) Cancel(ctx context.Context, orderID string) (generated.OrderCancelResult, error) {
	path, err := pathOf("/orders/{orderId}", map[string]string{"orderId": orderID})
	if err != nil {
		return generated.OrderCancelResult{}, err
	}
	return doRequest[generated.OrderCancelResult](ctx, r.config, requestOptions{method: "DELETE", path: path, auth: authBearer})
}

// List returns today's order book.
func (r *OrdersResource) List(ctx context.Context) (generated.OrderBook, error) {
	return doRequest[generated.OrderBook](ctx, r.config, requestOptions{method: "GET", path: "/orders/list", auth: authBearer})
}

// Trades returns today's trade book.
func (r *OrdersResource) Trades(ctx context.Context) (generated.TradeBook, error) {
	return doRequest[generated.TradeBook](ctx, r.config, requestOptions{method: "GET", path: "/orders/trades", auth: authBearer})
}

// TradesFor returns the fills of one order.
func (r *OrdersResource) TradesFor(ctx context.Context, orderID string) (generated.TradeBook, error) {
	path, err := pathOf("/orders/{orderId}/trades", map[string]string{"orderId": orderID})
	if err != nil {
		return generated.TradeBook{}, err
	}
	return doRequest[generated.TradeBook](ctx, r.config, requestOptions{method: "GET", path: path, auth: authBearer})
}

// Charges returns the estimated brokerage and statutory charges for one order.
func (r *OrdersResource) Charges(ctx context.Context, body OrderChargesRequest) (generated.OrderCharges, error) {
	return doRequest[generated.OrderCharges](ctx, r.config, requestOptions{
		method: "POST", path: "/orders/charges", auth: authBearer, body: body,
	})
}

// Margin returns the margin required for a basket of orders.
func (r *OrdersResource) Margin(ctx context.Context, body OrderMarginRequest) (generated.OrderMargin, error) {
	return doRequest[generated.OrderMargin](ctx, r.config, requestOptions{
		method: "POST", path: "/orders/margin", auth: authBearer, body: body,
	})
}
