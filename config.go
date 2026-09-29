// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Environment selects which HIGH host a client talks to.
type Environment string

const (
	// EnvironmentProduction is the default.
	EnvironmentProduction Environment = "production"
	EnvironmentSandbox    Environment = "sandbox"
)

// EnvironmentHosts is the REST and datafeed-socket host for one environment.
//
// WS is reserved for the datafeed client, which is not in this release (see
// contract §9) — it is resolved here so that client reuses this configuration
// and these credentials rather than introducing a second config surface.
type EnvironmentHosts struct {
	API string
	WS  string
}

// Environments maps each environment to its API host, taken from the spec's
// `servers` block by its `x-environment` name, and its datafeed-socket host.
// A test asserts this still matches the pinned spec.
var Environments = map[Environment]EnvironmentHosts{
	EnvironmentProduction: {API: "https://openapi.high.live", WS: "wss://openapi.high.live"},
	EnvironmentSandbox:    {API: "https://sandbox.high.live", WS: "wss://sandbox.high.live"},
}

// Ptr is a small convenience for building the pointer-typed Options fields —
// TimeoutMs, MaxRetries, MaxRetryDelayMs and VersionPath all distinguish "not
// set, use the default" (nil) from an explicit value (including an explicit
// zero, which the numeric ones reject at construction). A plain int/string
// field cannot make that distinction, and the contract requires it: an
// explicitly-zero timeout must fail construction naming the option, not
// silently take the default.
func Ptr[T any](v T) *T { return &v }

// Options configures a Client. Every field is optional; see the README for
// the resolution order and the environment variables that back each one.
type Options struct {
	// Environment selects the host. Ignored when BaseURL is set. Default: production.
	Environment Environment
	// BaseURL overrides Environment entirely. Use for a staging host or a local mock.
	BaseURL string
	// WSBaseURL is the datafeed socket host. Reserved for the feed client; overrides Environment.
	WSBaseURL string
	// VersionPath is the segment between the host and the operation path.
	// nil means "not set" (default "v1"); Ptr("") is a valid, explicit
	// unversioned host.
	VersionPath *string
	// APIKey is sent as x-api-key. Required by auth.GenerateAccessToken.
	APIKey string
	// AccessToken is sent as Authorization: Bearer. Required by the other 23 operations.
	AccessToken string
	// TimeoutMs is the per-request timeout, covering the whole exchange
	// (headers and body). nil means "not set" (default 30000); must be > 0.
	TimeoutMs *int
	// MaxRetries is the retry count for idempotent reads only. nil means "not set" (default 2).
	MaxRetries *int
	// MaxRetryDelayMs ceils a single retry delay; a longer Retry-After is not
	// waited out. nil means "not set" (default 30000); must be > 0.
	MaxRetryDelayMs *int
	// UserAgent is appended to the SDK's own.
	UserAgent string
	// LogLevel controls how much the SDK prints. Empty means "not set" (default silent).
	LogLevel LogLevel
	// LogSink is where log lines go. Default: an os.Stderr-backed sink.
	LogSink LogSink
	// HTTPClient overrides the transport, for tests or a proxy-aware client.
	HTTPClient *http.Client
	// InstrumentsAllowedHosts is the allowlist of hosts Instruments.Stream and
	// Instruments.List may download the instrument list from. nil means "not
	// set" (default: the CDN host the instrument list is published under
	// today). A file whose host is not on this list is refused before any
	// request for it is made.
	InstrumentsAllowedHosts []string

	// getenv backs environment-variable resolution. Exposed only for tests;
	// production callers never set it (New defaults it to os.Getenv).
	getenv func(string) string
}

// resolvedConfig is what every request is actually built from. Constructed
// once, in resolveConfig, and never mutated afterwards.
//
// apiKey and accessToken are unexported so that nothing outside this package
// can read them off a live config, and Client.String/GoString redact
// explicitly on top of that as defense in depth (contract §7).
type resolvedConfig struct {
	baseURL                 string
	wsBaseURL               string
	versionPath             string
	apiKey                  string
	accessToken             string
	timeoutMs               int
	maxRetries              int
	maxRetryDelayMs         int
	userAgent               string
	logLevel                LogLevel
	logger                  *logger
	httpClient              *http.Client
	instrumentsAllowedHosts []string
}

const defaultUserAgent = "high-sdk-go/0.0.1"

func trimSlashes(s string) string {
	return strings.Trim(s, "/")
}

// resolveConfig settles where requests go and what they carry, once, at
// client construction. Resolution order: an explicit BaseURL wins outright;
// then an explicit Environment (which also suppresses the HIGH_BASE_URL /
// HIGH_WS_BASE_URL env vars — see contract §2); then the env vars; then
// production. An unknown environment or log level, or a non-positive
// timeout/negative retry count, fails here — never at request time.
func resolveConfig(opts Options) (*resolvedConfig, error) {
	getenv := opts.getenv
	if getenv == nil {
		getenv = noopGetenv
	}

	explicitEnvironment := opts.Environment != ""
	environment := EnvironmentProduction
	if explicitEnvironment {
		if _, ok := Environments[opts.Environment]; !ok {
			return nil, fmt.Errorf("unknown HIGH environment %q (Options.Environment). Valid values: %s.",
				opts.Environment, validEnvironments())
		}
		environment = opts.Environment
	} else if fromEnv := getenv("HIGH_ENVIRONMENT"); fromEnv != "" {
		if _, ok := Environments[Environment(fromEnv)]; !ok {
			return nil, fmt.Errorf("unknown HIGH environment %q (HIGH_ENVIRONMENT). Valid values: %s.",
				fromEnv, validEnvironments())
		}
		environment = Environment(fromEnv)
	}

	hosts := Environments[environment]

	baseURL := opts.BaseURL
	if baseURL == "" {
		if explicitEnvironment {
			baseURL = hosts.API
		} else if fromEnv := getenv("HIGH_BASE_URL"); fromEnv != "" {
			baseURL = fromEnv
		} else {
			baseURL = hosts.API
		}
	}

	wsBaseURL := opts.WSBaseURL
	if wsBaseURL == "" {
		if explicitEnvironment {
			wsBaseURL = hosts.WS
		} else if fromEnv := getenv("HIGH_WS_BASE_URL"); fromEnv != "" {
			wsBaseURL = fromEnv
		} else {
			wsBaseURL = hosts.WS
		}
	}

	userAgent := defaultUserAgent
	if opts.UserAgent != "" {
		userAgent = defaultUserAgent + " " + opts.UserAgent
	}

	logLevel := LogLevelSilent
	if opts.LogLevel != "" {
		if !opts.LogLevel.valid() {
			return nil, fmt.Errorf("unknown HIGH log level %q (Options.LogLevel). Valid values: %s.",
				opts.LogLevel, strings.Join(logLevelStrings(), ", "))
		}
		logLevel = opts.LogLevel
	} else if fromEnv := getenv("HIGH_LOG_LEVEL"); fromEnv != "" {
		if !LogLevel(fromEnv).valid() {
			return nil, fmt.Errorf("unknown HIGH log level %q (HIGH_LOG_LEVEL). Valid values: %s.",
				fromEnv, strings.Join(logLevelStrings(), ", "))
		}
		logLevel = LogLevel(fromEnv)
	}

	timeoutMs := 30_000
	if opts.TimeoutMs != nil {
		timeoutMs = *opts.TimeoutMs
	}
	if timeoutMs <= 0 {
		return nil, fmt.Errorf("timeoutMs must be a positive number, received %d.", timeoutMs)
	}

	maxRetries := 2
	if opts.MaxRetries != nil {
		maxRetries = *opts.MaxRetries
	}
	if maxRetries < 0 {
		return nil, fmt.Errorf("maxRetries must be a non-negative integer, received %d.", maxRetries)
	}

	maxRetryDelayMs := 30_000
	if opts.MaxRetryDelayMs != nil {
		maxRetryDelayMs = *opts.MaxRetryDelayMs
	}
	if maxRetryDelayMs <= 0 {
		return nil, fmt.Errorf("maxRetryDelayMs must be a positive number, received %d.", maxRetryDelayMs)
	}

	versionPath := "v1"
	if opts.VersionPath != nil {
		versionPath = trimSlashes(*opts.VersionPath)
	}

	apiKey := opts.APIKey
	if apiKey == "" {
		apiKey = getenv("HIGH_API_KEY")
	}
	accessToken := opts.AccessToken
	if accessToken == "" {
		accessToken = getenv("HIGH_ACCESS_TOKEN")
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	instrumentsAllowedHosts := opts.InstrumentsAllowedHosts
	if instrumentsAllowedHosts == nil {
		instrumentsAllowedHosts = []string{defaultInstrumentsAllowedHost}
	}

	return &resolvedConfig{
		baseURL:                 strings.TrimRight(baseURL, "/"),
		wsBaseURL:               strings.TrimRight(wsBaseURL, "/"),
		versionPath:             versionPath,
		apiKey:                  apiKey,
		accessToken:             accessToken,
		timeoutMs:               timeoutMs,
		maxRetries:              maxRetries,
		maxRetryDelayMs:         maxRetryDelayMs,
		userAgent:               userAgent,
		logLevel:                logLevel,
		logger:                  newLogger(logLevel, opts.LogSink),
		httpClient:              httpClient,
		instrumentsAllowedHosts: instrumentsAllowedHosts,
	}, nil
}

func noopGetenv(string) string { return "" }

// defaultGetenv is what a real Client resolves environment variables with.
// Tests inject Options.getenv instead, so config resolution is deterministic
// regardless of the host shell's environment.
func defaultGetenv(key string) string { return os.Getenv(key) }

func validEnvironments() string {
	return "production, sandbox"
}

// buildURL joins baseURL + versionPath + the operation path, tolerant of
// stray slashes on every part.
func buildURL(config *resolvedConfig, path string) string {
	segments := make([]string, 0, 3)
	if config.baseURL != "" {
		segments = append(segments, config.baseURL)
	}
	if config.versionPath != "" {
		segments = append(segments, config.versionPath)
	}
	if p := trimSlashes(path); p != "" {
		segments = append(segments, p)
	}
	return strings.Join(segments, "/")
}
