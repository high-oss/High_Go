// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"context"

	"github.com/high-oss/High_Go/generated"
)

// AuthResource covers TOTP authentication.
//
// The redirect consent flow (/auth/generate-consent, /auth/login,
// /auth/consume-consent) is deliberately not wrapped: it is built around a
// browser page only a human can complete. Token introspection
// (/auth/validate-token) is not wrapped either — a caller learns a token is
// invalid from the next call's 401. See the README.
type AuthResource struct {
	config *resolvedConfig
}

// GenerateAccessToken exchanges the API key plus a TOTP for a 24-hour access token.
func (r *AuthResource) GenerateAccessToken(ctx context.Context, clientID, tOtp string) (generated.AccessToken, error) {
	return doRequest[generated.AccessToken](ctx, r.config, requestOptions{
		method: "GET",
		path:   "/auth/generate-access-token",
		auth:   authAPIKey,
		query:  map[string]string{"clientId": clientID, "tOtp": tOtp},
	})
}
