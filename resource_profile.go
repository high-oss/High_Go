// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"context"

	"github.com/high-oss/High_Go/generated"
)

// ProfileResource covers the account holder's profile.
type ProfileResource struct {
	config *resolvedConfig
}

// Get returns the account holder's identity, demat and bank details. PAN,
// Aadhaar and bank account numbers are masked to their last four characters.
func (r *ProfileResource) Get(ctx context.Context) (generated.Profile, error) {
	return doRequest[generated.Profile](ctx, r.config, requestOptions{method: "GET", path: "/profile", auth: authBearer})
}
