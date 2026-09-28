// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"context"

	"github.com/high-oss/High_Go/generated"
)

// MarketResource covers exchange session state.
type MarketResource struct {
	config *resolvedConfig
}

// Status returns session state per exchange for today.
func (r *MarketResource) Status(ctx context.Context) (generated.MarketStatus, error) {
	return doRequest[generated.MarketStatus](ctx, r.config, requestOptions{
		method: "GET", path: "/market/status", auth: authBearer,
	})
}
