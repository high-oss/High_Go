// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

// Package highopenapi is the official Go SDK for the HIGH Open API.
package highopenapi

// Client is the HIGH Open API client.
//
//	high, err := highopenapi.New(highopenapi.Options{
//		Environment: highopenapi.EnvironmentSandbox,
//		APIKey:      apiKey,
//		AccessToken: accessToken,
//	})
//	funds, err := high.Portfolio.Funds(ctx)
type Client struct {
	config *resolvedConfig

	Auth        *AuthResource
	Instruments *InstrumentsResource
	Market      *MarketResource
	Orders      *OrdersResource
	Portfolio   *PortfolioResource
	Scrips      *ScripsResource
}

// New builds a Client. Every invalid option — an unknown environment, an
// unknown log level, a non-positive timeout, a negative retry count — fails
// here, naming the valid values, never at request time.
func New(opts Options) (*Client, error) {
	if opts.getenv == nil {
		opts.getenv = defaultGetenv
	}
	config, err := resolveConfig(opts)
	if err != nil {
		return nil, err
	}
	return &Client{
		config:      config,
		Auth:        &AuthResource{config: config},
		Instruments: &InstrumentsResource{config: config},
		Market:      &MarketResource{config: config},
		Orders:      &OrdersResource{config: config},
		Portfolio:   &PortfolioResource{config: config},
		Scrips:      &ScripsResource{config: config},
	}, nil
}

// String never includes credentials — it is the redacted form fmt.Sprintf,
// log.Printf and every %v/%+v call site fall back to. Go does print
// unexported struct fields under %v, so a custom Stringer is the only
// reliable guard, not the unexported fields alone (those are still defence
// in depth: nothing outside this package can read them off a live config).
func (c *Client) String() string {
	return "highopenapi.Client{baseURL: " + c.config.baseURL + ", environment: redacted-credentials}"
}

// GoString backs %#v the same way String backs %v/%s.
func (c *Client) GoString() string { return c.String() }
