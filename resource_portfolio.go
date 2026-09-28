// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"context"

	"github.com/high-live/high-openapi/generated"
)

type (
	ConvertPositionRequest = generated.ConvertPositionJSONRequestBody
	ExitPositionRequest    = generated.ExitPositionJSONRequestBody
)

// PortfolioResource covers positions, holdings and funds.
type PortfolioResource struct {
	config *resolvedConfig
}

// Positions returns open intraday and carry-forward positions with their P&L snapshot.
func (r *PortfolioResource) Positions(ctx context.Context) (generated.Positions, error) {
	return doRequest[generated.Positions](ctx, r.config, requestOptions{method: "GET", path: "/portfolio/positions", auth: authBearer})
}

// Holdings returns demat holdings with their investment snapshot.
func (r *PortfolioResource) Holdings(ctx context.Context) (generated.Holdings, error) {
	return doRequest[generated.Holdings](ctx, r.config, requestOptions{method: "GET", path: "/portfolio/holdings", auth: authBearer})
}

// Funds returns cash, margin and charge balances.
func (r *PortfolioResource) Funds(ctx context.Context) (generated.Funds, error) {
	return doRequest[generated.Funds](ctx, r.config, requestOptions{method: "GET", path: "/portfolio/funds", auth: authBearer})
}

// ConvertPosition converts a position between product types. Never retried.
func (r *PortfolioResource) ConvertPosition(ctx context.Context, body ConvertPositionRequest) (generated.PositionConvertResult, error) {
	return doRequest[generated.PositionConvertResult](ctx, r.config, requestOptions{
		method: "PATCH", path: "/portfolio/positions/convert", auth: authBearer, body: body,
	})
}

// ExitAllPositions squares off every open position. Never retried.
func (r *PortfolioResource) ExitAllPositions(ctx context.Context) (generated.PositionExitAllResult, error) {
	return doRequest[generated.PositionExitAllResult](ctx, r.config, requestOptions{
		method: "DELETE", path: "/portfolio/positions/exit/all", auth: authBearer,
	})
}

// ExitPosition squares off one position. The API expects a body on this
// DELETE. Never retried.
func (r *PortfolioResource) ExitPosition(ctx context.Context, body ExitPositionRequest) (generated.PositionExitResult, error) {
	return doRequest[generated.PositionExitResult](ctx, r.config, requestOptions{
		method: "DELETE", path: "/portfolio/positions/exit", auth: authBearer, body: body,
	})
}
